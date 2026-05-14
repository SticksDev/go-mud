package ws

import (
	"context"
	"encoding/json"
)

// Result is the outcome of executing a single command.
type Result struct {
	Command    string `json:"command"`
	Output     string `json:"output"`
	RawOutput  string `json:"raw_output,omitempty"`
	Error      string `json:"error,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// CommandExecutor runs hackmud commands and returns results.
type CommandExecutor interface {
	ExecuteSequence(ctx context.Context, cmds []string, onResult func(idx int, r Result)) error
}

// Inbound messages (server -> go-mud)

type InboundMessage struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type ExecuteRequest struct {
	Commands []string `json:"commands"`
}

// Outbound messages (go-mud -> server)

type OutboundMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Data any    `json:"data,omitempty"`
}

type ExecuteResultData struct {
	Results []Result `json:"results"`
}

type StatusData struct {
	State string `json:"state"`
}

type ErrorData struct {
	Message string `json:"message"`
}

type ProgressData struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Command string `json:"command"`
}
