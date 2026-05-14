package main

import (
	"flag"
	"time"

	"gomud/shell"
)

type Config struct {
	WebSocketURL       string
	ShellPath          string
	WindowTitle        string
	IdleTimeout        time.Duration
	KeystrokeDelay     time.Duration
	PostFlushDelay     time.Duration
	MinCommandDelay    time.Duration
	SilenceSettleDelay time.Duration
	AudioGracePeriod   time.Duration
	ReconnectMax       time.Duration
}

func parseConfig() Config {
	cfg := Config{}

	flag.StringVar(&cfg.WebSocketURL, "ws-url", "", "WebSocket server URL to connect to (required)")
	flag.StringVar(&cfg.ShellPath, "shell-path", shell.DefaultShellPath(), "Path to hackmud shell.txt")
	flag.StringVar(&cfg.WindowTitle, "window-title", "hackmud", "Hackmud window title to find")
	flag.DurationVar(&cfg.IdleTimeout, "idle-timeout", 15*time.Second, "Max time to wait for command output")
	flag.DurationVar(&cfg.KeystrokeDelay, "keystroke-delay", 15*time.Millisecond, "Delay between individual keystrokes")
	flag.DurationVar(&cfg.PostFlushDelay, "post-flush-delay", 200*time.Millisecond, "Delay after flush before reading shell.txt")
	flag.DurationVar(&cfg.MinCommandDelay, "min-command-delay", 300*time.Millisecond, "Min wait for silent commands with no audio")
	flag.DurationVar(&cfg.SilenceSettleDelay, "silence-settle", 150*time.Millisecond, "Delay after silence detected before flushing")
	flag.DurationVar(&cfg.AudioGracePeriod, "audio-grace", 2*time.Second, "How long to wait for audio to start before assuming silent command")
	flag.DurationVar(&cfg.ReconnectMax, "reconnect-max", 30*time.Second, "Max reconnect backoff delay")

	flag.Parse()
	return cfg
}
