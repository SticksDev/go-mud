package shell

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Output represents parsed shell output from a flush.
type Output struct {
	Raw    string
	Clean  string
	ReadAt time.Time
}

// Reader handles reading hackmud's shell.txt file.
type Reader struct {
	shellPath string
}

// NewReader creates a Reader for the given shell.txt path.
func NewReader(shellPath string) *Reader {
	return &Reader{shellPath: shellPath}
}

// DefaultShellPath returns the platform-specific default path to shell.txt.
func DefaultShellPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "hackmud", "shell.txt")
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "hackmud", "shell.txt")
	default:
		return ""
	}
}

// Truncate empties shell.txt so the next flush only contains fresh output.
func (r *Reader) Truncate() {
	slog.Debug("truncating shell.txt")
	os.Truncate(r.shellPath, 0)
}

// Read reads the current contents of shell.txt with retry logic
// for file locking (the game may hold a write lock while flushing).
func (r *Reader) Read() (Output, error) {
	var lastErr error
	for i := range 5 {
		data, err := os.ReadFile(r.shellPath)
		if err == nil {
			raw := string(data)
			slog.Debug("shell.txt read", "bytes", len(data), "attempt", i+1)
			return Output{
				Raw:    raw,
				Clean:  StripColors(raw),
				ReadAt: time.Now(),
			}, nil
		}
		lastErr = err
		slog.Debug("shell.txt read failed, retrying", "attempt", i+1, "error", err)
		if i < 4 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return Output{}, fmt.Errorf("reading %s after 5 attempts: %w", r.shellPath, lastErr)
}
