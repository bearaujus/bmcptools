package user

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

type dialogAttachmentPayload struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
}

type dialogAttachmentFile struct {
	Name string
	MIME string
	Size int
	Path string
}

func promptUser(ctx context.Context, htmlSource, question, details, title, subtitle string, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
		return promptBrowser(ctx, htmlSource, question, details, title, subtitle, choices, timeout, activity)
	default:
		return "", fmt.Errorf("ask_user is not supported on Linux")
	}
}

func promptBrowser(ctx context.Context, htmlSource, question, details, title, subtitle string, choices []string, timeout time.Duration, activity *dialogActivity) (string, error) {
	timeoutSec := int(timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 600
	}

	resultCh := make(chan string, 1)
	mux := http.NewServeMux()

	page := buildDialogHTML(htmlSource, question, details, title, subtitle, choices, timeoutSec)
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
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")

		var payload struct {
			Choice      string                    `json:"choice"`
			Notes       string                    `json:"notes"`
			Dismissed   bool                      `json:"dismissed"`
			Attachments []dialogAttachmentPayload `json:"attachments"`
		}
		answer := ""
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
			if payload.Dismissed {
				answer = "[User dismissed the dialog — no reply was sent]"
			} else {
				files, saveErr := saveDialogAttachments(payload.Attachments)
				answer = formatDialogAnswer(payload.Choice, payload.Notes, files)
				if saveErr != nil {
					if strings.TrimSpace(answer) != "" {
						answer += "\n\n"
					}
					answer += "[Some attached images could not be saved: " + saveErr.Error() + "]"
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
				case evt := <-ch:
					if evt.Type == "" {
						evt.Type = dialogEventUpdate
					}
					jsonMsg, _ := json.Marshal(evt)
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
			activity.broadcastDismiss()
		}
		return "[Dialog cancelled by AI]", nil
	case <-time.After(timeout + 2*time.Second):
		// +2s grace window lets the browser's JS timer fire and POST /answer before
		// the server tears down. A blank string triggers a retry in runDialogBlocking.
		return "", nil
	}
}

func formatDialogAnswer(choice, notes string, attachments []dialogAttachmentFile) string {
	var parts []string
	switch {
	case choice != "" && notes != "":
		parts = append(parts, choice+"\n\n"+notes)
	case choice != "":
		parts = append(parts, choice)
	case notes != "":
		parts = append(parts, notes)
	}
	if len(attachments) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Attached images (%d):", len(attachments))
		for i, a := range attachments {
			fmt.Fprintf(&sb, "\n%d. %s (%s, %d bytes)\n   Local path: %s", i+1, a.Name, a.MIME, a.Size, a.Path)
		}
		parts = append(parts, sb.String())
	}
	return strings.Join(parts, "\n\n")
}

func saveDialogAttachments(payloads []dialogAttachmentPayload) ([]dialogAttachmentFile, error) {
	if len(payloads) == 0 {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "bmcptools-ask-user-*")
	if err != nil {
		return nil, err
	}

	var files []dialogAttachmentFile
	var errs []string
	usedNames := make(map[string]struct{}, len(payloads))
	seenData := make(map[[sha256.Size]byte]struct{}, len(payloads))
	for i, p := range payloads {
		mimeType, raw, err := decodeImageDataURL(p.MIME, p.Data)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fallbackAttachmentName(p.Name, i+1, ".png"), err))
			continue
		}
		digest := sha256.Sum256(raw)
		if _, exists := seenData[digest]; exists {
			continue
		}
		seenData[digest] = struct{}{}
		name := uniqueAttachmentName(fallbackAttachmentName(p.Name, i+1, extensionForImageMIME(mimeType)), usedNames)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		files = append(files, dialogAttachmentFile{
			Name: name,
			MIME: mimeType,
			Size: len(raw),
			Path: path,
		})
	}
	if len(errs) > 0 {
		return files, errors.New(strings.Join(errs, "; "))
	}
	return files, nil
}

func decodeImageDataURL(fallbackMIME, dataURL string) (string, []byte, error) {
	dataURL = strings.TrimSpace(dataURL)
	if dataURL == "" {
		return "", nil, fmt.Errorf("empty image data")
	}
	mimeType := strings.TrimSpace(fallbackMIME)
	encoded := dataURL
	if len(dataURL) >= len("data:") && strings.EqualFold(dataURL[:len("data:")], "data:") {
		comma := strings.IndexByte(dataURL, ',')
		if comma < 0 {
			return "", nil, fmt.Errorf("image data URL is not base64")
		}
		meta := dataURL[len("data:"):comma]
		metaParts := strings.Split(meta, ";")
		isBase64 := false
		for _, part := range metaParts[1:] {
			if strings.EqualFold(strings.TrimSpace(part), "base64") {
				isBase64 = true
				break
			}
		}
		if !isBase64 {
			return "", nil, fmt.Errorf("image data URL is not base64")
		}
		if strings.TrimSpace(metaParts[0]) != "" {
			mimeType = strings.TrimSpace(metaParts[0])
		}
		encoded = dataURL[comma+1:]
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "", nil, fmt.Errorf("unsupported attachment MIME type %q", mimeType)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("invalid base64 image data: %w", err)
	}
	return mimeType, raw, nil
}

func uniqueAttachmentName(name string, used map[string]struct{}) string {
	if name == "" {
		name = "image.img"
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	if base == "" {
		base = "image"
	}
	candidate := name
	for i := 2; ; i++ {
		key := strings.ToLower(candidate)
		if _, exists := used[key]; !exists {
			used[key] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func fallbackAttachmentName(name string, index int, ext string) string {
	name = sanitizeAttachmentName(name)
	if ext == "" {
		ext = ".img"
	}
	if name == "" {
		return fmt.Sprintf("image-%d%s", index, ext)
	}
	if filepath.Ext(name) == "" {
		name += ext
	}
	return name
}

func sanitizeAttachmentName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32:
			return -1
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, " .")
	if len(name) > 120 {
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		if len(stem) > 100 {
			stem = stem[:100]
		}
		name = stem + ext
	}
	return name
}

func extensionForImageMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "image/tiff":
		return ".tiff"
	default:
		return ".img"
	}
}

func chipHTML(c string) string {
	return strings.ReplaceAll(html.EscapeString(c), "\n", "<br>")
}

// encodeURIComponentJS mirrors JavaScript's encodeURIComponent so the client
// can decode it with decodeURIComponent. Go's url.QueryEscape uses '+' for
// spaces (form encoding), which decodeURIComponent leaves untouched — so we
// post-process to swap '+' back to '%20'.
func encodeURIComponentJS(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// buildDialogHTML renders the ask_user dialog HTML template.
// htmlSource is the base template (default or custom override).
func buildDialogHTML(htmlSource, question, details, title, subtitle string, choices []string, timeoutSec int) string {
	chipsSection := ""
	if len(choices) > 0 {
		var sb strings.Builder
		sb.WriteString(`<div class="chips-card"><div class="suggested-label">Suggested replies</div><div class="chips-row">`)
		for i, c := range choices {
			jC, _ := json.Marshal(c)
			// data-md carries the encodeURIComponent-equivalent of the raw
			// markdown so the client can render inline markdown (bold/code/
			// links) inside the chip body. chipHTML provides a no-JS fallback
			// that simply escapes + <br>s.
			sb.WriteString(fmt.Sprintf(
				`<button class="chip" id="chip%d" data-md="%s" onclick="pickChip(%s,%d)">%s</button>`,
				i, encodeURIComponentJS(c), html.EscapeString(string(jC)), i, chipHTML(c),
			))
		}
		sb.WriteString(`</div></div>`)
		chipsSection = sb.String()
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
			body, _ := io.ReadAll(r.Body)
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
