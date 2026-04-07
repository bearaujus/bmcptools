package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
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

func promptUser(question, title, subtitle string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return promptBrowser(question, title, subtitle, true, nil, timeout, activity)
	default:
		return "", fmt.Errorf("ask_user is not supported on Linux")
	}
}

func promptUserChoice(question, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return promptBrowser(question, title, subtitle, allowFreeform, choices, timeout, activity)
	default:
		return "", fmt.Errorf("ask_user is not supported on Linux")
	}
}

// openBrowser opens url in the system's default browser on macOS and Windows.
func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Run()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
	}
}

// promptBrowser serves a local HTML dialog in the user's default browser.
// activity, when non-nil, is updated from JavaScript heartbeat POSTs so callers
// can report the user's real-time state (typing, idle, browser closed).
func promptBrowser(question, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
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
					escaped := strings.ReplaceAll(msg, "\n", "\\n")
					fmt.Fprintf(w, "data: %s\n\n", escaped)
					flusher.Flush()
				case <-time.After(25 * time.Second):
					fmt.Fprintf(w, ": keepalive\n\n")
					flusher.Flush()
				}
			}
		})
	}

	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	go sendNotification("Your input is needed — check your browser", title, "info", 10)
	openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))

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
func buildRestHTML(title, subtitle, notes string, timeoutSec int) string {
	notesJSON, _ := json.Marshal(notes)
	page := strings.ReplaceAll(restHTMLTemplate, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[NOTES_ESCAPED]]", string(notesJSON))
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
	return page
}

// restHandler opens a browser "AI is resting" page and returns a poll token.
// The user can press "Wake me up!" to signal the AI via get_user_response.
func restHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if runtime.GOOS == "linux" {
		return mcp.NewToolResultError("rest is not supported on Linux"), nil
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

	page := buildRestHTML(title, subtitle, notes, int(timeoutSec))
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
		time.Sleep(5 * time.Minute)
		pendingDialogs.Delete(token)
	}()

	go sendNotification(title+" is now resting", title, "info", 10)
	openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))

	return mcp.NewToolResultText(
		"AI is now resting. Browser page opened for the user.\n" +
			"Token: " + token + "\n" +
			"Call get_user_response(token=\"" + token + "\") to wait for the user to wake you up.",
	), nil
}
