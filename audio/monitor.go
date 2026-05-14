package audio

import (
	"context"
	"errors"
)

// ErrNoAudioSession is returned when the target process has no audio session,
// indicating the command produced no sound.
var ErrNoAudioSession = errors.New("no audio session found for target process")

// Monitor detects when a specific process transitions from active audio to silence.
type Monitor interface {
	// WaitForSilence blocks until the target process's audio goes from active
	// to silent, the context is cancelled, or the timeout expires.
	//
	// Returns nil when silence is detected.
	// Returns ErrNoAudioSession if no audio session was found within the grace period.
	// Returns context errors on cancellation/timeout.
	WaitForSilence(ctx context.Context) error

	// PeakLevel returns the current peak audio level (0.0-1.0) for the target
	// process, or -1 if no session is found.
	PeakLevel() (float32, error)

	// Close releases any resources held by the monitor.
	Close() error
}
