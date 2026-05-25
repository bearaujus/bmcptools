// preview renders each HTML template with sample data and opens the results in
// the browser. Run from the repo root:
//
//	go run ./scripts/preview [dialog|rest|confirm]
//
// With no arguments all pages are rendered and opened simultaneously.
package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/confirm"
)

func main() {
	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"dialog", "rest", "confirm"}
	}

	for _, t := range targets {
		switch t {
		case "dialog":
			openPreview("dialog", renderDialog())
		case "rest":
			openPreview("rest", renderRest())
		case "confirm":
			openPreview("confirm", renderConfirm())
		default:
			fmt.Fprintf(os.Stderr, "unknown template %q — must be dialog, rest, or confirm\n", t)
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

	chips := `<div class="chips-card"><div class="suggested-label">Suggested replies</div><div class="chips-row">` +
		chip("Yes, looks **great**!", 0) +
		chip("I have some tweaks — see `notes` below", 1) +
		chip("Let me think…", 2) +
		chip("Run **project cleanup** for `dist/cache/tmp` with dry-run first — long enough to wrap to a second line so we can verify the alignment fix", 3) +
		chip("Reject — path is **outside workspace**", 4) +
		chip("[Docs link](https://example.com) inside a chip", 5) +
		`</div></div>`

	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: ask_user dialog"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	question := "Does the table styling look good to you?"
	questionJSON, _ := json.Marshal(question)
	tmpl = strings.ReplaceAll(tmpl, "[[QUESTION]]", html.EscapeString(question))
	tmpl = strings.ReplaceAll(tmpl, "[[QUESTION_JSON]]", string(questionJSON))

	details := "## Markdown showcase — every renderer feature\n\n" +
		"This preview exercises every code path in `mdRender()` so visual regressions are obvious. " +
		"Use the **copy button** at the top-right of the AI card to grab this whole message as raw markdown.\n\n" +
		"---\n\n" +
		"### 1. Headings\n\n" +
		"# H1 — Page title\n" +
		"## H2 — Section\n" +
		"### H3 — Subsection\n" +
		"#### H4 — Group\n" +
		"##### H5 — Detail\n" +
		"###### H6 — Note\n\n" +
		"### 2. Inline emphasis\n\n" +
		"Plain · **bold** · *italic* · ***bold italic*** · `inline code` · " +
		"[external link](https://github.com/bearaujus/bmcptools) · escaped backslash `\\n` becomes a newline.\n\n" +
		"### 3. Blockquotes\n\n" +
		"> Single-line quote with **bold** + `code` + [link](https://example.com) inside.\n\n" +
		"### 4. Lists\n\n" +
		"Unordered (mixed markers):\n\n" +
		"- First bullet\n" +
		"* Star marker\n" +
		"- Third — has `inline code` and **bold**\n\n" +
		"Ordered starting at 1:\n\n" +
		"1. One\n" +
		"2. Two\n" +
		"3. Three\n\n" +
		"Ordered starting at 7 (regression: must show 7, 8, 9 — not 1, 1, 1):\n\n" +
		"7. Seven\n" +
		"8. Eight\n" +
		"9. Nine\n\n" +
		"### 5. Horizontal rule\n\n" +
		"---\n\n" +
		"### 6. Tables — alignment + horizontal scroll\n\n" +
		"Aligned table:\n\n" +
		"| Left | Center | Right |\n" +
		"|:-----|:------:|------:|\n" +
		"| a    | b      | 1.00  |\n" +
		"| long text | mid | 99.99 |\n\n" +
		"Wide table — scroll inside the rounded frame, **never overflows the card**:\n\n" +
		"| File | Operation | Size | Modified | Owner | Status | Risk | Backup | Hash | Notes | Next Step | Ticket |\n" +
		"|---|---|---|---|---|---|---|---|---|---|---|---|\n" +
		"| `dist/app.js` | delete | 2.4 MB | today | build | ignored | low | yes | `a3f1...` | generated bundle | remove | OPS-12 |\n" +
		"| `cache/index.db` | inspect | 18 MB | yesterday | app | untracked | medium | no | `9bc2...` | may contain local state | ask user | OPS-13 |\n" +
		"| `logs/debug.log` | truncate | 41 MB | today | dev | ignored | low | yes | `71de...` | noisy output | rotate | OPS-14 |\n\n" +
		"Pipe escaping inside cell: `val\\|with\\|pipes` should render as one cell.\n\n" +
		"### 7. Inline code edge cases\n\n" +
		"Single backtick: `os.Args[1]`. Double backtick allows ` inside: ``echo `pwd` ``. Empty `` `` should not crash.\n\n" +
		"### 8. Code blocks\n\n" +
		"Language label visible in the new header bar; copy button no longer overlaps the code.\n\n" +
		"```go\nfunc main() {\n    fmt.Println(\"Hello, bmcptools!\")\n}\n```\n\n" +
		"With JSON:\n\n" +
		"```json\n{\n  \"mcpServers\": {\n    \"bmcptools\": {\n      \"command\": \"/usr/local/bin/bmcptools\",\n      \"args\": [\"--disable=user\"]\n    }\n  }\n}\n```\n\n" +
		"With shell:\n\n" +
		"```sh\n# Drop interactive user prompts\nbmcptools --disable=user\n\n# List valid group names\nbmcptools --list-groups\n```\n\n" +
		"Plain (no language) — header still shows `text`:\n\n" +
		"```\nplain text\nno highlighting\n```\n\n" +
		"Long block (collapses with \"Show N more lines\"):\n\n" +
		"```go\npackage main\n\nimport (\n\t\"fmt\"\n\t\"math/big\"\n\t\"crypto/elliptic\"\n)\n\n" +
		"// Point represents a point on an elliptic curve.\ntype Point struct{ X, Y *big.Int }\n\n" +
		"func main() {\n\tcurve := elliptic.P256()\n\tparams := curve.Params()\n\tGx, Gy := params.Gx, params.Gy\n\tfmt.Printf(\"Curve: P-256\\n\")\n\tfmt.Printf(\"Generator: (%x, %x)\\n\", Gx, Gy)\n\tfmt.Printf(\"Order: %x\\n\", params.N)\n\n" +
		"\tk := big.NewInt(42)\n\tPx, Py := curve.ScalarBaseMult(k.Bytes())\n\tfmt.Printf(\"42*G = (%x, %x)\\n\", Px, Py)\n\n" +
		"\tQx, Qy := curve.ScalarBaseMult(big.NewInt(7).Bytes())\n\tRx, Ry := curve.Add(Px, Py, Qx, Qy)\n\tfmt.Printf(\"P+Q = (%x, %x)\\n\", Rx, Ry)\n\n" +
		"\t// More lines so it definitely collapses\n\tfor i := 0; i < 5; i++ { fmt.Println(i) }\n}\n```\n\n" +
		"### 9. Mixed nested content\n\n" +
		"> A quote that contains:\n> - a bullet\n> - and **bold** + `code`\n\n" +
		"### 10. Auto-grow textarea sanity\n\n" +
		"Scroll down to the reply box and paste something **very long** — the textarea should cap at ~40% of the viewport and scroll internally instead of pushing the page around."

	// Match production: empty container with id="detailsBody"; the dialog JS
	// renders the markdown into it from DETAILS_JSON.
	detailsSection := `<div class="details-card"><div class="details-body md-body" id="detailsBody"></div></div>`
	detailsJSON, _ := json.Marshal(details)
	tmpl = strings.ReplaceAll(tmpl, "[[DETAILS_SECTION]]", detailsSection)
	tmpl = strings.ReplaceAll(tmpl, "[[DETAILS_JSON]]", string(detailsJSON))
	tmpl = strings.ReplaceAll(tmpl, "[[CHIPS_SECTION]]", chips)
	tmpl = strings.ReplaceAll(tmpl, "[[TIMEOUT_SEC]]", "600")
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

func renderConfirm() string {
	details := "### Proposed action\n" +
		"Delete directory **`D:\\\\tmp\\\\old-build`** with recursive cleanup enabled.\n\n" +
		"### Risk\n" +
		"| Check | Value |\n" +
		"|---|---|\n" +
		"| Target path | `D:\\\\tmp\\\\old-build` |\n" +
		"| Recursive | `true` |\n" +
		"| Reversible | No |\n\n" +
		"### AI reasoning\n" +
		"The directory is an ignored build output folder and no source files are expected inside it.\n\n" +
		"```sh\n# Equivalent CLI for audit:\nbmcptools delete_directory --path D:\\\\tmp\\\\old-build --force true\n```"

	return confirm.BuildConfirmHTML("delete_directory", details, 300,
		confirm.WithEditableParams([]confirm.EditableParam{
			{Key: "path", Label: "Path", Value: "D:\\tmp\\old-build", Type: "text"},
			{Key: "force", Label: "Force", Value: "true", Type: "text"},
		}),
	)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func chip(label string, idx int) string {
	jLabel, _ := json.Marshal(label)
	return fmt.Sprintf(
		`<button class="chip" id="chip%d" data-md="%s" onclick="pickChip(%s,%d)">%s</button>`,
		idx, encodeURIComponentJS(label), html.EscapeString(string(jLabel)), idx, html.EscapeString(label),
	)
}

// encodeURIComponentJS mirrors JavaScript's encodeURIComponent.
func encodeURIComponentJS(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
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
