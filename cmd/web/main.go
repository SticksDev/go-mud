package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

//go:embed static
var staticFiles embed.FS

type message struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type hub struct {
	mu       sync.Mutex
	mudConn  *websocket.Conn
	uiConns  []*websocket.Conn
	nextID   int
	mudReady bool
}

var h hub

func (h *hub) setMud(c *websocket.Conn) {
	h.mu.Lock()
	h.mudConn = c
	h.mudReady = true
	h.mu.Unlock()
}

func (h *hub) clearMud() {
	h.mu.Lock()
	h.mudConn = nil
	h.mudReady = false
	h.mu.Unlock()
}

func (h *hub) addUI(c *websocket.Conn) {
	h.mu.Lock()
	h.uiConns = append(h.uiConns, c)
	h.mu.Unlock()
}

func (h *hub) removeUI(c *websocket.Conn) {
	h.mu.Lock()
	for i, cl := range h.uiConns {
		if cl == c {
			h.uiConns = append(h.uiConns[:i], h.uiConns[i+1:]...)
			break
		}
	}
	h.mu.Unlock()
}

func (h *hub) sendToMud(ctx context.Context, msg message) error {
	h.mu.Lock()
	conn := h.mudConn
	h.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("no go-mud client connected")
	}
	return wsjson.Write(ctx, conn, msg)
}

func (h *hub) broadcastToUI(ctx context.Context, msg message) {
	h.mu.Lock()
	snapshot := make([]*websocket.Conn, len(h.uiConns))
	copy(snapshot, h.uiConns)
	h.mu.Unlock()
	for _, c := range snapshot {
		wsjson.Write(ctx, c, msg)
	}
}

func handleMudWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Error("accept mud ws", "error", err)
		return
	}
	defer conn.CloseNow()

	h.setMud(conn)
	defer h.clearMud()

	slog.Info("go-mud client connected", "remote", r.RemoteAddr)

	notify, _ := json.Marshal(map[string]string{"event": "go-mud connected"})
	h.broadcastToUI(r.Context(), message{Type: "info", Data: notify})

	ctx := r.Context()
	for {
		var msg message
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			slog.Info("go-mud disconnected", "error", err)
			notify, _ := json.Marshal(map[string]string{"event": "go-mud disconnected"})
			h.broadcastToUI(context.Background(), message{Type: "info", Data: notify})
			return
		}
		slog.Info("from go-mud", "type", msg.Type, "id", msg.ID)
		h.broadcastToUI(ctx, msg)
	}
}

func handleUIWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("accept ui ws", "error", err)
		return
	}
	defer conn.CloseNow()

	h.addUI(conn)
	defer h.removeUI(conn)

	h.mu.Lock()
	ready := h.mudReady
	h.mu.Unlock()
	if ready {
		notify, _ := json.Marshal(map[string]string{"event": "go-mud connected"})
		wsjson.Write(r.Context(), conn, message{Type: "info", Data: notify})
	}

	ctx := r.Context()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	commands := r.FormValue("commands")
	if commands == "" {
		http.Error(w, "commands required", http.StatusBadRequest)
		return
	}

	var cmdList []string
	if err := json.Unmarshal([]byte(commands), &cmdList); err != nil {
		for _, part := range strings.Split(commands, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				cmdList = append(cmdList, trimmed)
			}
		}
	}

	h.mu.Lock()
	h.nextID++
	id := fmt.Sprintf("req-%d", h.nextID)
	h.mu.Unlock()

	data, _ := json.Marshal(map[string]any{"commands": cmdList})
	msg := message{Type: "execute", ID: id, Data: data}

	if err := h.sendToMud(r.Context(), msg); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	slog.Info("sent execute", "id", id, "commands", cmdList)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "sent"})
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	id := r.FormValue("id")
	if err := h.sendToMud(r.Context(), message{Type: "cancel", ID: id}); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancel sent"})
}

func main() {
	addr := ":8765"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	staticFS, _ := fs.Sub(staticFiles, "static")

	http.HandleFunc("/ws", handleMudWS)
	http.HandleFunc("/ui-ws", handleUIWS)
	http.HandleFunc("/send", handleSend)
	http.HandleFunc("/cancel", handleCancel)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := staticFiles.ReadFile("static/index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	server := &http.Server{Addr: addr}
	go func() {
		<-ctx.Done()
		server.Close()
	}()

	slog.Info("test server starting", "addr", "http://localhost"+addr)
	slog.Info("go-mud connects to", "url", "ws://localhost"+addr+"/ws")
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
