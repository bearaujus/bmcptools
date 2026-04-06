package main

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed assets/html/dialog.html
var dialogHTMLTemplate string

//go:embed assets/html/chat.html
var chatHTMLTemplate string

//go:embed assets/html/rest.html
var restHTMLTemplate string

//go:embed assets/html/md.css
var mdCSS string

//go:embed assets/html/md.js
var mdJS string

// askUserMu ensures at most one ask_user dialog is visible at a time.
// Concurrent calls queue and are served in arrival order.
var askUserMu sync.Mutex

// pendingDialogs stores state for non-blocking ask_user sessions.
// Key: token string → Value: *pendingDialogState
var pendingDialogs sync.Map

// dialogActivity tracks real-time user activity reported by the browser dialog's heartbeat.
// Only populated for macOS browser-based dialogs.
type dialogActivity struct {
	mu           sync.Mutex
	connected    bool          // browser has loaded the page at least once
	typing       bool          // user has non-empty text in the input
	idleSec      float64       // seconds since last browser interaction
	lastBeat     time.Time     // time of last /heartbeat POST
	outboundSubs []chan string  // live SSE subscriber channels (AI → browser)
}

func (a *dialogActivity) update(typing bool, idleSec float64) {
	a.mu.Lock()
	a.connected = true
	a.typing = typing
	a.idleSec = idleSec
	a.lastBeat = time.Now()
	a.mu.Unlock()
}

// subscribe registers a new SSE listener and returns the channel plus an unsubscribe func.
func (a *dialogActivity) subscribe() (chan string, func()) {
	ch := make(chan string, 8)
	a.mu.Lock()
	a.outboundSubs = append(a.outboundSubs, ch)
	a.mu.Unlock()
	return ch, func() {
		a.mu.Lock()
		for i, s := range a.outboundSubs {
			if s == ch {
				a.outboundSubs = append(a.outboundSubs[:i], a.outboundSubs[i+1:]...)
				break
			}
		}
		a.mu.Unlock()
	}
}

// broadcast delivers msg to every active SSE subscriber. Slow/full subscribers are skipped.
func (a *dialogActivity) broadcast(msg string) {
	a.mu.Lock()
	for _, ch := range a.outboundSubs {
		select {
		case ch <- msg:
		default:
		}
	}
	a.mu.Unlock()
}

// pendingDialogState holds everything needed to track a non-blocking dialog.
type pendingDialogState struct {
	responseCh chan string
	activity   *dialogActivity // non-nil only for macOS browser dialogs
}

func registerUserTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("notify_user",
			mcp.WithDescription(td("notify_user")),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("notify_user", "message")),
			),
			mcp.WithString("title",
				mcp.Description(pd("notify_user", "title")),
			),
			mcp.WithString("level",
				mcp.Description(pd("notify_user", "level")),
			),
			mcp.WithNumber("duration_seconds",
				mcp.Description(pd("notify_user", "duration_seconds")),
			),
		),
		notifyUserHandler,
	)

	s.AddTool(
		mcp.NewTool("ask_user",
			mcp.WithDescription(td("ask_user")),
			mcp.WithString("question",
				mcp.Required(),
				mcp.Description(pd("ask_user", "question")),
			),
			mcp.WithString("title",
				mcp.Description(pd("ask_user", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("ask_user", "subtitle")),
			),
			mcp.WithArray("choices",
				mcp.Description(pd("ask_user", "choices")),
				mcp.Items(map[string]any{"type": "string"}),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(pd("ask_user", "timeout_seconds")),
			),
			mcp.WithBoolean("allow_freeform",
				mcp.Description(pd("ask_user", "allow_freeform")),
			),
			mcp.WithBoolean("notify",
				mcp.Description(pd("ask_user", "notify")),
			),
			mcp.WithBoolean("non_blocking",
				mcp.Description(pd("ask_user", "non_blocking")),
			),
		),
		askUserHandler,
	)

	s.AddTool(
		mcp.NewTool("get_user_response",
			mcp.WithDescription(td("get_user_response")),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(pd("get_user_response", "token")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(pd("get_user_response", "wait_seconds")),
			),
		),
		getUserResponseHandler,
	)

	s.AddTool(
		mcp.NewTool("update_dialog",
			mcp.WithDescription(td("update_dialog")),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(pd("update_dialog", "token")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("update_dialog", "message")),
			),
		),
		updateDialogHandler,
	)

	s.AddTool(
		mcp.NewTool("open_chat",
			mcp.WithDescription(td("open_chat")),
			mcp.WithString("title",
				mcp.Description(pd("open_chat", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("open_chat", "subtitle")),
			),
		),
		openChatHandler,
	)

	s.AddTool(
		mcp.NewTool("send_chat_message",
			mcp.WithDescription(td("send_chat_message")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("send_chat_message", "chat_id")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("send_chat_message", "message")),
			),
		),
		sendChatMessageHandler,
	)

	s.AddTool(
		mcp.NewTool("get_chat_messages",
			mcp.WithDescription(td("get_chat_messages")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("get_chat_messages", "chat_id")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(pd("get_chat_messages", "wait_seconds")),
			),
		),
		getChatMessagesHandler,
	)

	s.AddTool(
		mcp.NewTool("close_chat",
			mcp.WithDescription(td("close_chat")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("close_chat", "chat_id")),
			),
		),
		closeChatHandler,
	)

	s.AddTool(
		mcp.NewTool("rest",
			mcp.WithDescription(td("rest")),
			mcp.WithString("notes",
				mcp.Description(pd("rest", "notes")),
			),
			mcp.WithString("title",
				mcp.Description(pd("rest", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("rest", "subtitle")),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(pd("rest", "timeout_seconds")),
			),
		),
		restHandler,
	)
}

func askUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question := req.GetString("question", "")
	if strings.TrimSpace(question) == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	subtitle := req.GetString("subtitle", "")

	choices := req.GetStringSlice("choices", nil)
	allowFreeform := req.GetBool("allow_freeform", true)

	timeoutSec := req.GetFloat("timeout_seconds", 600)
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	timeout := time.Duration(timeoutSec) * time.Second

	notify := req.GetBool("notify", true)
	nonBlocking := req.GetBool("non_blocking", false)

	// Non-blocking mode: open the dialog in a goroutine and return a poll token immediately.
	// This avoids MCP client request timeouts (typically 30–120 s) when waiting for user input.
	if nonBlocking {
		token := newDialogToken()
		act := &dialogActivity{} // activity tracking for macOS browser dialogs
		state := &pendingDialogState{
			responseCh: make(chan string, 1),
			activity:   act,
		}
		pendingDialogs.Store(token, state)

		go func() {
			answer := runDialogBlocking(question, title, subtitle, choices, allowFreeform, notify, timeout, act)
			select {
			case state.responseCh <- answer:
			default:
			}
			// Clean up after a grace period in case get_user_response is never called.
			// This prevents a leak while still allowing the caller to retrieve the answer.
			time.Sleep(5 * time.Minute)
			pendingDialogs.Delete(token)
		}()

		return mcp.NewToolResultText(
			"PENDING — dialog opened in background.\n" +
				"Token: " + token + "\n" +
				"Call get_user_response(token=\"" + token + "\") to retrieve the answer.\n" +
				"Each get_user_response call waits up to wait_seconds (default 55) before returning PENDING again.",
		), nil
	}

	// Blocking mode: serialize so at most one dialog is visible at a time.
	askUserMu.Lock()
	defer askUserMu.Unlock()

	return mcp.NewToolResultText(runDialogBlocking(question, title, subtitle, choices, allowFreeform, notify, timeout, nil)), nil
}

// runDialogBlocking drives the dialog loop and returns the user's answer (or a timeout message).
// activity is non-nil only for macOS non-blocking dialogs — it gets threaded into promptMacBrowser
// so the local HTTP server can populate it from JavaScript heartbeats.
func runDialogBlocking(question, title, subtitle string, choices []string, allowFreeform, notify bool, timeout time.Duration, activity *dialogActivity) string {
	if notify {
		msg := question
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		go sendNotification(msg, title, "info", 10)
	}

	deadline := time.Now().Add(timeout)

	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "[User did not respond within the allotted time]"
		}

		var answer string
		var err error
		if len(choices) > 0 {
			answer, err = promptUserChoice(question, title, subtitle, allowFreeform, choices, remaining, activity)
		} else {
			answer, err = promptUser(question, title, subtitle, remaining, activity)
		}
		if err != nil {
			return fmt.Sprintf("[Failed to get user input: %v]", err)
		}

		if strings.TrimSpace(answer) != "" {
			return answer
		}

		if attempt < maxRetries-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return "[User did not respond within the allotted time]"
}

// getUserResponseHandler polls for the result of a non-blocking ask_user dialog.
func getUserResponseHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}

	waitSec := req.GetFloat("wait_seconds", 55)
	if waitSec <= 0 {
		waitSec = 55
	}
	if waitSec > 115 {
		waitSec = 115 // stay comfortably under the typical 120 s MCP client timeout
	}

	val, ok := pendingDialogs.Load(token)
	if !ok {
		return mcp.NewToolResultError(
			"unknown or expired token — the dialog may have already been answered or timed out",
		), nil
	}
	state := val.(*pendingDialogState)

	select {
	case answer := <-state.responseCh:
		pendingDialogs.Delete(token)
		return mcp.NewToolResultText(answer), nil
	case <-time.After(time.Duration(waitSec) * time.Second):
		return mcp.NewToolResultText(buildPendingMessage(token, state.activity)), nil
	}
}

// buildPendingMessage composes a status-aware PENDING message for get_user_response.
// When activity tracking is available (macOS browser), it reports the user's real-time state
// so the AI can decide whether to keep waiting or remind the user.
func buildPendingMessage(token string, act *dialogActivity) string {
	callAgain := fmt.Sprintf("Call get_user_response(token=%q) to continue waiting.", token)

	if act == nil {
		return "PENDING — user has not responded yet.\n" + callAgain
	}

	act.mu.Lock()
	connected := act.connected
	typing := act.typing
	idleSec := act.idleSec
	lastBeat := act.lastBeat
	act.mu.Unlock()

	if !connected {
		return "PENDING — dialog has been opened. Waiting for user to load the page.\n" + callAgain
	}

	beatAge := time.Since(lastBeat).Seconds()
	if beatAge > 20 {
		return fmt.Sprintf(
			"PENDING — browser connection appears lost (last heartbeat was %.0fs ago). "+
				"The user may have closed the tab. "+
				"Consider calling notify_user to re-alert them, then %s",
			beatAge, callAgain,
		)
	}

	if typing {
		return fmt.Sprintf("PENDING — user is actively composing a reply. %s", callAgain)
	}

	if idleSec > 90 {
		return fmt.Sprintf(
			"PENDING — user has been idle for %.0fs (dialog open but no interaction). "+
				"Consider calling notify_user to remind them, then %s",
			idleSec, callAgain,
		)
	}

	return fmt.Sprintf(
		"PENDING — user has the dialog open (last activity %.0fs ago). %s",
		idleSec, callAgain,
	)
}

// newDialogToken generates a random hex token for a pending dialog session.
func newDialogToken() string {
	b := make([]byte, 8)
	if _, err := cryptorand.Read(b); err != nil {
		// Fall back to a time-based token on the rare read failure.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%016x", b)
}

// updateDialogHandler broadcasts a live message into an open non-blocking ask_user dialog.
func updateDialogHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}
	message := req.GetString("message", "")
	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	val, ok := pendingDialogs.Load(token)
	if !ok {
		return mcp.NewToolResultError("unknown or expired token — dialog may have been answered or timed out"), nil
	}
	state := val.(*pendingDialogState)
	if state.activity == nil {
		return mcp.NewToolResultError("this dialog does not support live updates"), nil
	}

	state.activity.broadcast(message)
	return mcp.NewToolResultText("message delivered to dialog"), nil
}

// ── persistent chat ───────────────────────────────────────────────────────────

// pendingChats stores active chat sessions.
// Key: chat_id string → Value: *chatState
var pendingChats sync.Map

// chatState holds the server and channels for a persistent two-way chat session.
type chatState struct {
	mu         sync.Mutex
	subs       []chan string // SSE subscribers for AI→browser messages
	inbound    chan string   // user→AI message queue
	done       chan struct{} // closed by close_chat to signal shutdown
	srv        *http.Server // the local HTTP server, for clean shutdown
	lastSeenAt time.Time    // when the user last had the chat tab visible
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
	if runtime.GOOS != "darwin" {
		return mcp.NewToolResultError("open_chat is only supported on macOS"), nil
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

	// SSE: AI→browser
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

	// POST: user→AI
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

	// POST /close: user-initiated close from the browser
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

	// POST /seen: browser notifies that user has the chat tab visible and can see AI messages.
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
	_ = exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d/", port)).Run()

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

	// Drain all currently queued messages first (non-blocking).
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

	// Nothing queued — signal the browser that the AI is waiting, then block.
	state.broadcast("__status:waiting")
	defer state.broadcast("__status:idle")

	// Wait up to waitSec for the first new message, draining any burst on arrival.
	select {
	case msg := <-state.inbound:
		// Drain any additional messages that arrived simultaneously.
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

	// Signal done (broadcast to browser + all goroutines) then shut server down.
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

// ── notify_user ───────────────────────────────────────────────────────────────

func notifyUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message := req.GetString("message", "")
	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}
	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	level := req.GetString("level", "info")
	if level != "warning" && level != "error" {
		level = "info"
	}
	durationSec := req.GetFloat("duration_seconds", 5)
	if durationSec <= 0 {
		durationSec = 5
	}
	if durationSec > 60 {
		durationSec = 60
	}

	go sendNotification(message, title, level, int(durationSec))
	fmt.Fprintf(os.Stderr, "\n[AI NOTIFY][%s] %s: %s\n", strings.ToUpper(level), title, message)

	return mcp.NewToolResultText(fmt.Sprintf("[Notification sent] %s", message)), nil
}

// sendNotification dispatches to the OS-appropriate non-blocking notification.
// On failure it falls back to writing to stderr so the message is never silently lost.
func sendNotification(message, title, level string, durationSec int) {
	var delivered bool
	switch runtime.GOOS {
	case "windows":
		sendNotificationWindows(message, title, level, durationSec)
		delivered = true
	case "darwin":
		// Use proper AppleScript quoting and add a sound for visibility.
		// Level-appropriate subtitle (⚠ WARNING / 🔴 ERROR) helps at-a-glance triage.
		subtitle := map[string]string{
			"warning": "⚠ WARNING",
			"error":   "🔴 ERROR",
		}[level]
		var script string
		if subtitle != "" {
			script = fmt.Sprintf(
				`display notification %s with title %s subtitle %s sound name "default"`,
				macASQuote(message), macASQuote(title), macASQuote(subtitle),
			)
		} else {
			script = fmt.Sprintf(
				`display notification %s with title %s sound name "default"`,
				macASQuote(message), macASQuote(title),
			)
		}
		if exec.Command("osascript", "-e", script).Run() == nil {
			delivered = true
			break
		}
		// Fallback: terminal-notifier (brew install terminal-notifier).
		// Has its own notification permissions, often works when osascript is blocked.
		args := []string{"-message", message, "-title", title, "-sound", "default"}
		if subtitle != "" {
			args = append(args, "-subtitle", subtitle)
		}
		delivered = exec.Command("terminal-notifier", args...).Run() == nil
	default:
		urgency := "low"
		switch level {
		case "warning":
			urgency = "normal"
		case "error":
			urgency = "critical"
		}
		expireMs := durationSec * 1000
		delivered = exec.Command("notify-send",
			fmt.Sprintf("--expire-time=%d", expireMs),
			"--urgency="+urgency, title, message,
		).Run() == nil
	}
	if !delivered {
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", strings.ToUpper(level), title, message)
	}
}

// ── free-form prompt ──────────────────────────────────────────────────────────

func promptUser(question, title, subtitle string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return promptWindows(question, title, timeout)
	case "darwin":
		return promptMac(question, title, subtitle, timeout, activity)
	default:
		return promptLinux(question, title, timeout)
	}
}

// macASQuote wraps s in an AppleScript string literal, safely escaping
// double-quotes and newlines so any text can be embedded in a script.
func macASQuote(s string) string {
	s = strings.ReplaceAll(s, `"`, `" & quote & "`)
	s = strings.ReplaceAll(s, "\r\n", `" & return & "`)
	s = strings.ReplaceAll(s, "\n", `" & return & "`)
	s = strings.ReplaceAll(s, "\r", `" & return & "`)
	return `"` + s + `"`
}

// promptMac shows a browser-based dialog on macOS with a full multi-line textarea.
func promptMac(question, title, subtitle string, timeout time.Duration, activity *dialogActivity) (string, error) {
	return promptMacBrowser(question, title, subtitle, true, nil, timeout, activity)
}

func promptWindows(question, title string, timeout time.Duration) (string, error) {
	script := buildWPFInputScript(
		sanitizePSHereString(question),
		sanitizePSHereString(title),
		int(timeout.Seconds()),
	)
	result, err := runPSTempFile(script, timeout)
	if err != nil {
		return promptConsole(question)
	}
	return result, nil
}

func promptLinux(question, title string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "zenity", "--entry", "--title="+title, "--text="+question)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	cmd = exec.CommandContext(ctx, "kdialog", "--inputbox", question, "--title", title)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	cmd = exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf(`whiptail --inputbox %q 8 72 2>&1 >/dev/tty`, question))
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	return promptConsole(question)
}

// ── multiple-choice prompt ────────────────────────────────────────────────────

func promptUserChoice(question, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return promptChoiceWindows(question, title, choices, timeout)
	case "darwin":
		return promptChoiceMac(question, title, subtitle, allowFreeform, choices, timeout, activity)
	default:
		return promptChoiceLinux(question, title, choices, timeout)
	}
}

func promptChoiceWindows(question, title string, choices []string, timeout time.Duration) (string, error) {
	choicesJSON, err := json.Marshal(choices)
	if err != nil {
		return promptChoiceConsole(question, title, choices)
	}
	script := buildWPFChoiceScript(
		sanitizePSHereString(question),
		sanitizePSHereString(title),
		string(choicesJSON),
		int(timeout.Seconds()),
	)
	result, err := runPSTempFile(script, timeout)
	if err != nil {
		return promptChoiceConsole(question, title, choices)
	}
	return result, nil
}

// promptChoiceMac shows choices as chips in a browser-based macOS dialog.
func promptChoiceMac(question, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	return promptMacBrowser(question, title, subtitle, allowFreeform, choices, timeout, activity)
}

// promptMacBrowser serves a local HTML dialog in the user's default browser.
// It provides a full multi-line textarea and optional clickable choice chips.
// activity, when non-nil, is updated from JavaScript heartbeat POSTs so callers
// can report the user's real-time state (typing, idle, browser closed).
func promptMacBrowser(question, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	timeoutSec := int(timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 600
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if len(choices) > 0 {
			return promptChoiceConsole(question, title, choices)
		}
		return promptConsole(question)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	resultCh := make(chan string, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	page := buildMacDialogHTML(question, title, subtitle, allowFreeform, choices, timeoutSec)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})
	mux.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")

		// Parse JSON body: {"choice":"...","notes":"..."}
		var payload struct {
			Choice string `json:"choice"`
			Notes  string `json:"notes"`
		}
		answer := ""
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
			switch {
			case payload.Choice != "" && payload.Notes != "":
				answer = payload.Choice + "\n\n" + payload.Notes
			case payload.Choice != "":
				answer = payload.Choice
			default:
				answer = payload.Notes
			}
		} else {
			answer = string(body)
		}

		select {
		case resultCh <- answer:
		default:
		}
	})

	// Heartbeat endpoint: the dialog HTML POSTs activity status every 5 s.
	// This lets get_user_response report whether the user is typing, idle, or gone.
	if activity != nil {
		mux.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var payload struct {
				Typing      bool    `json:"typing"`
				IdleSeconds float64 `json:"idle_seconds"`
			}
			body, _ := io.ReadAll(io.LimitReader(r.Body, 512))
			_ = json.Unmarshal(body, &payload)
			w.WriteHeader(http.StatusOK)
			activity.update(payload.Typing, payload.IdleSeconds)
		})

		// SSE endpoint: pushes AI→dialog messages from update_dialog in real time.
		// The browser opens an EventSource to this URL and renders messages in the dialog.
		mux.HandleFunc("/updates", func(w http.ResponseWriter, r *http.Request) {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher.Flush()

			ch, unsub := activity.subscribe()
			defer unsub()

			ctx := r.Context()
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-ch:
					// SSE lines may not contain bare newlines; escape them.
					escaped := strings.ReplaceAll(msg, "\n", "\\n")
					fmt.Fprintf(w, "data: %s\n\n", escaped)
					flusher.Flush()
				case <-time.After(25 * time.Second):
					// Keep-alive comment prevents proxy/browser timeouts.
					fmt.Fprintf(w, ": keepalive\n\n")
					flusher.Flush()
				}
			}
		})
	}

	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	go sendNotification("Your input is needed — check your browser", title, "info", 10)
	_ = exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d/", port)).Run()

	select {
	case answer := <-resultCh:
		return answer, nil
	case <-time.After(timeout + 2*time.Second):
		return "", nil
	}
}

// chipHTML returns the HTML for a chip label: escapes HTML and converts newlines to <br>.
func chipHTML(c string) string {
	return strings.ReplaceAll(html.EscapeString(c), "\n", "<br>")
}

// buildMacDialogHTML builds the HTML page shown in the browser dialog.
func buildMacDialogHTML(question, title, subtitle string, allowFreeform bool, choices []string, timeoutSec int) string {
	chipsSection := ""
	if len(choices) > 0 {
		var sb strings.Builder
		sb.WriteString(`<div class="suggested-label">Suggested replies</div><div class="chips-row">`)
		for i, c := range choices {
			jC, _ := json.Marshal(c)
			sb.WriteString(fmt.Sprintf(
				`<button class="chip" id="chip%d" onclick="pickChip(%s,%d)">%s</button>`,
				i, html.EscapeString(string(jC)), i, chipHTML(c),
			))
		}
		sb.WriteString(`</div>`)
		chipsSection = sb.String()
	}

	allowFreeformVal := "true"
	if !allowFreeform {
		allowFreeformVal = "false"
	}

	page := strings.ReplaceAll(dialogHTMLTemplate, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[QUESTION]]", html.EscapeString(question))
	page = strings.ReplaceAll(page, "[[CHIPS_SECTION]]", chipsSection)
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	page = strings.ReplaceAll(page, "[[ALLOW_FREEFORM]]", allowFreeformVal)
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
	return page
}


// buildChatHTML returns the HTML for the persistent two-way chat window.
func buildChatHTML(title, subtitle string) string {
	page := strings.ReplaceAll(chatHTMLTemplate, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
	return page
}

// buildRestHTML returns the HTML for the AI-resting page.
func buildRestHTML(title, subtitle, notes string) string {
	page := strings.ReplaceAll(restHTMLTemplate, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[NOTES]]", html.EscapeString(notes))
	return page
}

// restHandler opens a browser "AI is resting" page and returns a poll token.
// The user can press "Wake me up!" (with an optional note) to signal the AI.
// The existing get_user_response tool polls for the wakeup.
func restHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if runtime.GOOS != "darwin" {
		return mcp.NewToolResultError("rest is only supported on macOS"), nil
	}

	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	subtitle := req.GetString("subtitle", "")
	notes := req.GetString("notes", "")

	timeoutSec := req.GetFloat("timeout_seconds", 3600)
	if timeoutSec <= 0 {
		timeoutSec = 3600
	}
	if timeoutSec > 86400 {
		timeoutSec = 86400
	}
	timeout := time.Duration(timeoutSec) * time.Second

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return mcp.NewToolResultError("failed to open rest page: " + err.Error()), nil
	}
	port := ln.Addr().(*net.TCPAddr).Port

	token := newDialogToken()
	act := &dialogActivity{}
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	pendingDialogs.Store(token, state)

	resultCh := make(chan string, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	page := buildRestHTML(title, subtitle, notes)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
		act.mu.Lock()
		act.connected = true
		act.mu.Unlock()
	})
	mux.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")

		var payload struct {
			Notes string `json:"notes"`
		}
		wakeMsg := "User woke up the AI."
		if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Notes) != "" {
			wakeMsg = "User woke up the AI with note: " + strings.TrimSpace(payload.Notes)
		}
		select {
		case resultCh <- wakeMsg:
		default:
		}
	})

	go func() { _ = srv.Serve(ln) }()

	// Feed result into the pending dialog state so get_user_response can pick it up.
	go func() {
		var answer string
		select {
		case answer = <-resultCh:
		case <-time.After(timeout):
			answer = "[Rest timed out — user did not wake the AI]"
		}
		select {
		case state.responseCh <- answer:
		default:
		}
		srv.Close()
		// Grace period before removing the token so get_user_response can retrieve it.
		time.Sleep(5 * time.Minute)
		pendingDialogs.Delete(token)
	}()

	go sendNotification(title+" is now resting", title, "info", 10)
	_ = exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d/", port)).Run()

	return mcp.NewToolResultText(
		"AI is now resting. Browser page opened for the user.\n" +
			"Token: " + token + "\n" +
			"Call get_user_response(token=\"" + token + "\") to wait for the user to wake you up.",
	), nil
}

func promptChoiceLinux(question, title string, choices []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"--list", "--title=" + title, "--text=" + question, "--column=Options", "--hide-header"}
	args = append(args, choices...)
	cmd := exec.CommandContext(ctx, "zenity", args...)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	args = []string{"--menu", question, "--title", title}
	for i, c := range choices {
		args = append(args, fmt.Sprintf("%d", i+1), c)
	}
	cmd = exec.CommandContext(ctx, "kdialog", args...)
	if out, err := cmd.Output(); err == nil {
		result := strings.TrimRight(string(out), "\r\n")
		for i := range choices {
			if result == fmt.Sprintf("%d", i+1) {
				return choices[i], nil
			}
		}
		return result, nil
	}

	return promptChoiceConsole(question, title, choices)
}

func promptChoiceConsole(question, title string, choices []string) (string, error) {
	printConsolePromptHeader(title)
	fmt.Fprintf(os.Stderr, "%s\n\nChoices:\n", question)
	for i, c := range choices {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, c)
	}
	fmt.Fprintf(os.Stderr, "\nEnter number or text: ")

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err != nil {
		return "", fmt.Errorf("cannot open console (%s): %w", ttyPath, err)
	}
	defer tty.Close()

	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		for i, c := range choices {
			if input == fmt.Sprintf("%d", i+1) {
				return c, nil
			}
		}
		return input, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func printConsolePromptHeader(title string) {
	const (
		borderCols = 50
		label      = "  AI ASSISTANT QUESTION"
	)
	border := strings.Repeat("═", borderCols)
	padding := strings.Repeat(" ", borderCols-len(label))
	fmt.Fprintf(os.Stderr, "\n╔%s╗\n║%s%s║\n╚%s╝\n", border, label, padding, border)
	if title != "" && title != "AI Assistant" {
		fmt.Fprintf(os.Stderr, "[%s]\n", title)
	}
}

func promptConsole(question string) (string, error) {
	printConsolePromptHeader("AI Assistant")
	fmt.Fprintf(os.Stderr, "%s\n\nYour answer: ", question)
	os.Stderr.Sync()

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err == nil {
		defer tty.Close()
		scanner := bufio.NewScanner(tty)
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", nil
	}

	// /dev/tty not available - this is expected when running as MCP server
	// Return empty string to trigger retry loop, which will eventually timeout
	fmt.Fprintf(os.Stderr, "\n[ask_user] Unable to access terminal. Please check your MCP client configuration.\n")
	os.Stderr.Sync()
	return "", nil
}

// ── Windows WPF notification / dialog helpers ─────────────────────────────────

// wpfNotifyScriptTmpl is a PowerShell WPF script that renders a themed toast-style
// popup in the bottom-right corner.  Placeholders: {{ACCENT}}, {{TITLE}}, {{MESSAGE}}.
const wpfNotifyScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$fg      = if ($isDark) { '#E0E0E0' } else { '#1A1A1A' }
$titleFg = if ($isDark) { '#FFFFFF' } else { '#000000' }
$accent  = '{{ACCENT}}'
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        AllowsTransparency="True" WindowStyle="None"
        SizeToContent="WidthAndHeight" Topmost="True"
        Background="Transparent" ShowInTaskbar="False" Opacity="0">
  <Border Background="$bg" CornerRadius="8"
          BorderBrush="$accent" BorderThickness="0,0,0,3">
    <Grid Width="320">
      <Grid.ColumnDefinitions>
        <ColumnDefinition Width="4"/>
        <ColumnDefinition Width="*"/>
      </Grid.ColumnDefinitions>
      <Border Grid.Column="0" Background="$accent" CornerRadius="8,0,0,8"/>
      <StackPanel Grid.Column="1" Margin="14,12,14,12">
        <TextBlock Name="TitleBlock" FontSize="13" FontWeight="SemiBold"
                   FontFamily="Segoe UI" Foreground="$titleFg" Margin="0,0,0,3"/>
        <TextBlock Name="MsgBlock" FontSize="12" FontFamily="Segoe UI"
                   Foreground="$fg" TextWrapping="Wrap"/>
      </StackPanel>
    </Grid>
  </Border>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.FindName('TitleBlock').Text = $titleVal.Trim()
$msgVal = @'
{{MESSAGE}}
'@
$window.FindName('MsgBlock').Text = $msgVal.Trim()
$window.Add_MouseLeftButtonDown({ $window.Close() })
$script:toastTimer = $null
$window.Add_ContentRendered({
    $screen = [System.Windows.SystemParameters]::WorkArea
    $window.Left = $screen.Right  - $window.ActualWidth  - 20
    $window.Top  = $screen.Bottom - $window.ActualHeight - 20
    $fadeIn = [System.Windows.Media.Animation.DoubleAnimation]::new(0, 1,
        [System.Windows.Duration]::new([TimeSpan]::FromMilliseconds(250)))
    $window.BeginAnimation([System.Windows.UIElement]::OpacityProperty, $fadeIn)
    $script:toastTimer = [System.Windows.Threading.DispatcherTimer]::new()
    $script:toastTimer.Interval = [TimeSpan]::FromSeconds({{DURATION_SEC}})
    $script:toastTimer.Add_Tick({
        $script:toastTimer.Stop()
        $fadeOut = [System.Windows.Media.Animation.DoubleAnimation]::new(1, 0,
            [System.Windows.Duration]::new([TimeSpan]::FromMilliseconds(350)))
        $fadeOut.Add_Completed({ $window.Close() })
        $window.BeginAnimation([System.Windows.UIElement]::OpacityProperty, $fadeOut)
    })
    $script:toastTimer.Start()
})
$window.ShowDialog() | Out-Null
`

// wpfInputScriptTmpl is a PowerShell WPF free-form input dialog.
// Features: mandatory input (Send disabled until text entered), multi-line TextBox
// (Enter = send, Shift+Enter = new line), resizable window, scrollable question,
// countdown timer, FlashWindowEx taskbar alert, and bring-to-front on open.
// Placeholders: {{TITLE}}, {{QUESTION}}, {{TIMEOUT_SEC}}.
const wpfInputScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
try {
    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public class WinFlash {
    [DllImport("user32.dll")]
    public static extern bool FlashWindowEx(ref FLASHWINFO pwfi);
    [StructLayout(LayoutKind.Sequential)]
    public struct FLASHWINFO {
        public uint cbSize;
        public IntPtr hwnd;
        public uint dwFlags;
        public uint uCount;
        public uint dwTimeout;
    }
    public static void Flash(IntPtr hwnd) {
        FLASHWINFO fw = new FLASHWINFO();
        fw.cbSize = (uint)System.Runtime.InteropServices.Marshal.SizeOf(fw);
        fw.hwnd = hwnd;
        fw.dwFlags = 3;
        fw.uCount = 5;
        fw.dwTimeout = 0;
        FlashWindowEx(ref fw);
    }
}
"@ -ErrorAction Stop
} catch {}
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#1E1E1E' } else { '#F9F9F9' }
$fg      = if ($isDark) { '#E8E8E8' } else { '#1A1A1A' }
$inputBg = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$border  = if ($isDark) { '#444444' } else { '#CCCCCC' }
$subFg   = if ($isDark) { '#888888' } else { '#999999' }
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        MinWidth="420" Width="520" MinHeight="260" Height="380"
        WindowStartupLocation="CenterScreen"
        ResizeMode="CanResizeWithGrip" Background="$bg"
        ShowInTaskbar="True">
  <Grid Margin="24,20,24,20">
    <Grid.RowDefinitions>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="12"/>
      <RowDefinition Height="*"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="6"/>
      <RowDefinition Height="Auto"/>
    </Grid.RowDefinitions>
    <ScrollViewer Grid.Row="0" MaxHeight="140" VerticalScrollBarVisibility="Auto"
                  HorizontalScrollBarVisibility="Disabled">
      <TextBlock Name="QuestionBlock" Foreground="$fg"
                 TextWrapping="Wrap" FontSize="13" FontFamily="Segoe UI"/>
    </ScrollViewer>
    <Grid Grid.Row="2">
      <TextBox Name="InputBox"
               Background="$inputBg" Foreground="$fg"
               BorderBrush="$border" BorderThickness="1"
               Padding="10,8" FontSize="13" FontFamily="Segoe UI"
               CaretBrush="$fg" AcceptsReturn="True"
               TextWrapping="Wrap" VerticalScrollBarVisibility="Auto"
               VerticalAlignment="Stretch"/>
      <TextBlock Name="PlaceholderBlock"
                 Text="Type your answer here...  (Shift+Enter for new line)"
                 Foreground="$subFg" FontSize="13" FontFamily="Segoe UI"
                 Padding="12,10,0,0" IsHitTestVisible="False"
                 VerticalAlignment="Top" HorizontalAlignment="Left"/>
    </Grid>
    <TextBlock Name="CountdownBlock" Grid.Row="4"
               Foreground="$subFg" FontSize="11" FontFamily="Segoe UI"
               HorizontalAlignment="Left"/>
    <StackPanel Grid.Row="6" Orientation="Horizontal" HorizontalAlignment="Right">
      <Button Name="CancelBtn" Content="Cancel" Width="90" Height="32" Margin="0,0,8,0"
              Background="$border" Foreground="$fg" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
      <Button Name="OkBtn" Content="Send" Width="90" Height="32"
              Background="#0078D4" Foreground="#FFFFFF" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" IsEnabled="False" Cursor="Hand"/>
    </StackPanel>
  </Grid>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.Title = $titleVal.Trim()
$questionVal = @'
{{QUESTION}}
'@
$window.FindName('QuestionBlock').Text = $questionVal.Trim()
$inputBox       = $window.FindName('InputBox')
$okBtn          = $window.FindName('OkBtn')
$cancelBtn      = $window.FindName('CancelBtn')
$placeholder    = $window.FindName('PlaceholderBlock')
$countdownBlock = $window.FindName('CountdownBlock')
$script:dialogResult = [string]::Empty

$inputBox.Add_TextChanged({
    $hasText = $inputBox.Text.Length -gt 0
    $placeholder.Visibility = if ($hasText) { 'Hidden' } else { 'Visible' }
    $okBtn.IsEnabled = $hasText
})

$script:doSubmit = {
    if ($inputBox.Text.Trim().Length -gt 0) {
        $script:dialogResult = $inputBox.Text
        $window.DialogResult = $true
    }
}

$okBtn.Add_Click({ & $script:doSubmit })
$cancelBtn.Add_Click({ $window.DialogResult = $false })

$inputBox.Add_PreviewKeyDown({
    param($s, $e)
    $shiftHeld = [System.Windows.Input.Keyboard]::IsKeyDown([System.Windows.Input.Key]::LeftShift) -or
                 [System.Windows.Input.Keyboard]::IsKeyDown([System.Windows.Input.Key]::RightShift)
    if ($e.Key -eq [System.Windows.Input.Key]::Return -and -not $shiftHeld) {
        $e.Handled = $true
        & $script:doSubmit
    }
    if ($e.Key -eq [System.Windows.Input.Key]::Escape) {
        $e.Handled = $true
        $window.DialogResult = $false
    }
})

$script:remainingSec = {{TIMEOUT_SEC}}
$script:countdownTimer = [System.Windows.Threading.DispatcherTimer]::new()
$script:countdownTimer.Interval = [TimeSpan]::FromSeconds(1)
$script:countdownTimer.Add_Tick({
    $script:remainingSec--
    if ($script:remainingSec -le 0) {
        $script:countdownTimer.Stop()
        $window.Close()
    } else {
        $mins = [int]($script:remainingSec / 60)
        $secs = $script:remainingSec % 60
        $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    }
})

$window.Add_ContentRendered({
    try {
        $hwnd = (New-Object System.Windows.Interop.WindowInteropHelper($window)).Handle
        [WinFlash]::Flash($hwnd)
    } catch {}
    $window.Topmost = $true
    $window.Activate()
    $window.Topmost = $false
    $inputBox.Focus() | Out-Null
    $mins = [int]($script:remainingSec / 60)
    $secs = $script:remainingSec % 60
    $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    $script:countdownTimer.Start()
})
$window.ShowDialog() | Out-Null
Write-Output $script:dialogResult
`

// wpfChoiceScriptTmpl is a PowerShell WPF list-picker dialog with live search,
// countdown timer, FlashWindowEx taskbar alert, and bring-to-front on open.
// Placeholders: {{TITLE}}, {{QUESTION}}, {{CHOICES_JSON}}, {{TIMEOUT_SEC}}.
const wpfChoiceScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
try {
    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public class WinFlash {
    [DllImport("user32.dll")]
    public static extern bool FlashWindowEx(ref FLASHWINFO pwfi);
    [StructLayout(LayoutKind.Sequential)]
    public struct FLASHWINFO {
        public uint cbSize;
        public IntPtr hwnd;
        public uint dwFlags;
        public uint uCount;
        public uint dwTimeout;
    }
    public static void Flash(IntPtr hwnd) {
        FLASHWINFO fw = new FLASHWINFO();
        fw.cbSize = (uint)System.Runtime.InteropServices.Marshal.SizeOf(fw);
        fw.hwnd = hwnd;
        fw.dwFlags = 3;
        fw.uCount = 5;
        fw.dwTimeout = 0;
        FlashWindowEx(ref fw);
    }
}
"@ -ErrorAction Stop
} catch {}
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#1E1E1E' } else { '#F9F9F9' }
$fg      = if ($isDark) { '#E8E8E8' } else { '#1A1A1A' }
$inputBg = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$border  = if ($isDark) { '#444444' } else { '#CCCCCC' }
$subFg   = if ($isDark) { '#888888' } else { '#999999' }
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        Width="460" Height="460"
        MinWidth="380" MinHeight="340"
        WindowStartupLocation="CenterScreen"
        ResizeMode="CanResizeWithGrip" Background="$bg"
        ShowInTaskbar="True">
  <Grid Margin="24,20,24,20">
    <Grid.RowDefinitions>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="8"/>
      <RowDefinition Height="*"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="6"/>
      <RowDefinition Height="Auto"/>
    </Grid.RowDefinitions>
    <ScrollViewer Grid.Row="0" MaxHeight="120" VerticalScrollBarVisibility="Auto"
                  HorizontalScrollBarVisibility="Disabled">
      <TextBlock Name="QuestionBlock" Foreground="$fg"
                 TextWrapping="Wrap" FontSize="13" FontFamily="Segoe UI"/>
    </ScrollViewer>
    <TextBox Name="SearchBox" Grid.Row="2"
             Background="$inputBg" Foreground="$fg"
             BorderBrush="$border" BorderThickness="1"
             Padding="8,6" FontSize="12" FontFamily="Segoe UI"
             CaretBrush="$fg"/>
    <ListBox Name="ChoiceList" Grid.Row="4"
             Background="$inputBg" Foreground="$fg"
             BorderBrush="$border" BorderThickness="1"
             FontSize="13" FontFamily="Segoe UI"
             ScrollViewer.VerticalScrollBarVisibility="Auto">
      <ListBox.ItemContainerStyle>
        <Style TargetType="ListBoxItem">
          <Setter Property="Padding" Value="10,7"/>
          <Setter Property="Foreground" Value="$fg"/>
          <Style.Triggers>
            <Trigger Property="IsSelected" Value="True">
              <Setter Property="Background" Value="#0078D4"/>
              <Setter Property="Foreground" Value="#FFFFFF"/>
            </Trigger>
          </Style.Triggers>
        </Style>
      </ListBox.ItemContainerStyle>
    </ListBox>
    <TextBlock Name="CountdownBlock" Grid.Row="6"
               Foreground="$subFg" FontSize="11" FontFamily="Segoe UI"
               HorizontalAlignment="Left"/>
    <StackPanel Grid.Row="8" Orientation="Horizontal" HorizontalAlignment="Right">
      <Button Name="CancelBtn" Content="Cancel" Width="90" Height="32" Margin="0,0,8,0"
              Background="$border" Foreground="$fg" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
      <Button Name="SelectBtn" Content="Select" Width="90" Height="32"
              Background="#0078D4" Foreground="#FFFFFF" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
    </StackPanel>
  </Grid>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.Title = $titleVal.Trim()
$questionVal = @'
{{QUESTION}}
'@
$window.FindName('QuestionBlock').Text = $questionVal.Trim()
$choicesJson = @'
{{CHOICES_JSON}}
'@
$listBox        = $window.FindName('ChoiceList')
$searchBox      = $window.FindName('SearchBox')
$selectBtn      = $window.FindName('SelectBtn')
$cancelBtn      = $window.FindName('CancelBtn')
$countdownBlock = $window.FindName('CountdownBlock')
$choicesArr = $choicesJson | ConvertFrom-Json
foreach ($c in $choicesArr) { $listBox.Items.Add($c) | Out-Null }
$script:dialogResult = [string]::Empty

$searchBox.Add_TextChanged({
    $filter = $searchBox.Text
    $listBox.Items.Clear()
    foreach ($c in $choicesArr) {
        if ($filter -eq '' -or $c -like "*$filter*") {
            $listBox.Items.Add($c) | Out-Null
        }
    }
    if ($listBox.Items.Count -gt 0) { $listBox.SelectedIndex = 0 }
})

$script:doSelect = {
    if ($null -ne $listBox.SelectedItem) {
        $script:dialogResult = $listBox.SelectedItem
        $window.DialogResult = $true
    }
}

$selectBtn.Add_Click({ & $script:doSelect })
$listBox.Add_MouseDoubleClick({ & $script:doSelect })
$cancelBtn.Add_Click({ $window.DialogResult = $false })

$window.Add_KeyDown({
    param($s, $e)
    if ($e.Key -eq [System.Windows.Input.Key]::Return) { & $script:doSelect }
    if ($e.Key -eq [System.Windows.Input.Key]::Escape)  { $window.DialogResult = $false }
    if ($e.Key -eq [System.Windows.Input.Key]::Down)    { $listBox.Focus() | Out-Null }
})

$script:remainingSec = {{TIMEOUT_SEC}}
$script:countdownTimer = [System.Windows.Threading.DispatcherTimer]::new()
$script:countdownTimer.Interval = [TimeSpan]::FromSeconds(1)
$script:countdownTimer.Add_Tick({
    $script:remainingSec--
    if ($script:remainingSec -le 0) {
        $script:countdownTimer.Stop()
        $window.Close()
    } else {
        $mins = [int]($script:remainingSec / 60)
        $secs = $script:remainingSec % 60
        $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    }
})

$window.Add_ContentRendered({
    try {
        $hwnd = (New-Object System.Windows.Interop.WindowInteropHelper($window)).Handle
        [WinFlash]::Flash($hwnd)
    } catch {}
    $window.Topmost = $true
    $window.Activate()
    $window.Topmost = $false
    $searchBox.Focus() | Out-Null
    $mins = [int]($script:remainingSec / 60)
    $secs = $script:remainingSec % 60
    $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    $script:countdownTimer.Start()
})
$window.ShowDialog() | Out-Null
Write-Output $script:dialogResult
`

// sanitizePSHereString prevents a value from accidentally closing a PS @'...'@ here-string.
func sanitizePSHereString(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "'@" || trimmed == `"@` {
			lines[i] = line + " "
		}
	}
	return strings.Join(lines, "\n")
}

// runPSTempFile writes a PowerShell script to a temp .ps1 file, executes it, and returns stdout.
func runPSTempFile(script string, timeout time.Duration) (string, error) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return "", fmt.Errorf("create temp ps1: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		return "", fmt.Errorf("write temp ps1: %w", err)
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass", "-File", name,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// runPSScriptBg writes a PowerShell script to a temp file and runs it synchronously
// (intended to be called from a goroutine for fire-and-forget behaviour).
func runPSScriptBg(script string) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return
	}
	name := f.Name()
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		os.Remove(name)
		return
	}
	f.Close()
	cmd := exec.Command("powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass", "-File", name,
	)
	_ = cmd.Run()
	os.Remove(name)
}

// sendNotificationWindows shows a WPF toast-style popup anchored to the bottom-right
// corner of the screen.  It fades in, stays for durationSec seconds, then fades out.
func sendNotificationWindows(message, title, level string, durationSec int) {
	accents := map[string]string{
		"info":    "#0078D4",
		"warning": "#F7A600",
		"error":   "#D13438",
	}
	accent, ok := accents[level]
	if !ok {
		accent = "#0078D4"
	}
	script := wpfNotifyScriptTmpl
	script = strings.ReplaceAll(script, "{{ACCENT}}", accent)
	script = strings.ReplaceAll(script, "{{TITLE}}", sanitizePSHereString(title))
	script = strings.ReplaceAll(script, "{{MESSAGE}}", sanitizePSHereString(message))
	script = strings.ReplaceAll(script, "{{DURATION_SEC}}", fmt.Sprintf("%d", durationSec))
	runPSScriptBg(script)
}

// buildWPFInputScript returns a fully substituted PowerShell WPF free-form input script.
func buildWPFInputScript(safeQuestion, safeTitle string, timeoutSec int) string {
	s := wpfInputScriptTmpl
	s = strings.ReplaceAll(s, "{{TITLE}}", safeTitle)
	s = strings.ReplaceAll(s, "{{QUESTION}}", safeQuestion)
	s = strings.ReplaceAll(s, "{{TIMEOUT_SEC}}", fmt.Sprintf("%d", timeoutSec))
	return s
}

// buildWPFChoiceScript returns a fully substituted PowerShell WPF list-picker script.
func buildWPFChoiceScript(safeQuestion, safeTitle, choicesJSON string, timeoutSec int) string {
	s := wpfChoiceScriptTmpl
	s = strings.ReplaceAll(s, "{{TITLE}}", safeTitle)
	s = strings.ReplaceAll(s, "{{QUESTION}}", safeQuestion)
	s = strings.ReplaceAll(s, "{{CHOICES_JSON}}", choicesJSON)
	s = strings.ReplaceAll(s, "{{TIMEOUT_SEC}}", fmt.Sprintf("%d", timeoutSec))
	return s
}
