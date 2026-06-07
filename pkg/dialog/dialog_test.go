package dialog

import "testing"

func TestNewDialogTemplateAcceptsFetchUsage(t *testing.T) {
	tmpl, err := NewDialogTemplate(`
		<script>
		function dialogEndpoint(path){ return path; }
		fetch(dialogEndpoint('/answer'), {method:'POST', body:'{}'});
		</script>
	`)
	if err != nil {
		t.Fatalf("expected template to validate, got %v", err)
	}
	if tmpl.HTML() == "" {
		t.Fatal("expected template HTML to be preserved")
	}
}

func TestNewDialogTemplateRejectsCommentOnlyEndpointMention(t *testing.T) {
	_, err := NewDialogTemplate(`<!-- /answer --><script>fetch('/not-answer')</script>`)
	if err == nil {
		t.Fatal("expected comment-only endpoint mention to fail validation")
	}
}

func TestNewDialogTemplateRejectsLongerEndpointSuffix(t *testing.T) {
	_, err := NewDialogTemplate(`<script>fetch('/answer-submitted', {method:'POST'})</script>`)
	if err == nil {
		t.Fatal("expected longer endpoint suffix to fail validation")
	}
}

func TestNewRestTemplateAcceptsFormAction(t *testing.T) {
	tmpl, err := NewRestTemplate(`<form method="post" action="/answer"><button>Wake</button></form>`)
	if err != nil {
		t.Fatalf("expected rest template to validate, got %v", err)
	}
	if tmpl.HTML() == "" {
		t.Fatal("expected template HTML to be preserved")
	}
}
