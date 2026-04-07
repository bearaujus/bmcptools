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
	"path/filepath"
	"strings"
)

func main() {
	// Resolve the repo root (one level up from scripts/preview).
	exe, _ := os.Executable()
	scriptDir := filepath.Dir(exe)
	_ = scriptDir

	// When run with `go run`, the CWD is typically the repo root.
	// Read assets relative to CWD.
	assetsDir := filepath.Join("assets", "html")

	mdCSS := mustRead(filepath.Join(assetsDir, "md.css"))
	mdJS := mustRead(filepath.Join(assetsDir, "md.js"))

	inject := func(page string) string {
		page = strings.ReplaceAll(page, "[[MD_CSS]]", "<style>\n"+mdCSS+"\n</style>")
		page = strings.ReplaceAll(page, "[[MD_JS]]", "<script>\n"+mdJS+"\n</script>")
		return page
	}

	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"dialog", "chat", "rest"}
	}

	for _, t := range targets {
		switch t {
		case "dialog":
			openPreview("dialog", renderDialog(inject, assetsDir))
		case "chat":
			openPreview("chat", renderChat(inject, assetsDir))
		case "rest":
			openPreview("rest", renderRest(inject, assetsDir))
		default:
			fmt.Fprintf(os.Stderr, "unknown template %q — must be dialog, chat, or rest\n", t)
			os.Exit(1)
		}
	}
}

// ── per-template renderers ────────────────────────────────────────────────────

func renderDialog(inject func(string) string, dir string) string {
	tmpl := mustRead(filepath.Join(dir, "dialog.html"))
	tmpl = inject(tmpl)

	chips := `<div class="chips">` +
		chip("Yes, looks great!", 0) +
		chip("I have some tweaks", 1) +
		chip("Let me think…", 2) +
		`</div>`

	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: ask_user dialog"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	tmpl = strings.ReplaceAll(tmpl, "[[QUESTION]]", html.EscapeString("Does the new `open_chat` UI look good to you?\n\nThis is a **sample question** rendered as markdown. You can write `code`, lists, and more.\n\n- Option A\n- Option B\n- Option C"))
	tmpl = strings.ReplaceAll(tmpl, "[[CHIPS_SECTION]]", chips)
	tmpl = strings.ReplaceAll(tmpl, "[[TIMEOUT_SEC]]", "600")
	tmpl = strings.ReplaceAll(tmpl, "[[ALLOW_FREEFORM]]", "true")
	return tmpl
}

func renderChat(inject func(string) string, dir string) string {
	tmpl := mustRead(filepath.Join(dir, "chat.html"))
	tmpl = inject(tmpl)

	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: open_chat"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	return tmpl
}

func renderRest(inject func(string) string, dir string) string {
	tmpl := mustRead(filepath.Join(dir, "rest.html"))
	tmpl = inject(tmpl)

	notes := "## Taking a short break\n\nI'm analysing the codebase. Feel free to:\n\n- Review the diff above\n- Grab a coffee ☕\n- Come back whenever you're ready\n\nI'll wake up the moment you press the button."
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

func mustRead(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		os.Exit(1)
	}
	return string(b)
}

func openPreview(name, html string) {
	tmpFile := filepath.Join(os.TempDir(), "bmcptools_preview_"+name+".html")
	if err := os.WriteFile(tmpFile, []byte(html), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", tmpFile, err)
		os.Exit(1)
	}
	fmt.Printf("▸ %s → %s\n", name, tmpFile)
	_ = exec.Command("open", tmpFile).Start()
}
