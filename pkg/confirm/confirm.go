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

// Theme selects a visual preset for the confirmation dialog.
type Theme int

const (
	// ThemeDefault is the generic red warning theme (used for non-specific operations).
	ThemeDefault Theme = iota
	// ThemeBinance applies Binance USDT-M Futures branding: gold avatar, candlestick
	// icon, Binance-yellow confirm button, and a "Binance USDT-M Futures" subtitle.
	ThemeBinance
)

// EditableParam defines a user-editable field shown in the confirmation dialog.
// Pre-filled with the AI's proposed value; the user may modify it before confirming.
// Final values (possibly modified) are returned by Ask in the params map.
type EditableParam struct {
	Key   string `json:"key"`            // internal key returned in params map
	Label string `json:"label"`          // human-readable label shown next to the input
	Value string `json:"value"`          // pre-filled (AI-proposed) value
	Type  string `json:"type"`           // "number" or "text"
	Step  string `json:"step,omitempty"` // for number inputs: "any", "1", "0.001"
	Min   string `json:"min,omitempty"`  // optional minimum for number inputs
}

// Option configures a confirm dialog call.
type Option func(*confirmConfig)

type confirmConfig struct {
	customHTML     string
	timeout        time.Duration
	theme          Theme
	subtitle       string
	editableParams []EditableParam
}

// WithEditableParams adds user-editable parameter fields to the confirmation dialog.
// The user can modify these before confirming; final values are returned by Ask.
// When BINANCE_SKIP_ASK_USER bypasses the dialog, the original values are returned.
func WithEditableParams(params []EditableParam) Option {
	return func(c *confirmConfig) { c.editableParams = params }
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

// WithTheme selects a visual preset for the confirmation dialog.
// Use ThemeBinance for Binance Futures operations.
func WithTheme(t Theme) Option {
	return func(c *confirmConfig) { c.theme = t }
}

// WithSubtitle overrides the subtitle text shown below the dialog title.
// If empty, the theme's default subtitle is used.
func WithSubtitle(s string) Option {
	return func(c *confirmConfig) { c.subtitle = s }
}

// Ask opens a browser confirmation dialog and blocks until the user responds.
//
//   - title:   short operation name shown in the dialog header
//   - details: markdown-formatted description of what will happen (rendered
//     client-side with the same renderer used by the ask_user dialog)
//
// Returns:
//   - (true,  params, nil)  — user confirmed; params contains final edited values keyed by EditableParam.Key
//   - (false, nil,    nil)  — user cancelled, dismissed, or dialog timed out
//   - (false, nil,    err)  — the local server could not be started, or the context was cancelled
func Ask(ctx context.Context, title, details string, opts ...Option) (bool, map[string]string, error) {
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

func ask(ctx context.Context, title, details string, cfg *confirmConfig) (bool, map[string]string, error) {
	if err := checkPlatform(); err != nil {
		return false, nil, err
	}

	resultCh := make(chan []byte, 1)
	mux := http.NewServeMux()

	var page string
	if cfg.customHTML != "" {
		page = cfg.customHTML
	} else {
		page = buildConfirmHTML(cfg, title, details, int(cfg.timeout.Seconds()))
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
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		select {
		case resultCh <- body:
		default:
		}
	})

	data, err := serveDialog(ctx, mux, cfg.timeout, resultCh)
	if err != nil {
		return false, nil, err
	}

	var payload struct {
		Confirmed bool              `json:"confirmed"`
		Params    map[string]string `json:"params"`
	}
	_ = json.Unmarshal(data, &payload)
	if !payload.Confirmed {
		return false, nil, nil
	}
	return true, payload.Params, nil
}

// BuildConfirmHTML renders the confirm dialog HTML with the given options.
// Useful for preview scripts and testing.
func BuildConfirmHTML(title, details string, timeoutSec int, opts ...Option) string {
	cfg := &confirmConfig{timeout: DefaultTimeout}
	for _, o := range opts {
		o(cfg)
	}
	return buildConfirmHTML(cfg, title, details, timeoutSec)
}
// `details` is treated as **markdown** and rendered client-side via the shared
// markdown renderer (md.js / md.css), matching the look of the ask_user dialog.
func buildConfirmHTML(cfg *confirmConfig, title, details string, timeoutSec int) string {
	escapedTitle := html.EscapeString(title)
	detailsJSON, _ := json.Marshal(details)

	// Resolve theme values.
	faviconHref, avatarSVG, extraCSS, extraJS, defaultSubtitle := resolveTheme(cfg.theme)

	subtitle := cfg.subtitle
	if subtitle == "" {
		subtitle = defaultSubtitle
	}

	editableParamsJSON := "[]"
	if len(cfg.editableParams) > 0 {
		if b, err := json.Marshal(cfg.editableParams); err == nil {
			editableParamsJSON = string(b)
		}
	}

	page := asset.HTML("confirm")
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+asset.CSS("md")+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+asset.JS("md")+"\n</script>")
	page = strings.ReplaceAll(page, "[[TITLE]]", escapedTitle)
	page = strings.ReplaceAll(page, "[[DETAILS_JSON]]", string(detailsJSON))
	page = strings.ReplaceAll(page, "[[TIMEOUT_SEC]]", fmt.Sprintf("%d", timeoutSec))
	page = strings.ReplaceAll(page, "[[FAVICON_HREF]]", faviconHref)
	page = strings.ReplaceAll(page, "[[AVATAR_SVG]]", avatarSVG)
	page = strings.ReplaceAll(page, "[[SUBTITLE]]", html.EscapeString(subtitle))
	page = strings.ReplaceAll(page, "[[EXTRA_CSS]]", extraCSS)
	page = strings.ReplaceAll(page, "[[EXTRA_JS]]", extraJS)
	page = strings.ReplaceAll(page, "[[EDITABLE_PARAMS_JSON]]", editableParamsJSON)
	return page
}

// resolveTheme returns the favicon href, avatar SVG, extra CSS, extra JS, and default subtitle
// for the given theme preset.
func resolveTheme(t Theme) (faviconHref, avatarSVG, extraCSS, extraJS, subtitle string) {
	switch t {
	case ThemeBinance:
		return binanceFaviconHref, binanceAvatarSVG, binanceExtraCSS, binanceExtraJS,
			"Binance USDT-M Futures \u00b7 Real funds at risk"
	default:
		return defaultFaviconHref, defaultAvatarSVG, "", "", "Destructive operation \u2014 review before confirming"
	}
}

const defaultFaviconHref = "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><circle cx='16' cy='16' r='16' fill='%23ff3b30'/><path d='M16 9v9' stroke='white' stroke-width='3' stroke-linecap='round'/><circle cx='16' cy='23' r='2' fill='white'/></svg>"

const defaultAvatarSVG = `<svg viewBox="0 0 24 24"><path d="M12 3 2 21h20L12 3z"/><path d="M12 10v5"/><circle cx="12" cy="18" r="0.5" fill="#fff"/></svg>`

// binanceFaviconHref is a yellow circle with a bar-chart icon (trading identity).
const binanceFaviconHref = "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><circle cx='16' cy='16' r='16' fill='%23F0B90B'/><path d='M8 24V18M13 24V12M19 24V15M24 24V9' stroke='white' stroke-width='2.5' stroke-linecap='round'/></svg>"

// binanceAvatarSVG is a 3-candle candlestick chart (Binance Futures trading icon).
const binanceAvatarSVG = `<svg viewBox="0 0 24 24"><line x1="5" y1="4" x2="5" y2="7"/><rect x="3" y="7" width="4" height="7" rx="0.5"/><line x1="5" y1="14" x2="5" y2="17"/><line x1="12" y1="3" x2="12" y2="6"/><rect x="10" y="6" width="4" height="8" rx="0.5"/><line x1="12" y1="14" x2="12" y2="18"/><line x1="19" y1="5" x2="19" y2="8"/><rect x="17" y="8" width="4" height="6" rx="0.5"/><line x1="19" y1="14" x2="19" y2="17"/></svg>`

// binanceExtraCSS overrides avatar and confirm-button colors to Binance gold,
// keeping danger/warning signals (timer, bubble emphasis) in their original red.
// fin-bull/fin-bear are applied by binanceExtraJS to LONG/SHORT/+%/-% tokens.
const binanceExtraCSS = `.avatar{background:linear-gradient(135deg,#F3A829 0%,#d4a017 100%);box-shadow:0 2px 8px rgba(240,185,11,.40);}
.btn-confirm{background:linear-gradient(135deg,#F3A829,#d4a017);box-shadow:0 2px 8px rgba(240,185,11,.40);}
@keyframes confirm-pulse{0%,100%{box-shadow:0 0 0 0 rgba(240,185,11,.55)}50%{box-shadow:0 0 0 8px rgba(240,185,11,0)}}
.fin-bull{color:#0ecb81;font-weight:600}
.fin-bear{color:#f6465d;font-weight:600}`

// binanceExtraJS walks the rendered details body and wraps directional terms
// (LONG/SHORT/BUY/SELL/bullish/bearish) and signed percentages (+X%/-X%) in
// colored spans using Binance's official bull (#0ecb81) and bear (#f6465d) colors.
// Code blocks (<code>, <pre>) are skipped to avoid breaking syntax highlighting.
const binanceExtraJS = `(function(){
  var body=document.getElementById('detailsBody');
  if(!body)return;
  var re=/\b(LONG|SHORT|BUY|SELL|bullish|bearish)\b|([+-]\d+(?:\.\d+)?%)/gi;
  function walk(node){
    if(node.nodeType===3){
      var txt=node.textContent;
      if(!re.test(txt)){re.lastIndex=0;return;}
      re.lastIndex=0;
      var frag=document.createDocumentFragment(),last=0,m;
      while((m=re.exec(txt))!==null){
        if(m.index>last)frag.appendChild(document.createTextNode(txt.slice(last,m.index)));
        var sp=document.createElement('span'),v=m[0];
        sp.className=/^(LONG|BUY|bullish)$/i.test(v)||v[0]==='+'?'fin-bull':'fin-bear';
        sp.textContent=v;frag.appendChild(sp);
        last=m.index+v.length;
      }
      if(last<txt.length)frag.appendChild(document.createTextNode(txt.slice(last)));
      node.parentNode.replaceChild(frag,node);
    }else if(node.nodeType===1&&!/^(script|style|code|pre)$/i.test(node.tagName)){
      Array.from(node.childNodes).forEach(walk);
    }
  }
  Array.from(body.childNodes).forEach(walk);
})()`
