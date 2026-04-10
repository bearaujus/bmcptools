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
//	ChatTemplate   — HTML must reference /events (SSE) and POST to /message
//	                  {"text": string}
//	RestTemplate   — HTML must POST JSON to /answer
//	                  {"notes": string}
package dialog

import (
	"fmt"
	"strings"
)

// DialogTemplate is a validated HTML template for ask_user / confirm dialogs.
// The HTML must POST JSON to the /answer endpoint.
type DialogTemplate struct{ html string }

// NewDialogTemplate validates html and returns a DialogTemplate.
// Returns an error if the HTML does not reference the /answer endpoint.
func NewDialogTemplate(html string) (DialogTemplate, error) {
	if err := requireEndpoints(html, "/answer"); err != nil {
		return DialogTemplate{}, fmt.Errorf("dialog template: %w", err)
	}
	return DialogTemplate{html: html}, nil
}

// HTML returns the validated HTML content.
func (t DialogTemplate) HTML() string { return t.html }

// ChatTemplate is a validated HTML template for open_chat dialogs.
// The HTML must reference the /events SSE stream and POST to /message.
type ChatTemplate struct{ html string }

// NewChatTemplate validates html and returns a ChatTemplate.
// Returns an error if the HTML does not reference /events and /message.
func NewChatTemplate(html string) (ChatTemplate, error) {
	if err := requireEndpoints(html, "/events", "/message"); err != nil {
		return ChatTemplate{}, fmt.Errorf("chat template: %w", err)
	}
	return ChatTemplate{html: html}, nil
}

// HTML returns the validated HTML content.
func (t ChatTemplate) HTML() string { return t.html }

// RestTemplate is a validated HTML template for the rest/AFK page.
// The HTML must POST JSON to the /answer endpoint.
type RestTemplate struct{ html string }

// NewRestTemplate validates html and returns a RestTemplate.
// Returns an error if the HTML does not reference /answer.
func NewRestTemplate(html string) (RestTemplate, error) {
	if err := requireEndpoints(html, "/answer"); err != nil {
		return RestTemplate{}, fmt.Errorf("rest template: %w", err)
	}
	return RestTemplate{html: html}, nil
}

// HTML returns the validated HTML content.
func (t RestTemplate) HTML() string { return t.html }

// requireEndpoints returns an error when html does not contain every listed
// endpoint path as a substring. This is intentionally a string-scan, not an
// HTML parser — it is a lightweight safety net, not a full validator.
// Note: a false positive is possible if an endpoint string appears in a comment
// or longer path (e.g. "/answer-submitted" satisfies the "/answer" check).
func requireEndpoints(html string, endpoints ...string) error {
	var missing []string
	for _, ep := range endpoints {
		if !strings.Contains(html, ep) {
			missing = append(missing, ep)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("HTML must reference endpoint(s): %s", strings.Join(missing, ", "))
	}
	return nil
}
