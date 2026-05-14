//go:build linux

package audio

import (
	"context"
	"encoding/json"
	"fmt"
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
		windowTitle:       windowTitle,
		pollInterval:      100 * time.Millisecond,
		gracePeriod:       gracePeriod,
		consecutiveNeeded: 3,
	}, nil
}

func (m *pactlMonitor) findTargetPID() (string, error) {
	out, err := exec.Command("xdotool", "search", "--name", m.windowTitle).Output()
	if err != nil {
		return "", fmt.Errorf("xdotool search: %w", err)
	}
	wid := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	if wid == "" {
		return "", fmt.Errorf("window %q not found", m.windowTitle)
	}

	pidOut, err := exec.Command("xdotool", "getwindowpid", wid).Output()
	if err != nil {
		return "", fmt.Errorf("xdotool getwindowpid: %w", err)
	}
	return strings.TrimSpace(string(pidOut)), nil
}

// getSinkInputState finds the sink-input for the given PID and returns its state.
// Returns ("", false, nil) if no sink-input is found for the PID.
func (m *pactlMonitor) getSinkInputState(pid string) (string, bool, error) {
	out, err := exec.Command("pactl", "--format=json", "list", "sink-inputs").Output()
	if err != nil {
		return "", false, fmt.Errorf("pactl list sink-inputs: %w", err)
	}

	var inputs []sinkInput
	if err := json.Unmarshal(out, &inputs); err != nil {
		return "", false, fmt.Errorf("parse pactl output: %w", err)
	}

	for _, si := range inputs {
		if si.Properties["application.process.id"] == pid {
			return si.State, true, nil
		}
	}
	return "", false, nil
}

func (m *pactlMonitor) WaitForSilence(ctx context.Context) error {
	pid, err := m.findTargetPID()
	if err != nil {
		return err
	}

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
			state, found, err := m.getSinkInputState(pid)
			if err != nil {
				continue
			}

			graceExpired := time.Now().After(graceDeadline)

			if !found {
				if audioSeen {
					return nil // session disappeared after audio was seen
				}
				if graceExpired {
					return ErrNoAudioSession
				}
				continue
			}

			isActive := state == "RUNNING"

			if isActive {
				audioSeen = true
				silentCount = 0
			} else if audioSeen {
				silentCount++
				if silentCount >= m.consecutiveNeeded {
					return nil
				}
			} else {
				if graceExpired {
					return ErrNoAudioSession
				}
			}
		}
	}
}

func (m *pactlMonitor) PeakLevel() (float32, error) {
	pid, err := m.findTargetPID()
	if err != nil {
		return -1, err
	}

	state, found, err := m.getSinkInputState(pid)
	if err != nil {
		return -1, err
	}
	if !found {
		return -1, nil
	}

	// pactl doesn't expose real-time peak levels; approximate from state
	if state == "RUNNING" {
		return 1.0, nil
	}
	return 0.0, nil
}

func (m *pactlMonitor) Close() error {
	return nil
}
