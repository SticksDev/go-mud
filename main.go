package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"gomud/audio"
	"gomud/input"
	"gomud/shell"
	"gomud/ws"
)

func main() {
	cfg := parseConfig()

	if cfg.WebSocketURL == "" {
		fmt.Fprintln(os.Stderr, "error: -ws-url is required")
		os.Exit(1)
	}

	slog.Info("starting go-mud",
		"ws_url", cfg.WebSocketURL,
		"shell_path", cfg.ShellPath,
		"window_title", cfg.WindowTitle,
	)

	drv := input.NewDriver(cfg.WindowTitle, cfg.KeystrokeDelay)
	rdr := shell.NewReader(cfg.ShellPath)

	mon, err := audio.NewMonitor(cfg.WindowTitle, cfg.AudioGracePeriod)
	if err != nil {
		slog.Error("audio monitor required but failed to initialize", "error", err)
		os.Exit(1)
	}
	defer mon.Close()
	slog.Info("audio monitor enabled")

	exec := New(drv, rdr, mon,
		WithIdleTimeout(cfg.IdleTimeout),
		WithPostFlushDelay(cfg.PostFlushDelay),
		WithMinCommandDelay(cfg.MinCommandDelay),
		WithSilenceSettleDelay(cfg.SilenceSettleDelay),
	)
	client := ws.NewClient(cfg.WebSocketURL, exec, cfg.ReconnectMax)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := client.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}
