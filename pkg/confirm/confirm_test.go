package confirm

import (
	"strings"
	"testing"
)

func TestBuildConfirmHTMLEscapesDynamicStrings(t *testing.T) {
	page := BuildConfirmHTML(
		`Delete </title><script>alert(1)</script>`,
		`Details </script><script>alert(2)</script>`,
		30,
		WithSubtitle(`Sub "><img src=x onerror=alert(3)>`),
		WithEditableParams([]EditableParam{{
			Key:   `path"><img src=x>`,
			Label: `Label <script>alert(4)</script>`,
			Value: `Value </script><script>alert(5)</script>`,
			Type:  `text`,
		}}),
	)

	for _, forbidden := range []string{
		`Delete </title><script>alert(1)</script>`,
		`Details </script><script>alert(2)</script>`,
		`Sub "><img src=x onerror=alert(3)>`,
		`Label <script>alert(4)</script>`,
		`Value </script><script>alert(5)</script>`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("rendered confirm page contains raw dynamic HTML/script text %q", forbidden)
		}
	}
	if !strings.Contains(page, `Delete &lt;/title&gt;&lt;script&gt;alert(1)&lt;/script&gt;`) {
		t.Fatalf("title was not HTML-escaped in markup")
	}
	if !strings.Contains(page, `\u003c/script\u003e`) {
		t.Fatalf("JSON script data did not HTML-escape closing script text")
	}
}
