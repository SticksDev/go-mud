//go:build linux

package input

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// LinuxDriver injects keystrokes via xdotool on X11.
// Uses type --window for all input (including Return) since xdotool key
// --window doesn't reliably deliver special keys to Electron apps via XSendEvent.
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
	return wid, nil
}

func (d *LinuxDriver) SendText(text string) error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	delayMs := fmt.Sprintf("%d", d.keystrokeDelay.Milliseconds())
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
	// Send Return via type (newline char) rather than key, which is more
	// reliable for Electron apps receiving XSendEvent.
	cmd := exec.Command("xdotool", "type", "--window", wid, "--clearmodifiers", "--", "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool type Return: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) SendEscape() error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	cmd := exec.Command("xdotool", "key", "--window", wid, "--clearmodifiers", "Escape")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool key Escape: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) SendCommand(cmdText string) error {
	wid, err := d.findWindow()
	if err != nil {
		return err
	}
	// Single xdotool call: text + newline to send Return in the same sequence.
	delayMs := fmt.Sprintf("%d", d.keystrokeDelay.Milliseconds())
	cmd := exec.Command("xdotool", "type", "--window", wid, "--clearmodifiers", "--delay", delayMs, "--", cmdText+"\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool type: %w: %s", err, out)
	}
	return nil
}

func (d *LinuxDriver) ClearAndSendCommand(cmdText string) error {
	if err := d.SendEscape(); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return d.SendCommand(cmdText)
}
