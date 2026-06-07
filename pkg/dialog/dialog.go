// Package dialog defines validated HTML template types for browser-based dialogs.
//
// Each type wraps an HTML string and verifies at construction time that the page
// references the HTTP endpoints the server expects it to call. This gives a
// fast-fail guarantee: a misconfigured template is rejected at server startup,
// not discovered at runtime when a user actually interacts with the page.
//
// # Built-in defaults
//
// bmcptools ships with default HTML files embedded in internal/asset/html/.
// Custom templates are only required when the upstream wants its own look & feel.
//
// # Endpoint contracts
//
//	DialogTemplate — HTML must POST JSON to /answer
//	                  {"choice": string, "notes": string, "dismissed": bool}
//	RestTemplate   — HTML must POST JSON to /answer
//	                  {"notes": string}
package dialog

import (
	"fmt"
	"regexp"
	"strings"
)

// DialogTemplate is a validated HTML template for ask_user / confirm dialogs.
// The HTML must POST JSON to the /answer endpoint.
type DialogTemplate struct{ html string }

// NewDialogTemplate validates html and returns a DialogTemplate.
// Returns an error if the HTML does not actively use the /answer endpoint.
func NewDialogTemplate(html string) (DialogTemplate, error) {
	if err := requireEndpoints(html, "/answer"); err != nil {
		return DialogTemplate{}, fmt.Errorf("dialog template: %w", err)
	}
	return DialogTemplate{html: html}, nil
}

// HTML returns the validated HTML content.
func (t DialogTemplate) HTML() string { return t.html }

// RestTemplate is a validated HTML template for the rest/AFK page.
// The HTML must POST JSON to the /answer endpoint.
type RestTemplate struct{ html string }

// NewRestTemplate validates html and returns a RestTemplate.
// Returns an error if the HTML does not actively use /answer.
func NewRestTemplate(html string) (RestTemplate, error) {
	if err := requireEndpoints(html, "/answer"); err != nil {
		return RestTemplate{}, fmt.Errorf("rest template: %w", err)
	}
	return RestTemplate{html: html}, nil
}

// HTML returns the validated HTML content.
func (t RestTemplate) HTML() string { return t.html }

var htmlCommentPattern = regexp.MustCompile(`(?is)<!--.*?-->`)

// requireEndpoints returns an error when html does not appear to submit to every
// listed endpoint via a form action or common JavaScript transport. This is
// still a lightweight guard, not a full browser/runtime validator, but it
// avoids the old false positives from comments and longer unrelated paths.
func requireEndpoints(html string, endpoints ...string) error {
	html = htmlCommentPattern.ReplaceAllString(html, "")
	var missing []string
	for _, ep := range endpoints {
		if !usesEndpoint(html, ep) {
			missing = append(missing, ep)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("HTML must submit to endpoint(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func usesEndpoint(html, endpoint string) bool {
	quoted := regexp.QuoteMeta(endpoint)
	attrRef := quoted + `(?:[?#][^"']*)?`
	scriptRef := `["']` + quoted + `(?:[?#][^"']*)?["']`
	helperRef := `\bdialogEndpoint\s*\(\s*` + scriptRef + `\s*\)`
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<form\b[^>]*\b(?:action|formaction|hx-post)\s*=\s*["'][^"']*` + attrRef + `["']`),
		regexp.MustCompile(`(?is)\bfetch\s*\([^)]*(?:` + helperRef + `|` + scriptRef + `)[^)]*\)`),
		regexp.MustCompile(`(?is)\bnew\s+EventSource\s*\([^)]*(?:` + helperRef + `|` + scriptRef + `)[^)]*\)`),
		regexp.MustCompile(`(?is)\bnavigator\.sendBeacon\s*\([^)]*(?:` + helperRef + `|` + scriptRef + `)[^)]*\)`),
		regexp.MustCompile(`(?is)\.open\s*\([^)]*(?:` + helperRef + `|` + scriptRef + `)[^)]*\)`),
		regexp.MustCompile(`(?is)` + helperRef),
	}
	for _, pattern := range patterns {
		if pattern.MatchString(html) {
			return true
		}
	}
	return false
}
