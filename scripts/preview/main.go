// preview renders each HTML template with sample data and opens the results in
// the browser. Run from the repo root:
//
//	go run ./scripts/preview [dialog|rest]
//
// With no arguments all pages are rendered and opened simultaneously.
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
		targets = []string{"dialog", "rest"}
	}

	for _, t := range targets {
		switch t {
		case "dialog":
			openPreview("dialog", renderDialog())
		case "rest":
			openPreview("rest", renderRest())
		default:
			fmt.Fprintf(os.Stderr, "unknown template %q — must be dialog or rest\n", t)
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
		chip("Yes, looks great!", 0) +
		chip("I have some tweaks", 1) +
		chip("Let me think…", 2) +
		`</div></div>`

	tmpl = strings.ReplaceAll(tmpl, "[[TITLE]]", html.EscapeString("Preview: ask_user dialog"))
	tmpl = strings.ReplaceAll(tmpl, "[[SUBTITLE]]", html.EscapeString("claude-sonnet-4.6 · preview mode"))
	tmpl = strings.ReplaceAll(tmpl, "[[QUESTION]]", html.EscapeString("Does the table styling look good to you?"))

	details := "### Summary of Changes\n\n" +
		"**Bold**, *italic*, `inline code`\n\n" +
		"---\n\n" +
		"#### Attack Registry\n\n" +
		"| Attack | Project | Status | Complexity |\n" +
		"|:-------|:-------|:-------|:-------|\n" +
		"| **Brute Force** | `00_intro` | ✅ Closed | *O(n)* |\n" +
		"| **Baby-step Giant-step** | `01_kronos` | ✅ Closed | *O(√n)* |\n" +
		"| **Pollard's Rho** | `01_kronos` | ✅ Closed | *O(√n)* |\n" +
		"| **MOV Attack** | `03_theseus` | ✅ Closed | *subexp* |\n" +
		"| **Index Calculus** | `10_dionysus` | ✅ Closed | *subexp* |\n" +
		"| **Shor's Algorithm** | `05_daedalus` | ⏳ Quantum | *poly* |\n\n" +
		"#### Short Code (no collapse)\n\n" +
		"```go\nfunc main() {\n    fmt.Println(\"Hello!\")\n}\n```\n\n" +
		"#### Long Code (collapsible)\n\n" +
		"```go\npackage main\n\nimport (\n\t\"fmt\"\n\t\"math/big\"\n\t\"crypto/elliptic\"\n)\n\n" +
		"// Point represents a point on an elliptic curve\ntype Point struct {\n\tX, Y *big.Int\n}\n\n" +
		"func main() {\n\tcurve := elliptic.P256()\n\tparams := curve.Params()\n\n" +
		"\t// Generator point\n\tGx := params.Gx\n\tGy := params.Gy\n\n" +
		"\tfmt.Printf(\"Curve: P-256\\n\")\n\tfmt.Printf(\"Generator: (%x, %x)\\n\", Gx, Gy)\n\tfmt.Printf(\"Order: %x\\n\", params.N)\n\n" +
		"\t// Scalar multiplication\n\tk := big.NewInt(42)\n\tPx, Py := curve.ScalarBaseMult(k.Bytes())\n\tfmt.Printf(\"42*G = (%x, %x)\\n\", Px, Py)\n\n" +
		"\t// Point addition\n\tQx, Qy := curve.ScalarBaseMult(big.NewInt(7).Bytes())\n\tRx, Ry := curve.Add(Px, Py, Qx, Qy)\n\tfmt.Printf(\"P+Q = (%x, %x)\\n\", Rx, Ry)\n}\n```\n\n" +
		"#### Checklist\n\n" +
		"1. First item\n2. Second item\n   - Nested bullet\n   - Another nested\n\n" +
		"> **Blockquote:** This is a quote with **bold** inside."

	detailsSection := `<div class="details-card"><div class="details-body md-body">` + html.EscapeString(details) + `</div></div>`
	tmpl = strings.ReplaceAll(tmpl, "[[DETAILS_SECTION]]", detailsSection)
	tmpl = strings.ReplaceAll(tmpl, "[[CHIPS_SECTION]]", chips)
	tmpl = strings.ReplaceAll(tmpl, "[[TIMEOUT_SEC]]", "600")
	tmpl = strings.ReplaceAll(tmpl, "[[ALLOW_FREEFORM]]", "true")
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
