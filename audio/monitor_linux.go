//go:build linux

package audio

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// peakWriter receives uint8 peak-detect samples from PulseAudio and stores
// the max as a float32 (0.0-1.0) in an atomic uint32.
type peakWriter struct {
	peak *atomic.Uint32
}

func (w *peakWriter) Write(p []byte) (int, error) {
	var max byte
	for _, b := range p {
		if b > max {
			max = b
		}
	}
	storePeak(w.peak, float32(max)/255.0)
	return len(p), nil
}

func (w *peakWriter) Format() byte { return proto.FormatUint8 }

func storePeak(a *atomic.Uint32, v float32) { a.Store(math.Float32bits(v)) }
func loadPeak(a *atomic.Uint32) float32     { return math.Float32frombits(a.Load()) }

// pulseMonitor implements Monitor using PulseAudio's PA_STREAM_PEAK_DETECT
// via the pipewire-pulse compatibility layer. Pure Go, no CGo.
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

// run loops trying to establish a peak-detect stream, reconnecting on failure.
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

// monitor connects to PulseAudio, finds the target sink-input, and runs a
// peak-detect recording stream until an error or quit.
func (m *pulseMonitor) monitor() error {
	client, err := pulse.NewClient(
		pulse.ClientApplicationName("gomud-monitor"),
	)
	if err != nil {
		return fmt.Errorf("pulse connect: %w", err)
	}
	defer client.Close()

	sinkInputIdx, sinkIdx, err := m.findTarget(client)
	if err != nil {
		return err
	}

	var sinkInfo proto.GetSinkInfoReply
	err = client.RawRequest(&proto.GetSinkInfo{SinkIndex: sinkIdx, SinkName: ""}, &sinkInfo)
	if err != nil {
		return fmt.Errorf("get sink info: %w", err)
	}

	slog.Debug("setting up peak monitor",
		"sink_input", sinkInputIdx,
		"sink", sinkInfo.SinkName,
		"monitor_source_index", sinkInfo.MonitorSourceIndex,
	)

	writer := &peakWriter{peak: &m.peak}

	stream, err := client.NewRecord(
		writer,
		pulse.RecordSampleRate(25),
		pulse.RecordMono,
		pulse.RecordRawOption(func(r *proto.CreateRecordStream) {
			r.SourceIndex = sinkInfo.MonitorSourceIndex
			r.PeakDetect = true
			r.DirectOnInputIndex = sinkInputIdx
			r.AdjustLatency = true
		}),
	)
	if err != nil {
		return fmt.Errorf("create record stream: %w", err)
	}
	defer stream.Close()

	stream.Start()
	m.active.Store(true)
	slog.Info("pulse peak monitor active", "target", m.target)

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

// findTarget locates the sink-input whose application.process.binary contains
// the target string (case-insensitive).
func (m *pulseMonitor) findTarget(client *pulse.Client) (sinkInputIdx, sinkIdx uint32, err error) {
	var reply proto.GetSinkInputInfoListReply
	err = client.RawRequest(&proto.GetSinkInputInfoList{}, &reply)
	if err != nil {
		return 0, 0, fmt.Errorf("list sink-inputs: %w", err)
	}

	for _, si := range reply {
		binary := ""
		if prop, ok := si.Properties["application.process.binary"]; ok {
			binary = prop.String()
		}
		if strings.Contains(strings.ToLower(binary), m.target) {
			slog.Debug("found target sink-input",
				"index", si.SinkInputIndex,
				"binary", binary,
				"sink_index", si.SinkIndex,
			)
			return si.SinkInputIndex, si.SinkIndex, nil
		}
	}

	return 0, 0, fmt.Errorf("no sink-input found for %q", m.target)
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
