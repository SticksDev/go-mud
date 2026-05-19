package main

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gomud/audio"
	"gomud/input"
	"gomud/shell"
	"gomud/ws"
)

// Executor orchestrates command execution against hackmud.
type Executor struct {
	input              input.Driver
	shell              *shell.Reader
	audioMonitor       audio.Monitor
	idleTimeout        time.Duration
	postFlushDelay     time.Duration
	minCommandDelay    time.Duration
	silenceSettleDelay time.Duration
}

type Option func(*Executor)

func WithIdleTimeout(d time.Duration) Option    { return func(e *Executor) { e.idleTimeout = d } }
func WithPostFlushDelay(d time.Duration) Option { return func(e *Executor) { e.postFlushDelay = d } }
func WithMinCommandDelay(d time.Duration) Option {
	return func(e *Executor) { e.minCommandDelay = d }
}
func WithSilenceSettleDelay(d time.Duration) Option {
	return func(e *Executor) { e.silenceSettleDelay = d }
}

func New(drv input.Driver, rdr *shell.Reader, mon audio.Monitor, opts ...Option) *Executor {
	e := &Executor{
		input:              drv,
		shell:              rdr,
		audioMonitor:       mon,
		idleTimeout:        15 * time.Second,
		postFlushDelay:     200 * time.Millisecond,
		minCommandDelay:    300 * time.Millisecond,
		silenceSettleDelay: 150 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// flush sends the flush command and waits for the file to be written.
func (e *Executor) flush() error {
	slog.Debug("flushing", "post_delay", e.postFlushDelay)
	if err := e.input.ClearAndSendCommand("flush"); err != nil {
		return err
	}
	time.Sleep(e.postFlushDelay)
	return nil
}

// Execute runs a single hackmud command and returns the result.
func (e *Executor) Execute(ctx context.Context, cmd string) ws.Result {
	start := time.Now()
	slog.Info("executing command", "command", cmd)

	// Clear the terminal so only this command's output will be on screen
	slog.Debug("sending clear")
	if err := e.input.ClearAndSendCommand("clear"); err != nil {
		slog.Error("clear failed", "command", cmd, "error", err)
		return ws.Result{Command: cmd, Error: "clear: " + err.Error(), DurationMs: ms(start)}
	}
	time.Sleep(100 * time.Millisecond)

	// Send the command (with Escape guard)
	slog.Debug("sending command to game", "command", cmd)
	if err := e.input.ClearAndSendCommand(cmd); err != nil {
		slog.Error("send command failed", "command", cmd, "error", err)
		return ws.Result{Command: cmd, Error: "send command: " + err.Error(), DurationMs: ms(start)}
	}

	// Wait for audio silence
	slog.Debug("waiting for audio silence", "timeout", e.idleTimeout)
	audioCtx, cancel := context.WithTimeout(ctx, e.idleTimeout)
	defer cancel()

	err := e.audioMonitor.WaitForSilence(audioCtx)

	if errors.Is(err, audio.ErrNoAudioSession) {
		slog.Debug("no audio session, using timed wait", "command", cmd, "delay", e.minCommandDelay)
		time.Sleep(e.minCommandDelay)
	} else if err != nil {
		slog.Warn("audio wait failed, flushing anyway", "command", cmd, "error", err)
		output, readErr := e.captureOutput(cmd)
		if readErr != nil {
			slog.Error("read after audio timeout failed", "command", cmd, "error", readErr)
			return ws.Result{Command: cmd, Error: "audio timeout + read error: " + readErr.Error(), TimedOut: true, DurationMs: ms(start)}
		}
		slog.Info("command completed (timed out)", "command", cmd, "duration_ms", ms(start), "output_len", len(output.Clean))
		return ws.Result{
			Command:    cmd,
			Output:     output.Clean,
			RawOutput:  output.Raw,
			TimedOut:   true,
			DurationMs: ms(start),
		}
	} else {
		slog.Debug("silence detected", "command", cmd)
	}

	// Silence detected (or no audio). Small settle delay then flush once.
	slog.Debug("settle delay before flush", "delay", e.silenceSettleDelay)
	time.Sleep(e.silenceSettleDelay)

	output, err := e.captureOutput(cmd)
	if err != nil {
		slog.Error("capture output failed", "command", cmd, "error", err)
		return ws.Result{Command: cmd, Error: "read after silence: " + err.Error(), DurationMs: ms(start)}
	}

	slog.Info("command completed", "command", cmd, "duration_ms", ms(start), "output_len", len(output.Clean))
	return ws.Result{
		Command:    cmd,
		Output:     output.Clean,
		RawOutput:  output.Raw,
		DurationMs: ms(start),
	}
}

// captureOutput truncates shell.txt, flushes, reads the result, and strips noise.
func (e *Executor) captureOutput(cmd string) (shell.Output, error) {
	slog.Debug("capturing output", "command", cmd)
	e.shell.Truncate()
	if err := e.flush(); err != nil {
		return shell.Output{}, err
	}
	out, err := e.shell.Read()
	if err != nil {
		return shell.Output{}, err
	}
	slog.Debug("raw output read", "command", cmd, "bytes", len(out.Raw))
	out.Clean = stripNoise(out.Clean, cmd)
	out.Raw = stripNoiseRaw(out.Raw, cmd)
	return out, nil
}

// ExecuteSequence runs commands in order, calling onResult after each.
func (e *Executor) ExecuteSequence(ctx context.Context, cmds []string, onResult func(idx int, r ws.Result)) error {
	slog.Info("starting sequence", "total", len(cmds))
	for i, cmd := range cmds {
		if ctx.Err() != nil {
			slog.Warn("sequence cancelled", "completed", i, "total", len(cmds))
			return ctx.Err()
		}
		result := e.Execute(ctx, cmd)
		onResult(i, result)
	}
	slog.Info("sequence complete", "total", len(cmds))
	return nil
}

// Patterns for stripping flush noise from output.
var (
	flushEchoRe       = regexp.MustCompile(`(?m)^>>flush\s*$`)
	flushSuccessRe    = regexp.MustCompile(`(?m)^Window contents have been written to disk successfully\.\s*$`)
	flushPathRe       = regexp.MustCompile(`(?m)^.*[/\\]shell\.txt\s*$`)
	flushEchoRawRe    = regexp.MustCompile(`(?m)^<color=[^>]*>>><color=[^>]*>flush</color></color>\s*$`)
	flushSuccessRawRe = regexp.MustCompile(`(?m)^<color=[^>]*>Window contents have been written to disk successfully\.</color>\s*$`)
	flushPathRawRe    = regexp.MustCompile(`(?m)^<color=[^>]*>.*[/\\]shell\.txt</color>\s*$`)
	rawPromptRe       = regexp.MustCompile(`(?m)^<color=[^>]*>>></color>\s*$`)
)

// stripNoise removes flush output, the command echo, and prompt lines.
func stripNoise(text string, cmd string) string {
	text = flushEchoRe.ReplaceAllString(text, "")
	text = flushSuccessRe.ReplaceAllString(text, "")
	text = flushPathRe.ReplaceAllString(text, "")

	cmdEcho := ">>" + cmd
	lines := strings.Split(text, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == cmdEcho || trimmed == ">>" {
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// stripNoiseRaw removes flush output from raw (color-tagged) text.
func stripNoiseRaw(text string, cmd string) string {
	text = flushEchoRawRe.ReplaceAllString(text, "")
	text = flushSuccessRawRe.ReplaceAllString(text, "")
	text = flushPathRawRe.ReplaceAllString(text, "")

	cmdEchoRawRe := regexp.MustCompile(`(?m)^<color=[^>]*>>>[^<]*` + regexp.QuoteMeta(cmd) + `[^<]*</color>\s*$`)
	text = cmdEchoRawRe.ReplaceAllString(text, "")
	text = rawPromptRe.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}

func ms(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
