//go:build linux

package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// f32Writer receives F32LE PCM samples and stores the peak level (0.0-1.0).
type f32Writer struct {
	peak *atomic.Uint32
}

func (w *f32Writer) Write(p []byte) (int, error) {
	var max float32
	for i := 0; i+3 < len(p); i += 4 {
		v := math.Float32frombits(binary.LittleEndian.Uint32(p[i : i+4]))
		if v < 0 {
			v = -v
		}
		if v > max {
			max = v
		}
	}
	if max > 1.0 {
		max = 1.0
	}
	storePeak(w.peak, max)
	return len(p), nil
}

func (w *f32Writer) Format() byte { return proto.FormatFloat32LE }

func storePeak(a *atomic.Uint32, v float32) { a.Store(math.Float32bits(v)) }
func loadPeak(a *atomic.Uint32) float32     { return math.Float32frombits(a.Load()) }

// pulseMonitor implements Monitor by recording F32 PCM from hackmud's audio
// source via PulseAudio (pipewire-pulse) and computing peak levels.
type pulseMonitor struct {
	target           string
	gracePeriod      time.Duration
	silenceThreshold float32
	silenceDuration  time.Duration
	pollInterval     time.Duration

	peak   atomic.Uint32
	active atomic.Bool
	quit   chan struct{}
	done   chan struct{}
}

func NewMonitor(windowTitle string, gracePeriod time.Duration) (Monitor, error) {
	m := &pulseMonitor{
		target:           strings.ToLower(windowTitle),
		gracePeriod:      gracePeriod,
		silenceThreshold: 0.001,
		silenceDuration:  150 * time.Millisecond,
		pollInterval:     50 * time.Millisecond,
		quit:             make(chan struct{}),
		done:             make(chan struct{}),
	}
	storePeak(&m.peak, -1.0)

	go m.run()
	return m, nil
}

func (m *pulseMonitor) run() {
	defer close(m.done)

	for {
		select {
		case <-m.quit:
			return
		default:
		}

		err := m.monitor()
		if err != nil {
			slog.Debug("pulse monitor error, retrying", "error", err)
		}

		m.active.Store(false)
		storePeak(&m.peak, -1.0)

		select {
		case <-m.quit:
			return
		case <-time.After(time.Second):
		}
	}
}

// monitor connects to PulseAudio, finds hackmud's source, and records F32
// audio to compute peak levels until an error or quit.
func (m *pulseMonitor) monitor() error {
	client, err := pulse.NewClient(
		pulse.ClientApplicationName("gomud-monitor"),
	)
	if err != nil {
		return fmt.Errorf("pulse connect: %w", err)
	}
	defer client.Close()

	sourceIdx, sourceName, err := m.findSource(client)
	if err != nil {
		return err
	}

	slog.Debug("setting up audio monitor",
		"source_index", sourceIdx,
		"source_name", sourceName,
	)

	writer := &f32Writer{peak: &m.peak}

	stream, err := client.NewRecord(
		writer,
		pulse.RecordSampleRate(25),
		pulse.RecordMono,
		pulse.RecordRawOption(func(r *proto.CreateRecordStream) {
			r.SourceIndex = sourceIdx
			r.AdjustLatency = true
		}),
	)
	if err != nil {
		return fmt.Errorf("create record stream: %w", err)
	}
	defer stream.Close()

	stream.Start()
	m.active.Store(true)
	slog.Info("audio monitor active", "source", sourceName)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.quit:
			return nil
		case <-ticker.C:
			if stream.Error() != nil {
				return fmt.Errorf("stream error: %w", stream.Error())
			}
			if !stream.Running() {
				return fmt.Errorf("stream stopped")
			}
		}
	}
}

// findSource looks for hackmud's audio source directly in the source list.
// PipeWire exposes per-stream monitor sources with application properties.
func (m *pulseMonitor) findSource(client *pulse.Client) (sourceIdx uint32, sourceName string, err error) {
	var sources proto.GetSourceInfoListReply
	err = client.RawRequest(&proto.GetSourceInfoList{}, &sources)
	if err != nil {
		return 0, "", fmt.Errorf("list sources: %w", err)
	}

	for _, src := range sources {
		binary := ""
		if prop, ok := src.Properties["application.process.binary"]; ok {
			binary = prop.String()
		}
		if strings.Contains(strings.ToLower(binary), m.target) {
			slog.Debug("found target source",
				"index", src.SourceIndex,
				"name", src.SourceName,
				"binary", binary,
			)
			return src.SourceIndex, src.SourceName, nil
		}
	}

	// Fallback: find via sink-input -> sink monitor source
	return m.findSourceViaSinkInput(client)
}

// findSourceViaSinkInput falls back to the sink-input -> sink monitor approach
// if hackmud isn't directly visible as a source.
func (m *pulseMonitor) findSourceViaSinkInput(client *pulse.Client) (sourceIdx uint32, sourceName string, err error) {
	var sinkInputs proto.GetSinkInputInfoListReply
	err = client.RawRequest(&proto.GetSinkInputInfoList{}, &sinkInputs)
	if err != nil {
		return 0, "", fmt.Errorf("list sink-inputs: %w", err)
	}

	for _, si := range sinkInputs {
		binary := ""
		if prop, ok := si.Properties["application.process.binary"]; ok {
			binary = prop.String()
		}
		if !strings.Contains(strings.ToLower(binary), m.target) {
			continue
		}

		slog.Debug("found target via sink-input",
			"sink_input", si.SinkInputIndex,
			"binary", binary,
			"sink_index", si.SinkIndex,
		)

		var sinkInfo proto.GetSinkInfoReply
		err = client.RawRequest(&proto.GetSinkInfo{SinkIndex: si.SinkIndex, SinkName: ""}, &sinkInfo)
		if err != nil {
			return 0, "", fmt.Errorf("get sink info: %w", err)
		}

		return sinkInfo.MonitorSourceIndex, sinkInfo.MonitorSourceName, nil
	}

	return 0, "", fmt.Errorf("no source or sink-input found for %q", m.target)
}

func (m *pulseMonitor) PeakLevel() (float32, error) {
	if !m.active.Load() {
		return -1, nil
	}
	return loadPeak(&m.peak), nil
}

func (m *pulseMonitor) WaitForSilence(ctx context.Context) error {
	requiredCount := max(1, int(m.silenceDuration/m.pollInterval))
	graceDeadline := time.Now().Add(m.gracePeriod)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	audioSeen := false
	silentCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			peak, _ := m.PeakLevel()
			graceExpired := time.Now().After(graceDeadline)

			if peak < 0 {
				if audioSeen {
					slog.Debug("audio stream disappeared after audio was seen")
					return nil
				}
				if graceExpired {
					slog.Debug("no audio stream found within grace period")
					return ErrNoAudioSession
				}
				continue
			}

			if peak > m.silenceThreshold {
				if !audioSeen {
					slog.Debug("audio started", "target", m.target, "peak", peak)
				}
				audioSeen = true
				silentCount = 0
			} else if audioSeen {
				silentCount++
				slog.Debug("audio silent tick",
					"count", silentCount,
					"needed", requiredCount,
					"peak", peak,
				)
				if silentCount >= requiredCount {
					slog.Debug("silence ok", "duration", time.Duration(silentCount)*m.pollInterval, "peak", peak)
					return nil
				}
			} else if graceExpired {
				slog.Debug("audio stream found but below threshold, grace expired", "peak", peak)
				return ErrNoAudioSession
			}
		}
	}
}

func (m *pulseMonitor) Close() error {
	slog.Debug("stopping pulse monitor")
	close(m.quit)

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		slog.Warn("pulse monitor goroutine did not exit within 5s")
		return fmt.Errorf("pulse monitor shutdown timeout")
	}

	slog.Debug("pulse monitor stopped")
	return nil
}
