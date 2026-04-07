package user

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

var pendingChats sync.Map

type chatState struct {
	mu         sync.Mutex
	subs       []chan string
	inbound    chan string
	done       chan struct{}
	srv        *http.Server
	lastSeenAt time.Time
}

func (c *chatState) subscribe() (chan string, func()) {
	ch := make(chan string, 16)
	c.mu.Lock()
	c.subs = append(c.subs, ch)
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		for i, s := range c.subs {
			if s == ch {
				c.subs = append(c.subs[:i], c.subs[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
	}
}

func (c *chatState) broadcast(msg string) {
	c.mu.Lock()
	for _, ch := range c.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	c.mu.Unlock()
}

func openChatHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if runtime.GOOS == "linux" {
		return mcp.NewToolResultError("open_chat is not supported on Linux"), nil
	}
	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	subtitle := req.GetString("subtitle", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return mcp.NewToolResultError("failed to open chat: " + err.Error()), nil
	}
	port := ln.Addr().(*net.TCPAddr).Port

	chatID := newDialogToken()
	state := &chatState{
		inbound: make(chan string, 64),
		done:    make(chan struct{}),
	}

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	state.srv = srv
	pendingChats.Store(chatID, state)

	page := buildChatHTML(title, subtitle)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		ch, unsub := state.subscribe()
		defer unsub()
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-state.done:
				fmt.Fprintf(w, "event: closed\ndata: closed\n\n")
				flusher.Flush()
				return
			case msg := <-ch:
				if strings.HasPrefix(msg, "__status:") {
					status := strings.TrimPrefix(msg, "__status:")
					fmt.Fprintf(w, "event: ai_status\ndata: %s\n\n", status)
				} else if strings.HasPrefix(msg, "__suggestions:") {
					data := strings.TrimPrefix(msg, "__suggestions:")
					fmt.Fprintf(w, "event: ai_suggestions\ndata: %s\n\n", data)
				} else {
					escaped := strings.ReplaceAll(msg, "\n", "\\n")
					fmt.Fprintf(w, "data: %s\n\n", escaped)
				}
				flusher.Flush()
			case <-time.After(25 * time.Second):
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")

		var payload struct {
			Text string `json:"text"`
		}
		msg := ""
		if err := json.Unmarshal(body, &payload); err == nil {
			msg = strings.TrimSpace(payload.Text)
		} else {
			msg = strings.TrimSpace(string(body))
		}
		if msg == "" {
			return
		}
		select {
		case state.inbound <- msg:
		default:
		}
	})

	mux.HandleFunc("/close", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		pendingChats.Delete(chatID)
		select {
		case <-state.done:
		default:
			close(state.done)
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			srv.Close()
		}()
	})

	mux.HandleFunc("/seen", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		state.mu.Lock()
		state.lastSeenAt = time.Now()
		state.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	go func() { _ = srv.Serve(ln) }()

	go sendNotification("Chat opened — check your browser", title, "info", 10)
	openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))

	return mcp.NewToolResultText(
		"Chat opened in browser.\n" +
			"chat_id: " + chatID + "\n" +
			"Use send_chat_message to send messages and get_chat_messages to receive replies.",
	), nil
}

func sendChatMessageHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	chatID := req.GetString("chat_id", "")
	if strings.TrimSpace(chatID) == "" {
		return mcp.NewToolResultError("chat_id is required"), nil
	}
	message := req.GetString("message", "")
	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	val, ok := pendingChats.Load(chatID)
	if !ok {
		return mcp.NewToolResultError("unknown or closed chat_id"), nil
	}
	state := val.(*chatState)

	select {
	case <-state.done:
		return mcp.NewToolResultError("chat has been closed"), nil
	default:
	}

	state.broadcast(message)

	chips := req.GetStringSlice("suggestions", nil)
	if len(chips) > 0 {
		var filtered []string
		for _, s := range chips {
			if strings.TrimSpace(s) != "" {
				filtered = append(filtered, strings.TrimSpace(s))
			}
		}
		if len(filtered) > 0 {
			b, _ := json.Marshal(filtered)
			state.broadcast("__suggestions:" + string(b))
		}
	}

	state.mu.Lock()
	seenAt := state.lastSeenAt
	state.mu.Unlock()

	seenNote := "user hasn't opened the chat yet"
	if !seenAt.IsZero() {
		seenNote = "user last active: " + seenAt.Format("15:04:05")
	}
	return mcp.NewToolResultText("message sent to chat (" + seenNote + ")"), nil
}

func getChatMessagesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	chatID := req.GetString("chat_id", "")
	if strings.TrimSpace(chatID) == "" {
		return mcp.NewToolResultError("chat_id is required"), nil
	}

	waitSec := req.GetFloat("wait_seconds", 55)
	if waitSec <= 0 {
		waitSec = 55
	}
	if waitSec > 115 {
		waitSec = 115
	}

	val, ok := pendingChats.Load(chatID)
	if !ok {
		return mcp.NewToolResultError("unknown or closed chat_id"), nil
	}
	state := val.(*chatState)

	var msgs []string
loop:
	for {
		select {
		case msg := <-state.inbound:
			msgs = append(msgs, msg)
		default:
			break loop
		}
	}
	if len(msgs) > 0 {
		state.broadcast("__status:read")
		return mcp.NewToolResultText(strings.Join(msgs, "\n---\n")), nil
	}

	state.broadcast("__status:waiting")
	defer state.broadcast("__status:idle")

	select {
	case msg := <-state.inbound:
		burst := []string{msg}
	drainLoop:
		for {
			select {
			case extra := <-state.inbound:
				burst = append(burst, extra)
			default:
				break drainLoop
			}
		}
		state.broadcast("__status:read")
		return mcp.NewToolResultText(strings.Join(burst, "\n---\n")), nil
	case <-state.done:
		return mcp.NewToolResultText("CLOSED — chat has been closed"), nil
	case <-time.After(time.Duration(waitSec) * time.Second):
		return mcp.NewToolResultText(
			"PENDING — no new messages yet. Call get_chat_messages(chat_id=\"" + chatID + "\") to keep waiting.",
		), nil
	}
}

func closeChatHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	chatID := req.GetString("chat_id", "")
	if strings.TrimSpace(chatID) == "" {
		return mcp.NewToolResultError("chat_id is required"), nil
	}

	val, ok := pendingChats.Load(chatID)
	if !ok {
		return mcp.NewToolResultError("unknown or already-closed chat_id"), nil
	}
	state := val.(*chatState)
	pendingChats.Delete(chatID)

	select {
	case <-state.done:
	default:
		close(state.done)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		state.srv.Close()
	}()

	return mcp.NewToolResultText("chat closed"), nil
}
