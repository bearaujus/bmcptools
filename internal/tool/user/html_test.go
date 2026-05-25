package user

import (
	"strings"
	"testing"
)

func TestBuildDialogHTMLEscapesDynamicTemplateValues(t *testing.T) {
	tmpl := `<title>[[TITLE]]</title>` +
		`<div id="sub">[[SUBTITLE]]</div>` +
		`<div id="q">[[QUESTION]]</div>` +
		`<script>var QUESTION_RAW=[[QUESTION_JSON]];var DETAILS_RAW=[[DETAILS_JSON]];</script>` +
		`[[DETAILS_SECTION]][[CHIPS_SECTION]][[TIMEOUT_SEC]][[MD_CSS]][[MD_JS]]`

	page := buildDialogHTML(
		tmpl,
		`Question <img src=x onerror=alert(1)>`,
		`Details </script><script>alert(2)</script>`,
		`Bad </title><script>alert(3)</script>`,
		`Sub "><img src=x>`,
		[]string{`Yes " onclick="alert(4)`, `</button><script>alert(5)</script>`},
		42,
	)

	for _, forbidden := range []string{
		`Question <img src=x onerror=alert(1)>`,
		`Bad </title><script>alert(3)</script>`,
		`Details </script><script>alert(2)</script>`,
		`</button><script>alert(5)</script>`,
		`onclick="alert(4)"`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("rendered page contains raw dynamic HTML/attribute text %q", forbidden)
		}
	}
	if !strings.Contains(page, `Question &lt;img src=x onerror=alert(1)&gt;`) {
		t.Fatalf("question fallback was not HTML-escaped")
	}
	if !strings.Contains(page, `\u003c/script\u003e`) {
		t.Fatalf("JSON script data did not HTML-escape closing script text")
	}
}

func TestBuildRestHTMLEscapesNotesInScriptData(t *testing.T) {
	tmpl := `<title>[[TITLE]]</title>` +
		`<div id="sub">[[SUBTITLE]]</div>` +
		`<script>var NOTES_RAW=[[NOTES_ESCAPED]];</script>` +
		`[[TIMEOUT_SEC]][[MD_CSS]][[MD_JS]]`

	page := buildRestHTML(
		tmpl,
		`Rest </title><script>alert(1)</script>`,
		`Sub <img src=x>`,
		`Note </script><script>alert(2)</script>`,
		60,
	)

	for _, forbidden := range []string{
		`Rest </title><script>alert(1)</script>`,
		`Sub <img src=x>`,
		`Note </script><script>alert(2)</script>`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("rendered rest page contains raw dynamic HTML/script text %q", forbidden)
		}
	}
	if !strings.Contains(page, `\u003c/script\u003e`) {
		t.Fatalf("notes JSON did not HTML-escape closing script text")
	}
}
