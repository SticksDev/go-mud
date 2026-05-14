# go-mud

A hackmud automation client. Sends commands to the game by injecting keystrokes into the window, detects when output is ready by monitoring the game's audio, and captures results via the `flush` command. Controlled over WebSocket with an included web UI.

## How it works

```
WebSocket server  <-->  go-mud  <-->  hackmud window
                          |
                     audio monitor
                     (silence = done)
```

1. A command arrives over WebSocket
2. go-mud sends `clear` to the game (wipes the terminal)
3. Types the command and presses Enter
4. Monitors the game's audio -- hackmud plays sound while processing
5. When audio goes silent, sends `flush` to write output to `shell.txt`
6. Reads `shell.txt`, and then sends the results back over WebSocket

## Quick start

```bash
# Start the web UI (port 8765)
go run ./cmd/web

# In another terminal, start the automation client
go run . -ws-url ws://localhost:8765/ws
```

Open `http://localhost:8765` in your browser. Type commands, see results (even with colors!)

## Building

```bash
go build -o go-mud .
go build -o go-mud-web ./cmd/web

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o go-mud .
```

Requires Go 1.25+ or higher. CGo is not required, but Windows users must have the Visual C++ Build Tools installed for the `golang.org/x/sys/windows` dependency.

## CLI flags

| Flag                 | Default          | Description                                 |
| -------------------- | ---------------- | ------------------------------------------- |
| `-ws-url`            | (required)       | WebSocket server URL to connect to          |
| `-window-title`      | `hackmud`        | Game window title to find                   |
| `-shell-path`        | platform default | Path to hackmud's `shell.txt`               |
| `-idle-timeout`      | `15s`            | Max wait for command output                 |
| `-keystroke-delay`   | `15ms`           | Delay between individual keystrokes         |
| `-post-flush-delay`  | `200ms`          | Wait after flush before reading `shell.txt` |
| `-min-command-delay` | `300ms`          | Fallback wait for commands with no audio    |
| `-silence-settle`    | `150ms`          | Wait after silence before flushing          |
| `-audio-grace`       | `2s`             | How long to wait for audio to appear        |
| `-reconnect-max`     | `30s`            | Max WebSocket reconnect backoff             |

Default `shell.txt` paths:

- Windows: `%APPDATA%\hackmud\shell.txt`
- Linux: `~/.config/hackmud/shell.txt`

## WebSocket protocol

go-mud connects as a client. All messages are JSON with `type`, `id`, and `data` fields.

### Sending commands

```json
{
    "type": "execute",
    "id": "req-1",
    "data": { "commands": ["accts.balance", "sys.status"] }
}
```

Commands run sequentially. Each gets its own `clear` / type / audio-wait / `flush` cycle.

### Responses

**Status** updates as execution progresses:

```json
{"type": "status", "id": "req-1", "data": {"state": "queued"}}
{"type": "status", "id": "req-1", "data": {"state": "executing"}}
{"type": "status", "id": "req-1", "data": {"state": "idle"}}
```

**Progress** for multi-command sequences:

```json
{
    "type": "progress",
    "id": "req-1",
    "data": { "current": 1, "total": 2, "command": "accts.balance" }
}
```

**Results** when all commands complete:

```json
{
    "type": "result",
    "id": "req-1",
    "data": {
        "results": [
            {
                "command": "accts.balance",
                "output": "Balance: 1K337GC",
                "raw_output": "<color=#1EFF00FF>Balance: 1K337GC</color>",
                "duration_ms": 1850
            }
        ]
    }
}
```

**Cancel** a running execution:

```json
{ "type": "cancel", "id": "req-1" }
```

## Web UI

The included web server (`cmd/web`) provides a browser interface:

- Send single or comma-separated commands
- Live status with queued/executing/done indicators
- Game output rendered with original hackmud colors (wow!)
- Per-command progress for multi-command sequences
- User switch detection with formatted banner
- JSON view toggle for raw response data

Run with `go run ./cmd/web` (default port 8765, or pass a custom address as the first argument).

## Audio monitoring

go-mud detects command completion by monitoring the game's audio output per-process.

**Windows**: Uses WASAPI to poll the hackmud process's audio session every 100ms. Checks the peak audio level to determine if sound is playing.

**Linux**: Polls `pactl` (PulseAudio, currently the only supported Linux audio server) every 100ms to find the hackmud process's audio stream and check its peak level.

When audio drops below threshold for 150ms, the command is considered done. If no audio session appears within the grace period (2s default), assumes the command produced no sound and falls back to a timed wait.

## Dependencies

- [github.com/coder/websocket](https://github.com/coder/websocket) -- WebSocket client/server
- [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) -- Windows syscalls for WASAPI and PostMessage
