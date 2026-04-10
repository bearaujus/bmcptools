// preview renders each HTML template with sample data and opens the results in
// the browser. Run from the repo root:
//
//	go run ./scripts/preview [dialog|chat|rest]
//
// With no arguments all three pages are rendered and opened simultaneously.
package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bearaujus/bmcptools/internal/asset"
)

func main() {
	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"dialog", "chat", "rest"}
	}

	for _, t := range targets {
		switch t {
		case "dialog":
			openPreview("dialog", renderDialog())
		case "chat":
			openPreview("chat", renderChat())
		case "rest":
			openPreview("rest", renderRest())
		default:
			fmt.Fprintf(os.Stderr, "unknown template %q — must be dialog, chat, or rest\n", t)
			os.Exit(1)
		}
	}
}

// ── per-template renderers ────────────────────────────────────────────────────

func inject(page string) string {
	page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+asset.CSS("md")+"\n</style>")
	page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+asset.JS("md")+"\n</script>")
	return page
}

func renderDialog() string {
	tmpl := inject(asset.HTML("dialog"))

	chips := `<div class="chips-row">` +
		chip("Yes, looks great!", 0) +
		chip("I have some tweaks", 1) +
		chip("Let me think…", 2) +
		`</div>`

	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: ask_user dialog"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	tmpl = strings.ReplaceAll(tmpl, "[[QUESTION]]", html.EscapeString("Does the new `open_chat` UI look good to you?\n\nThis is a **sample question** rendered as markdown."))
	tmpl = strings.ReplaceAll(tmpl, "[[DETAILS_SECTION]]", `<div class="details-body">Some optional details here.</div>`)
	tmpl = strings.ReplaceAll(tmpl, "[[CHIPS_SECTION]]", chips)
	tmpl = strings.ReplaceAll(tmpl, "[[TIMEOUT_SEC]]", "600")
	tmpl = strings.ReplaceAll(tmpl, "[[ALLOW_FREEFORM]]", "true")
	return tmpl
}

func renderChat() string {
	tmpl := inject(asset.HTML("chat"))
	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: open_chat"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	return tmpl
}

func renderRest() string {
	tmpl := inject(asset.HTML("rest"))
	notes := "## Taking a short break\n\nI'm analysing the codebase."
	notesJSON, _ := json.Marshal(notes)
	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: rest page"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	tmpl = strings.ReplaceAll(tmpl, "[[NOTES_ESCAPED]]", string(notesJSON))
	tmpl = strings.ReplaceAll(tmpl, "[[TIMEOUT_SEC]]", "120")
	return tmpl
}

// ── helpers ───────────────────────────────────────────────────────────────────

func chip(label string, idx int) string {
	jLabel, _ := json.Marshal(label)
	return fmt.Sprintf(
		`<button class="chip" id="chip%d" onclick="pickChip(%s,%d)">%s</button>`,
		idx, html.EscapeString(string(jLabel)), idx, html.EscapeString(label),
	)
}

func openPreview(name, pageHTML string) {
	tmpFile := fmt.Sprintf("%s/bmcptools_preview_%s.html", os.TempDir(), name)
	if err := os.WriteFile(tmpFile, []byte(pageHTML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", tmpFile, err)
		os.Exit(1)
	}
	fmt.Printf("▸ %s → %s\n", name, tmpFile)
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", tmpFile).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", tmpFile).Start()
	}
}
