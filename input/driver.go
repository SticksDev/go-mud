package input

// Driver sends keystrokes to the hackmud game window.
type Driver interface {
	// SendText types a string into the hackmud window, character by character.
	SendText(text string) error

	// SendReturn presses the Enter key.
	SendReturn() error

	// SendEscape presses the Escape key (clears the input line in hackmud).
	SendEscape() error

	// SendCommand types text then presses Enter.
	SendCommand(cmd string) error

	// ClearAndSendCommand presses Escape to clear the input line,
	// then types text and presses Enter. Use this to avoid keystroke
	// concatenation when the input buffer may not be empty.
	ClearAndSendCommand(cmd string) error
}
