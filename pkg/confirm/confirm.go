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
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// DefaultTimeout is how long the dialog stays open before auto-cancelling.
const DefaultTimeout = 5 * time.Minute

// Ask opens a browser confirmation dialog and blocks until the user responds.
//
//   - title:   short operation name shown in the dialog header (e.g. "Revoke 3 Grants")
//   - details: plain-text description of exactly what will happen, shown verbatim in
//     a monospace box so structured data (JSON, YAML, lists) renders cleanly
//
// Returns:
//   - (true,  nil)  — user clicked Confirm
//   - (false, nil)  — user clicked Cancel (or dialog timed out / was dismissed)
//   - (false, err)  — the local server could not be started, or the context was cancelled
func Ask(ctx context.Context, title, details string) (bool, error) {
	return AskWithTimeout(ctx, title, details, DefaultTimeout)
}

// AskWithTimeout is like Ask but lets the caller override the auto-cancel timeout.
func AskWithTimeout(ctx context.Context, title, details string, timeout time.Duration) (bool, error) {
	switch runtime.GOOS {
	case "darwin", "windows":
	default:
		return false, fmt.Errorf("confirm.Ask is not supported on %s", runtime.GOOS)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false, fmt.Errorf("failed to start confirmation server: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	resultCh := make(chan bool, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	page := buildConfirmHTML(title, details, int(timeout.Seconds()))

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

		var payload struct {
			Confirmed bool `json:"confirmed"`
		}
		_ = json.Unmarshal(body, &payload)
		select {
		case resultCh <- payload.Confirmed:
		default:
		}
	})

	go func() { _ = srv.Serve(ln) }()
	// Use graceful Shutdown (with a short deadline) instead of Close to avoid
	// cutting off in-flight response writes and leaking goroutines.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port))

	select {
	case confirmed := <-resultCh:
		return confirmed, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(timeout):
		return false, fmt.Errorf("confirmation timed out after %s — operation was not executed", timeout)
	}
}

// openBrowser launches the default browser to the given URL.
// It is a best-effort call; errors are intentionally ignored.
var openBrowser = func(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = openCommand("open", url)
	case "windows":
		_ = openCommand("cmd", "/c", "start", url)
	}
}

// buildConfirmHTML generates the full HTML page for the confirmation dialog.
func buildConfirmHTML(title, details string, timeoutSec int) string {
	escapedTitle := html.EscapeString(title)
	escapedDetails := html.EscapeString(details)
	detailsHTML := strings.ReplaceAll(escapedDetails, "\n", "<br>")

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>⚠️ Confirm: %s</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #0f0f0f;
    color: #e0e0e0;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
  }
  .card {
    background: #1a1a1a;
    border: 1px solid #333;
    border-radius: 12px;
    max-width: 720px;
    width: 100%%;
    overflow: hidden;
    box-shadow: 0 20px 60px rgba(0,0,0,0.5);
  }
  .header {
    background: linear-gradient(135deg, #c0392b, #8e1a13);
    padding: 20px 24px;
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .header-icon { font-size: 28px; }
  .header-text h1 { font-size: 20px; font-weight: 700; color: #fff; }
  .header-text p  { font-size: 13px; color: rgba(255,255,255,0.8); margin-top: 2px; }
  .body { padding: 24px; }
  .warning-box {
    background: rgba(192,57,43,0.15);
    border: 1px solid rgba(192,57,43,0.4);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 20px;
    font-size: 13px;
    color: #e74c3c;
  }
  .details-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #888;
    margin-bottom: 8px;
  }
  .details-box {
    background: #111;
    border: 1px solid #2a2a2a;
    border-radius: 8px;
    padding: 16px;
    font-family: 'SF Mono', 'Fira Code', 'Courier New', monospace;
    font-size: 13px;
    line-height: 1.7;
    color: #ccc;
    max-height: 400px;
    overflow-y: auto;
    margin-bottom: 24px;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .timer {
    font-size: 12px;
    color: #666;
    text-align: center;
    margin-bottom: 20px;
  }
  .timer span { color: #e74c3c; font-weight: 600; }
  .buttons { display: flex; gap: 12px; justify-content: flex-end; }
  button {
    padding: 10px 28px;
    border-radius: 8px;
    font-size: 15px;
    font-weight: 600;
    border: none;
    cursor: pointer;
    transition: all 0.15s;
  }
  button:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-cancel  { background: #2a2a2a; color: #aaa; border: 1px solid #3a3a3a; }
  .btn-cancel:hover:not(:disabled) { background: #333; color: #fff; }
  .btn-confirm { background: #c0392b; color: #fff; }
  .btn-confirm:hover:not(:disabled) { background: #e74c3c; }
  .status { text-align: center; font-size: 16px; padding: 16px; display: none; }
</style>
</head>
<body>
<div class="card">
  <div class="header">
    <div class="header-icon">⚠️</div>
    <div class="header-text">
      <h1>%s</h1>
      <p>This is a destructive operation. Please review carefully before confirming.</p>
    </div>
  </div>
  <div class="body">
    <div class="warning-box">
      🔴 <strong>This action cannot be undone automatically.</strong>
      Review the operation details below, then click <strong>Confirm</strong> to proceed
      or <strong>Cancel</strong> to abort.
    </div>
    <div class="details-label">Operation Details</div>
    <div class="details-box">%s</div>
    <div class="timer">Auto-cancels in <span id="countdown">%d</span>s if no action is taken.</div>
    <div class="buttons">
      <button class="btn-cancel"  id="cancelBtn"  onclick="answer(false)">✕ Cancel</button>
      <button class="btn-confirm" id="confirmBtn" onclick="answer(true)">✓ Confirm &amp; Execute</button>
    </div>
    <div class="status" id="status"></div>
  </div>
</div>
<script>
  var remaining = %d;
  var timer = setInterval(function() {
    remaining--;
    document.getElementById('countdown').textContent = remaining;
    if (remaining <= 0) { clearInterval(timer); answer(false); }
  }, 1000);

  function answer(confirmed) {
    clearInterval(timer);
    document.getElementById('cancelBtn').disabled  = true;
    document.getElementById('confirmBtn').disabled = true;
    var s = document.getElementById('status');
    s.style.display = 'block';
    s.textContent   = confirmed ? '✓ Confirmed. Executing…' : '✕ Cancelled. Closing…';
    s.style.color   = confirmed ? '#2ecc71' : '#e74c3c';
    fetch('/answer', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({confirmed: confirmed})
    }).catch(function(){});
    setTimeout(function() { window.close(); }, 1500);
  }
</script>
</body>
</html>`, escapedTitle, escapedTitle, detailsHTML, timeoutSec, timeoutSec)
}
