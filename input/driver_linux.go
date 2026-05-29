//go:build linux

package input

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// LinuxDriver injects keystrokes via xdotool on X11.
// Text uses type --window (XSendEvent), special keys use key --window.
type LinuxDriver struct {
	windowTitle    string
	keystrokeDelay time.Duration
}

func NewDriver(windowTitle string, keystrokeDelay time.Duration) *LinuxDriver {
	return &LinuxDriver{
		windowTitle:    windowTitle,
		keystrokeDelay: keystrokeDelay,
	}
}

func (d *LinuxDriver) findWindow() (string, error) {
	out, err := exec.Command("xdotool", "search", "--name", d.windowTitle).Output()
	if err != nil {
		return "", fmt.Errorf("xdotool search for %q: %w", d.windowTitle, err)
	}
	wid := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	if wid == "" {
		return "", fmt.Errorf("window %q not found", d.windowTitle)
	}
	slog.Debug("found window", "title", d.windowTitle, "wid", wid)
	return wid, nil
}

func (d *LinuxDriver) SendText(text string) error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	delayMs := fmt.Sprintf("%d", d.keystrokeDelay.Milliseconds())
	slog.Debug("sending text", "len", len(text), "wid", wid)

	// xdotool type --window uses XSendEvent which can't reliably send
	// unicode characters that aren't in the X keymap. We split text into
	// ASCII runs (sent with "type" for speed) and non-ASCII characters
	// (sent individually with "key UXXXX" keysym format).
	var asciiRun strings.Builder
	for _, r := range text {
		if r >= 0x20 && r <= 0x7E {
			asciiRun.WriteRune(r)
			continue
		}
		// Flush any buffered ASCII text first.
		if asciiRun.Len() > 0 {
			if err := d.xdotoolType(wid, delayMs, asciiRun.String()); err != nil {
				return err
			}
			asciiRun.Reset()
		}
		// Send the non-ASCII character via key with unicode keysym.
		keysym := fmt.Sprintf("U%04X", r)
		cmd := exec.Command("xdotool", "key", "--window", wid, "--clearmodifiers", keysym)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("xdotool key %s: %w: %s", keysym, err, out)
		}
		if d.keystrokeDelay > 0 {
			time.Sleep(d.keystrokeDelay)
		}
	}
	// Flush remaining ASCII.
	if asciiRun.Len() > 0 {
		if err := d.xdotoolType(wid, delayMs, asciiRun.String()); err != nil {
			return err
		}
	}
	return nil
}

func (d *LinuxDriver) xdotoolType(wid, delayMs, text string) error {
	cmd := exec.Command("xdotool", "type", "--window", wid, "--clearmodifiers", "--delay", delayMs, "--", text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool type: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) SendReturn() error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	slog.Debug("sending Return", "wid", wid)
	cmd := exec.Command("xdotool", "key", "--window", wid, "--clearmodifiers", "Return")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool key Return: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) SendEscape() error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	slog.Debug("sending Escape", "wid", wid)
	cmd := exec.Command("xdotool", "key", "--window", wid, "--clearmodifiers", "Escape")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool key Escape: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) SendCommand(cmdText string) error {
	if err := d.SendText(cmdText); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	return d.SendReturn()
}

func (d *LinuxDriver) ClearAndSendCommand(cmdText string) error {
	if err := d.SendEscape(); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return d.SendCommand(cmdText)
}
