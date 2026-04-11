package user

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/browser"
)

// openBrowserFn delegates to browser.Open and can be overridden in tests
// to suppress real browser windows.
var openBrowserFn = browser.Open

func promptUser(ctx context.Context, htmlSource, question, details, title, subtitle string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return promptBrowser(ctx, htmlSource, question, details, title, subtitle, true, nil, timeout, activity)
	default:
		return "", fmt.Errorf("ask_user is not supported on Linux")
	}
}

func promptUserChoice(ctx context.Context, htmlSource, question, details, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return promptBrowser(ctx, htmlSource, question, details, title, subtitle, allowFreeform, choices, timeout, activity)
	default:
		return "", fmt.Errorf("ask_user is not supported on Linux")
	}
}

func promptBrowser(ctx context.Context, htmlSource, question, details, title, subtitle string, allowFreeform bool, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	timeoutSec := int(timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 600
	}

	resultCh := make(chan string, 1)
	mux := http.NewServeMux()

	page := buildDialogHTML(htmlSource, question, details, title, subtitle, allowFreeform, choices, timeoutSec)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
		if activity != nil {
			activity.mu.Lock()
			activity.connected = true
			activity.mu.Unlock()
		}
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
			Choice    string `json:"choice"`
			Notes     string `json:"notes"`
			Dismissed bool   `json:"dismissed"`
		}
		answer := ""
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
			if payload.Dismissed {
				answer = "[User dismissed the dialog — no reply was sent]"
			} else {
				switch {
				case payload.Choice != "" && payload.Notes != "":
					answer = payload.Choice + "\n\n" + payload.Notes
				case payload.Choice != "":
					answer = payload.Choice
				default:
					answer = payload.Notes
				}
			}
		} else {
			answer = string(body)
		}

		select {
		case resultCh <- answer:
		default:
		}
	})

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
					jsonMsg, _ := json.Marshal(msg)
					fmt.Fprintf(w, "data: %s\n\n", string(jsonMsg))
					flusher.Flush()
				case <-time.After(25 * time.Second):
					fmt.Fprintf(w, ": keepalive\n\n")
					flusher.Flush()
				}
			}
		})
	}

	port, shutdown, err := browser.Serve(mux)
	if err != nil {
		if len(choices) > 0 {
			return promptChoiceConsole(question, details, title, choices)
		}
		return promptConsole(question, details)
	}
	defer shutdown()

	openBrowserFn(fmt.Sprintf("http://127.0.0.1:%d/", port))

	select {
	case answer := <-resultCh:
		return answer, nil
	case <-ctx.Done():
		if activity != nil {
			activity.broadcast("__DISMISS__")
		}
		return "[Dialog cancelled by AI]", nil
	case <-time.After(timeout + 2*time.Second):
		// +2s grace window lets the browser's JS timer fire and POST /answer before
		// the server tears down. A blank string triggers a retry in runDialogBlocking.
		return "", nil
	}
}

func chipHTML(c string) string {
	return strings.ReplaceAll(html.EscapeString(c), "\n", "<br>")
}


// buildDialogHTML renders the ask_user dialog HTML template.
// htmlSource is the base template (default or custom override).
func buildDialogHTML(htmlSource, question, details, title, subtitle string, allowFreeform bool, choices []string, timeoutSec int) string {
	chipsSection := ""
	if len(choices) > 0 {
		var sb strings.Builder
		sb.WriteString(`<div class="chips-card"><div class="suggested-label">Suggested replies</div><div class="chips-row">`)
		for i, c := range choices {
			jC, _ := json.Marshal(c)
			sb.WriteString(fmt.Sprintf(
				`<button class="chip" id="chip%d" onclick="pickChip(%s,%d)">%s</button>`,
				i, html.EscapeString(string(jC)), i, chipHTML(c),
			))
		}
		sb.WriteString(`</div></div>`)
		chipsSection = sb.String()
	}

	allowFreeformVal := "true"
	if !allowFreeform {
		allowFreeformVal = "false"
	}


	detailsSection := ""
	detailsJSON := "null"
	if strings.TrimSpace(details) != "" {
		detailsSection = `<div class="details-card"><div class="details-body md-body" id="detailsBody"></div></div>`
		detailsJSONBytes, _ := json.Marshal(details)
		detailsJSON = string(detailsJSONBytes)
	}

	questionJSONBytes, _ := json.Marshal(question)
	questionJSON := string(questionJSONBytes)

	mdCSS := asset.CSS("md")
	mdJS := asset.JS("md")

	page := strings.ReplaceAll(htmlSource, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[QUESTION]]", html.EscapeString(question))
	page = strings.ReplaceAll(page, "[[QUESTION_JSON]]", questionJSON)
	page = strings.ReplaceAll(page, "[[DETAILS_SECTION]]", detailsSection)
	page = strings.ReplaceAll(page, "[[DETAILS_JSON]]", detailsJSON)
	page = strings.ReplaceAll(page, "[[CHIPS_SECTION]]", chipsSection)
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	page = strings.ReplaceAll(page, "[[ALLOW_FREEFORM]]", allowFreeformVal)
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
	return page
}

func buildRestHTML(htmlSource, title, subtitle, notes string, timeoutSec int) string {
	notesJSON, _ := json.Marshal(notes)
	mdCSS := asset.CSS("md")
	mdJS := asset.JS("md")
	page := strings.ReplaceAll(htmlSource, "[[TITLE]]", html.EscapeString(title))
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[NOTES_ESCAPED]]", string(notesJSON))
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
	return page
}

func makeRestHandler(htmlSource string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		token := newDialogToken()
		act := &dialogActivity{}
		state := &pendingDialogState{
			responseCh: make(chan string, 1),
			activity:   act,
		}
		storePendingDialog(token, state)

		resultCh := make(chan string, 1)
		mux := http.NewServeMux()

		page := buildRestHTML(htmlSource, title, subtitle, notes, int(timeoutSec))
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

		port, shutdown, err := browser.Serve(mux)
		if err != nil {
			return mcp.NewToolResultError("failed to open rest page: " + err.Error()), nil
		}

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
			shutdown()
			time.Sleep(5 * time.Minute)
			deletePendingDialog(token)
		}()

		go sendNotificationFn(title+" is now resting", title, "info", 10)
		openBrowserFn(fmt.Sprintf("http://127.0.0.1:%d/", port))

		return mcp.NewToolResultText(
			"{\n" +
				"  \"status\": \"RESTING\",\n" +
				"  \"token\": \"" + token + "\",\n" +
				"  \"instructions\": \"AI is now resting. Browser page opened for the user. Call get_user_response(token=\\\"" + token + "\\\") to wait for the user to wake you up.\"\n" +
				"}",
		), nil
	}
}
