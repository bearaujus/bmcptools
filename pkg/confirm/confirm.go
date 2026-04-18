// Package confirm provides a blocking browser-based confirm/cancel dialog.
//
// It starts a local HTTP server on a random port, opens the browser via the
// platform's default launcher, and blocks until the user clicks Confirm or
// Cancel — or until the context is cancelled / the built-in timeout fires.
//
// Supported platforms: macOS, Windows.
// Linux is not supported; calling Ask on Linux returns an error immediately.
package confirm

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

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/browser"
	"github.com/bearaujus/bmcptools/pkg/dialog"
)

// DefaultTimeout is how long the dialog stays open before auto-cancelling.
const DefaultTimeout = 5 * time.Minute

// Option configures a confirm dialog call.
type Option func(*confirmConfig)

type confirmConfig struct {
	customHTML string
	timeout    time.Duration
}

// WithHTML overrides the default confirm dialog HTML with a custom template.
// Use dialog.NewDialogTemplate to create and validate the template.
func WithHTML(t dialog.DialogTemplate) Option {
	return func(c *confirmConfig) { c.customHTML = t.HTML() }
}

// WithTimeout sets a custom auto-cancel timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *confirmConfig) { c.timeout = d }
}

// Ask opens a browser confirmation dialog and blocks until the user responds.
//
//   - title:   short operation name shown in the dialog header
//   - details: markdown-formatted description of what will happen (rendered
//     client-side with the same renderer used by the ask_user dialog)
//
// Returns:
//   - (true,  nil)  — user clicked Confirm
//   - (false, nil)  — user clicked Cancel (or dialog timed out / was dismissed)
//   - (false, err)  — the local server could not be started, or the context was cancelled
func Ask(ctx context.Context, title, details string, opts ...Option) (bool, error) {
	cfg := &confirmConfig{timeout: DefaultTimeout}
	for _, o := range opts {
		o(cfg)
	}
	return ask(ctx, title, details, cfg)
}

// AskWithHTML serves a caller-provided dialog template instead of the built-in confirm page.
// The HTML must POST JSON to /answer when the user acts — e.g. {"confirmed": true}.
// All JSON fields in that payload are returned in the result map.
func AskWithHTML(ctx context.Context, tmpl dialog.DialogTemplate, timeout time.Duration) (map[string]interface{}, error) {
	if err := checkPlatform(); err != nil {
		return nil, err
	}

	resultCh := make(chan []byte, 1)
	mux := http.NewServeMux()

	pageHTML := tmpl.HTML()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	mux.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<10))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		select {
		case resultCh <- body:
		default:
		}
	})

	data, err := serveDialog(ctx, mux, timeout, resultCh)
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		payload = map[string]interface{}{"confirmed": false}
	}
	return payload, nil
}

// ShowHTML opens a browser window displaying the given HTML for displayDuration,
// then shuts down the local server. It returns immediately after opening the browser
// (fire-and-forget). Errors starting the server are silently ignored.
func ShowHTML(pageHTML string, displayDuration time.Duration) {
	if checkPlatform() != nil {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	_, shutdown, err := browser.ServeAndOpen(mux)
	if err != nil {
		return
	}
	go func() {
		time.Sleep(displayDuration)
		shutdown()
	}()
}

// checkPlatform returns an error if the current OS is unsupported.
func checkPlatform() error {
	switch runtime.GOOS {
	case "darwin", "windows":
		return nil
	default:
		return fmt.Errorf("confirm is not supported on %s", runtime.GOOS)
	}
}

// serveDialog starts the browser server and blocks until /answer is POSTed,
// the context is cancelled, or timeout elapses.
// resultCh must be buffered (capacity ≥ 1) and written to by the /answer handler.
func serveDialog(ctx context.Context, mux *http.ServeMux, timeout time.Duration, resultCh <-chan []byte) ([]byte, error) {
	_, shutdown, err := browser.ServeAndOpen(mux)
	if err != nil {
		return nil, fmt.Errorf("failed to start confirmation server: %w", err)
	}
	defer shutdown()

	select {
	case data := <-resultCh:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("confirmation timed out after %s — operation was not executed", timeout)
	}
}

func ask(ctx context.Context, title, details string, cfg *confirmConfig) (bool, error) {
	if err := checkPlatform(); err != nil {
		return false, err
	}

	resultCh := make(chan []byte, 1)
	mux := http.NewServeMux()

	var page string
	if cfg.customHTML != "" {
		page = cfg.customHTML
	} else {
		page = buildConfirmHTML(title, details, int(cfg.timeout.Seconds()))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})
	mux.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<10))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		select {
		case resultCh <- body:
		default:
		}
	})

	data, err := serveDialog(ctx, mux, cfg.timeout, resultCh)
	if err != nil {
		return false, err
	}

	var payload struct {
		Confirmed bool `json:"confirmed"`
	}
	_ = json.Unmarshal(data, &payload)
	return payload.Confirmed, nil
}

// buildConfirmHTML renders the default confirm dialog HTML from the embedded asset.
// `details` is treated as **markdown** and rendered client-side via the shared
// markdown renderer (md.js / md.css), matching the look of the ask_user dialog.
func buildConfirmHTML(title, details string, timeoutSec int) string {
	escapedTitle := html.EscapeString(title)
	detailsJSON, _ := json.Marshal(details)

	page := asset.HTML("confirm")
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+asset.CSS("md")+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+asset.JS("md")+"\n</script>")
	page = strings.ReplaceAll(page, "[[TITLE]]", escapedTitle)
	page = strings.ReplaceAll(page, "[[DETAILS_JSON]]", string(detailsJSON))
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	return page
}
