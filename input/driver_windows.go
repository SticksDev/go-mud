//go:build windows

package input

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmKeydown = 0x0100
	wmKeyup   = 0x0101
	wmChar    = 0x0102
	vkReturn  = 0x0D
	vkEscape  = 0x1B
)

var (
	user32       = windows.NewLazySystemDLL("user32.dll")
	findWindowW  = user32.NewProc("FindWindowW")
	postMessageW = user32.NewProc("PostMessageW")
)

// WindowsDriver injects keystrokes via win32 PostMessage,
// which sends to the target window without requiring focus.
type WindowsDriver struct {
	windowTitle    string
	keystrokeDelay time.Duration
}

func NewDriver(windowTitle string, keystrokeDelay time.Duration) *WindowsDriver {
	return &WindowsDriver{
		windowTitle:    windowTitle,
		keystrokeDelay: keystrokeDelay,
	}
}

func (d *WindowsDriver) findWindow() (uintptr, error) {
	titlePtr, err := windows.UTF16PtrFromString(d.windowTitle)
	if err != nil {
		return 0, fmt.Errorf("utf16 conversion: %w", err)
	}
	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return 0, fmt.Errorf("window %q not found", d.windowTitle)
	}
	return hwnd, nil
}

func (d *WindowsDriver) postKey(hwnd uintptr, vk uintptr) error {
	ret, _, err := postMessageW.Call(hwnd, wmKeydown, vk, 0)
	if ret == 0 {
		return fmt.Errorf("PostMessage WM_KEYDOWN: %w", err)
	}
	ret, _, err = postMessageW.Call(hwnd, wmKeyup, vk, 0)
	if ret == 0 {
		return fmt.Errorf("PostMessage WM_KEYUP: %w", err)
	}
	return nil
}

func (d *WindowsDriver) postChar(hwnd uintptr, ch rune) error {
	ret, _, err := postMessageW.Call(hwnd, wmChar, uintptr(ch), 0)
	if ret == 0 {
		return fmt.Errorf("PostMessage WM_CHAR: %w", err)
	}
	return nil
}

func (d *WindowsDriver) SendText(text string) error {
	hwnd, err := d.findWindow()
	if err != nil {
		return err
	}
	for _, r := range text {
		if err := d.postChar(hwnd, r); err != nil {
			return fmt.Errorf("sending char %q: %w", string(r), err)
		}
		if d.keystrokeDelay > 0 {
			time.Sleep(d.keystrokeDelay)
		}
	}
	return nil
}

func (d *WindowsDriver) SendReturn() error {
	hwnd, err := d.findWindow()
	if err != nil {
		return err
	}
	return d.postKey(hwnd, vkReturn)
}

func (d *WindowsDriver) SendEscape() error {
	hwnd, err := d.findWindow()
	if err != nil {
		return err
	}
	return d.postKey(hwnd, vkEscape)
}

func (d *WindowsDriver) SendCommand(cmd string) error {
	if err := d.SendText(cmd); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	return d.SendReturn()
}

func (d *WindowsDriver) ClearAndSendCommand(cmd string) error {
	if err := d.SendEscape(); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return d.SendCommand(cmd)
}
