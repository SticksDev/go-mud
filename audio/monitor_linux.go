//go:build linux

package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

type sinkInput struct {
	Index      int               `json:"index"`
	State      string            `json:"state"`
	Properties map[string]string `json:"properties"`
}

// pactlMonitor implements Monitor using PulseAudio/PipeWire sink-input state.
type pactlMonitor struct {
	windowTitle       string
	pollInterval      time.Duration
	gracePeriod       time.Duration
	consecutiveNeeded int
}

// NewMonitor creates a pactl-based audio monitor.
func NewMonitor(windowTitle string, gracePeriod time.Duration) (Monitor, error) {
	if _, err := exec.LookPath("pactl"); err != nil {
		return nil, fmt.Errorf("pactl not found: %w", err)
	}
	return &pactlMonitor{
		windowTitle:       strings.ToLower(windowTitle),
		pollInterval:      100 * time.Millisecond,
		gracePeriod:       gracePeriod,
		consecutiveNeeded: 3,
	}, nil
}

// findSinkInput finds the sink-input whose binary name contains the window title.
// this is useful because the same process may create multiple sink-inputs (e.g. music and voice audio), and we want to monitor all of them together.
func (m *pactlMonitor) findSinkInput() (*sinkInput, error) {
	out, err := exec.Command("pactl", "--format=json", "list", "sink-inputs").Output()
	if err != nil {
		return nil, fmt.Errorf("pactl list sink-inputs: %w", err)
	}

	var inputs []sinkInput
	if err := json.Unmarshal(out, &inputs); err != nil {
		return nil, fmt.Errorf("parse pactl output: %w", err)
	}

	for i := range inputs {
		si := &inputs[i]
		binary := strings.ToLower(si.Properties["application.process.binary"])
		if strings.Contains(binary, m.windowTitle) {
			return si, nil
		}
	}
	return nil, nil
}

func (m *pactlMonitor) WaitForSilence(ctx context.Context) error {
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
			si, err := m.findSinkInput()
			if err != nil {
				slog.Debug("audio poll error", "error", err)
				continue
			}

			graceExpired := time.Now().After(graceDeadline)

			if si == nil {
				if audioSeen {
					slog.Debug("audio session disappeared after audio was seen")
					return nil
				}
				if graceExpired {
					slog.Debug("no audio session found within grace period")
					return ErrNoAudioSession
				}
				continue
			}

			isActive := si.State == "RUNNING"

			if isActive {
				if !audioSeen {
					slog.Debug("audio started",
						"binary", si.Properties["application.process.binary"],
						"pid", si.Properties["application.process.id"],
					)
				}
				audioSeen = true
				silentCount = 0
			} else if audioSeen {
				silentCount++
				slog.Debug("audio silent tick", "count", silentCount, "needed", m.consecutiveNeeded, "state", si.State)
				if silentCount >= m.consecutiveNeeded {
					slog.Debug("silence confirmed")
					return nil
				}
			} else {
				if graceExpired {
					slog.Debug("audio session found but never active, grace expired", "state", si.State)
					return ErrNoAudioSession
				}
			}
		}
	}
}

func (m *pactlMonitor) PeakLevel() (float32, error) {
	si, err := m.findSinkInput()
	if err != nil {
		return -1, err
	}
	if si == nil {
		return -1, nil
	}

	if si.State == "RUNNING" {
		return 1.0, nil
	}
	return 0.0, nil
}

func (m *pactlMonitor) Close() error {
	return nil
}
