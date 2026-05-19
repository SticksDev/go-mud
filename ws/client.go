package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type executeJob struct {
	ctx context.Context
	msg InboundMessage
}

type Client struct {
	url          string
	executor     CommandExecutor
	reconnectMax time.Duration
	hello        HelloData

	mu       sync.Mutex
	conn     *websocket.Conn
	cancelFn context.CancelFunc

	queue chan executeJob
}

func NewClient(url string, exec CommandExecutor, reconnectMax time.Duration, hello HelloData) *Client {
	return &Client{
		url:          url,
		executor:     exec,
		reconnectMax: reconnectMax,
		hello:        hello,
		queue:        make(chan executeJob, 1024),
	}
}

// Run connects and processes messages. Reconnects on failure until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	go c.worker(ctx)

	delay := time.Second
	for {
		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Warn("websocket disconnected", "error", err)

		slog.Info("reconnecting", "delay", delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay = delay * 2
		delay = min(delay, c.reconnectMax)
	}
}

// worker processes execute jobs one at a time.
func (c *Client) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-c.queue:
			c.handleExecute(job.ctx, job.msg)
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	slog.Info("connected", "url", c.url)
	c.mu.Lock()
	hello := c.hello
	hello.ActiveJob = c.cancelFn != nil
	hello.QueuedJobs = len(c.queue)
	c.mu.Unlock()
	c.send(ctx, OutboundMessage{Type: "hello", Data: hello})
	c.send(ctx, OutboundMessage{Type: "status", Data: StatusData{State: "connected"}})

	for {
		var msg InboundMessage
		err := wsjson.Read(ctx, conn, &msg)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		slog.Debug("received message", "type", msg.Type, "id", msg.ID)

		switch msg.Type {
		case "execute":
			slog.Info("execute request received", "id", msg.ID, "queued", len(c.queue))
			c.send(ctx, OutboundMessage{
				Type: "status",
				ID:   msg.ID,
				Data: StatusData{State: "queued"},
			})
			c.queue <- executeJob{ctx: ctx, msg: msg}
		case "ping":
			c.send(ctx, OutboundMessage{Type: "pong", ID: msg.ID})
		case "cancel":
			slog.Info("cancel request received", "id", msg.ID)
			c.mu.Lock()
			if c.cancelFn != nil {
				c.cancelFn()
			}
			c.mu.Unlock()
		default:
			slog.Warn("unknown message type", "type", msg.Type)
		}
	}
}

func (c *Client) handleExecute(ctx context.Context, msg InboundMessage) {
	slog.Debug("handling execute job", "id", msg.ID)
	var req ExecuteRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		c.send(ctx, OutboundMessage{
			Type: "error",
			ID:   msg.ID,
			Data: ErrorData{Message: "invalid execute request: " + err.Error()},
		})
		return
	}

	if len(req.Commands) == 0 {
		c.send(ctx, OutboundMessage{
			Type: "error",
			ID:   msg.ID,
			Data: ErrorData{Message: "no commands provided"},
		})
		return
	}

	execCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancelFn = cancel
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		c.cancelFn = nil
		c.mu.Unlock()
	}()

	c.send(ctx, OutboundMessage{
		Type: "status",
		ID:   msg.ID,
		Data: StatusData{State: "executing"},
	})

	total := len(req.Commands)
	var results []Result
	err := c.executor.ExecuteSequence(execCtx, req.Commands, func(idx int, r Result) {
		results = append(results, r)
		c.send(ctx, OutboundMessage{
			Type: "progress",
			ID:   msg.ID,
			Data: ProgressData{Current: idx + 1, Total: total, Command: r.Command},
		})
	})

	if err != nil {
		c.send(ctx, OutboundMessage{
			Type: "error",
			ID:   msg.ID,
			Data: ErrorData{Message: err.Error()},
		})
		return
	}

	c.send(ctx, OutboundMessage{
		Type: "result",
		ID:   msg.ID,
		Data: ExecuteResultData{Results: results},
	})

	c.send(ctx, OutboundMessage{
		Type: "status",
		ID:   msg.ID,
		Data: StatusData{State: "idle"},
	})
}

func (c *Client) send(ctx context.Context, msg OutboundMessage) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		slog.Warn("cannot send, no connection", "type", msg.Type)
		return
	}

	if err := wsjson.Write(ctx, conn, msg); err != nil {
		slog.Warn("send failed", "type", msg.Type, "error", err)
	} else {
		slog.Debug("sent message", "type", msg.Type, "id", msg.ID)
	}
}
