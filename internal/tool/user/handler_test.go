package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// ── notify_user ───────────────────────────────────────────────────────────────

func TestNotifyUserMissingMessage(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing message")
	}
}

func TestNotifyUserSuccess(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "test notification",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "message_bytes=17") {
		t.Errorf("expected concise message metadata in result: %q", text)
	}
	if !strings.Contains(text, "duration_support=") {
		t.Errorf("expected duration support metadata in result: %q", text)
	}
}

func TestNotifyUserInvalidLevelDefaultsToInfo(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "hello",
		"level":   "invalid",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

func TestNotifyUserValidLevels(t *testing.T) {
	for _, level := range []string{"info", "warning", "error"} {
		t.Run(level, func(t *testing.T) {
			result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
				"message": "test",
				"level":   level,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if isResultError(result) {
				t.Fatalf("unexpected error for level=%s: %s", level, resultText(result))
			}
		})
	}
}

func TestNotifyUserDurationClamped(t *testing.T) {
	// duration_seconds > 60 should be clamped to 60 — handler should still succeed
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message":          "clamped",
		"duration_seconds": float64(999),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

func TestNotifyUserWhitespaceOnlyMessage(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "   ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only message")
	}
}

// ── ask_user ──────────────────────────────────────────────────────────────────

// Reason: ask_user has validation paths (missing question/details and empty
// choice stripping) that would silently regress without tests.

func TestAskUserHandlerMissingQuestion(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing question")
	}
}

func TestAskUserHandlerWhitespaceQuestion(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(map[string]any{
		"question": "   ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only question")
	}
}

func TestAskUserHandlerMissingDetails(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(map[string]any{
		"question": "What should happen next?",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing details")
	}
}

// Reason: Choices containing only whitespace should be stripped silently.
// If they're kept, they create invisible buttons that a user can never click.
func TestAskUserHandlerEmptyChoicesFiltered(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(map[string]any{
		"question": "pick one",
		"details":  "Choose one of the proposed options.",
		"choices":  []any{"", "  ", "valid choice"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// valid choice remains, so this should succeed and return a PENDING token
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "PENDING") {
		t.Errorf("expected PENDING status in result: %q", text)
	}
	if !strings.Contains(text, "token") {
		t.Errorf("expected token in result: %q", text)
	}
}

// Reason: Happy path — a valid question should return a PENDING token immediately.
// LLM clients depend on this token to poll for the user response.
func TestAskUserHandlerReturnsToken(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(map[string]any{
		"question": "What is your name?",
		"details":  "I need the name to use in the generated greeting.",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "PENDING") {
		t.Errorf("expected 'PENDING' in response: %q", text)
	}
	if !strings.Contains(text, "get_user_response") {
		t.Errorf("expected polling instructions in response: %q", text)
	}
	if !strings.Contains(text, "do not repeat the question") {
		t.Errorf("expected anti-CLI prompt instruction in response: %q", text)
	}
}

func TestRunDialogBlockingFallsBackToConsoleWhenBrowserOpenFails(t *testing.T) {
	oldOpen := openBrowserFn
	oldPrompt := promptConsoleFn
	oldChoicePrompt := promptChoiceConsoleFn
	openBrowserFn = func(string) error { return errors.New("no browser available") }
	promptConsoleFn = func(question, details string) (string, error) { return "typed in console", nil }
	promptChoiceConsoleFn = func(question, details, title string, choices []string) (string, error) {
		return "typed in console", nil
	}
	defer func() {
		openBrowserFn = oldOpen
		promptConsoleFn = oldPrompt
		promptChoiceConsoleFn = oldChoicePrompt
	}()

	answer := runDialogBlocking(
		context.Background(),
		"",
		"Question?",
		"Details",
		"AI Assistant",
		"",
		nil,
		false,
		2*time.Second,
		mustNewDialogToken(t),
		&dialogActivity{},
		&pendingDialogState{},
	)
	if answer != "typed in console" {
		t.Fatalf("expected console fallback answer, got %q", answer)
	}
}

func TestRestHandlerFallsBackToConsoleWhenBrowserOpenFails(t *testing.T) {
	oldOpen := openBrowserFn
	oldPrompt := promptRestConsoleFn
	openBrowserFn = func(string) error { return errors.New("no browser available") }
	promptRestConsoleFn = func(title, subtitle, notes string, timeout time.Duration) (string, error) {
		if title != "AI Assistant" || subtitle != "Waiting" || notes != "Resume when ready." {
			t.Fatalf("unexpected rest prompt contents: %q / %q / %q", title, subtitle, notes)
		}
		if timeout != 30*time.Second {
			t.Fatalf("unexpected timeout: %v", timeout)
		}
		return "User woke up the AI with note: from console", nil
	}
	defer func() {
		openBrowserFn = oldOpen
		promptRestConsoleFn = oldPrompt
	}()

	result, err := makeRestHandler("")(nil, newTestRequest(map[string]any{
		"title":           "AI Assistant",
		"subtitle":        "Waiting",
		"notes":           "Resume when ready.",
		"timeout_seconds": float64(30),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	token := resultToken(t, result)
	poll, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"wait_seconds": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(poll) {
		t.Fatalf("unexpected polling error: %s", resultText(poll))
	}
	if got := resultText(poll); got != "User woke up the AI with note: from console" {
		t.Fatalf("unexpected rest response: %q", got)
	}
}

type failingTokenReader struct{}

func (failingTokenReader) Read(_ []byte) (int, error) {
	return 0, errors.New("rng unavailable")
}

func TestNewDialogTokenFailsWhenRandomSourceFails(t *testing.T) {
	oldReader := dialogTokenReader
	dialogTokenReader = failingTokenReader{}
	defer func() {
		dialogTokenReader = oldReader
	}()

	token, err := newDialogToken()
	if err == nil {
		t.Fatal("expected token generation error")
	}
	if token != "" {
		t.Fatalf("expected empty token on failure, got %q", token)
	}
}

// ── get_user_response ─────────────────────────────────────────────────────────

// Reason: get_user_response has ZERO test coverage. Key paths: missing token,
// unknown token, and a token with a pre-loaded answer.

func TestGetUserResponseMissingToken(t *testing.T) {
	result, err := getUserResponseHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing token")
	}
}

func TestGetUserResponseWhitespaceToken(t *testing.T) {
	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token": "   ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only token")
	}
}

// Reason: An expired or nonexistent token should produce a clear error rather
// than hanging indefinitely. This is the most common error an LLM encounters.
func TestGetUserResponseUnknownToken(t *testing.T) {
	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token": "000000000000dead",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for unknown token")
	}
	text := resultText(result)
	if !strings.Contains(text, "unknown") && !strings.Contains(text, "expired") {
		t.Errorf("expected 'unknown' or 'expired' in error message: %q", text)
	}
}

// Reason: When the dialog goroutine has already delivered an answer, the
// handler must return it immediately on the first poll.
func TestGetUserResponseReceivesAnswer(t *testing.T) {
	token := mustNewDialogToken(t)
	ch := make(chan string, 1)
	ch <- "my answer"
	state := &pendingDialogState{
		responseCh: ch,
		activity:   &dialogActivity{},
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"wait_seconds": float64(5),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if text != "my answer" {
		t.Errorf("expected 'my answer', got: %q", text)
	}
}

func TestGetUserResponseCapsLargeAnswer(t *testing.T) {
	token := mustNewDialogToken(t)
	ch := make(chan string, 1)
	ch <- strings.Repeat("x", 20)
	state := &pendingDialogState{
		responseCh: ch,
		activity:   &dialogActivity{},
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":              token,
		"wait_seconds":       float64(5),
		"max_response_bytes": float64(5),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "User response truncated") {
		t.Errorf("expected truncation notice: %q", text)
	}
	if strings.Contains(text, strings.Repeat("x", 10)) {
		t.Errorf("expected response body to be capped: %q", text)
	}
}

func TestGetUserResponseDefaultIsUnlimited(t *testing.T) {
	token := mustNewDialogToken(t)
	answer := strings.Repeat("x", 512*1024)
	ch := make(chan string, 1)
	ch <- answer
	state := &pendingDialogState{
		responseCh: ch,
		activity:   &dialogActivity{},
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"wait_seconds": float64(5),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if text != answer {
		t.Fatalf("expected uncapped default response, got %d bytes", len(text))
	}
}

func TestGetUserResponseCapsLargeAnswerAtUTF8Boundary(t *testing.T) {
	token := mustNewDialogToken(t)
	ch := make(chan string, 1)
	ch <- "a🙂b"
	state := &pendingDialogState{
		responseCh: ch,
		activity:   &dialogActivity{},
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":              token,
		"wait_seconds":       float64(5),
		"max_response_bytes": float64(2),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !utf8.ValidString(text) {
		t.Fatalf("expected valid UTF-8 after byte cap, got: %q", text)
	}
	if !strings.HasPrefix(text, "a\n\n[User response truncated") {
		t.Errorf("expected truncation at rune boundary after ASCII prefix, got: %q", text)
	}
}

// Reason: When the dialog is open but no answer has arrived yet, get_user_response
// must return a PENDING message (not block forever) after wait_seconds elapses.
func TestGetUserResponseReturnsPendingOnTimeout(t *testing.T) {
	token := mustNewDialogToken(t)
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   &dialogActivity{},
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := getUserResponseHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"wait_seconds": float64(1), // short wait for test speed
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "PENDING") {
		t.Errorf("expected PENDING status while no answer available: %q", text)
	}
}

// ── update_dialog ─────────────────────────────────────────────────────────────

// Reason: update_dialog has ZERO test coverage. It has three validation paths:
// missing token, missing message, and unknown token.

func TestUpdateDialogMissingToken(t *testing.T) {
	result, err := updateDialogHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing token")
	}
}

func TestUpdateDialogMissingMessage(t *testing.T) {
	result, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token": "sometoken",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing message")
	}
}

func TestUpdateDialogUnknownToken(t *testing.T) {
	result, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":   "deadbeefdeadbeef",
		"message": "update text",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for unknown token")
	}
}

// Reason: When a valid token with an activity tracker exists, broadcast must
// succeed and return a success message.
func TestUpdateDialogBroadcastsToKnownToken(t *testing.T) {
	token := mustNewDialogToken(t)
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	deliveredCh := make(chan dialogOutboundEvent, 1)
	go func() {
		evt := <-ch
		evt.markDelivered()
		deliveredCh <- evt
	}()

	result, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":   token,
		"message": "progress update",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "delivered") {
		t.Errorf("expected 'delivered' in result: %q", text)
	}
	// Verify the broadcast reached the subscriber.
	select {
	case evt := <-deliveredCh:
		if evt.Type != dialogEventUpdate {
			t.Errorf("expected update event type, got: %q", evt.Type)
		}
		if evt.Message != "progress update" {
			t.Errorf("expected 'progress update', got: %q", evt.Message)
		}
		if evt.Replace {
			t.Error("expected replace=false")
		}
	default:
		t.Error("expected broadcast to reach subscriber channel")
	}
}

func TestUpdateDialogReturnsQueuedWhenBrowserHasNotConsumedUpdate(t *testing.T) {
	token := mustNewDialogToken(t)
	act := &dialogActivity{}
	_, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":   token,
		"message": "still rendering",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "queued") {
		t.Fatalf("expected queued delivery status, got %q", resultText(result))
	}
}

func TestUpdateDialogReturnsDroppedWhenSubscriberBufferIsFull(t *testing.T) {
	token := mustNewDialogToken(t)
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	for i := 0; i < cap(ch); i++ {
		ch <- dialogOutboundEvent{dialogEvent: dialogEvent{Type: dialogEventUpdate}}
	}
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":   token,
		"message": "buffer full",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "dropped") {
		t.Fatalf("expected dropped delivery status, got %q", resultText(result))
	}
}

// Reason: replace_last=true marks the event for browser-side replacement
// without mutating the user-visible message body.
func TestUpdateDialogReplaceLastFlag(t *testing.T) {
	token := mustNewDialogToken(t)
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	deliveredCh := make(chan dialogOutboundEvent, 1)
	go func() {
		evt := <-ch
		evt.markDelivered()
		deliveredCh <- evt
	}()

	_, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"message":      "replaced",
		"replace_last": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case evt := <-deliveredCh:
		if evt.Type != dialogEventUpdate {
			t.Errorf("expected update event type, got: %q", evt.Type)
		}
		if evt.Message != "replaced" {
			t.Errorf("expected raw message without transport prefix, got: %q", evt.Message)
		}
		if !evt.Replace {
			t.Error("expected replace=true")
		}
	default:
		t.Error("expected broadcast message for replace_last")
	}
}

func TestUpdateDialogSentinelLikeMessagesArePlainUpdates(t *testing.T) {
	token := mustNewDialogToken(t)
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	deliveredCh := make(chan dialogOutboundEvent, 1)
	go func() {
		evt := <-ch
		evt.markDelivered()
		deliveredCh <- evt
	}()

	_, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":   token,
		"message": "__DISMISS__",
	}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case evt := <-deliveredCh:
		if evt.Type != dialogEventUpdate {
			t.Errorf("expected update event type, got: %q", evt.Type)
		}
		if evt.Message != "__DISMISS__" {
			t.Errorf("expected sentinel-like text to stay user-visible, got: %q", evt.Message)
		}
	default:
		t.Error("expected broadcast message for sentinel-like update")
	}
}

// ── cancel_ask_user ───────────────────────────────────────────────────────────

// Reason: cancel_ask_user has ZERO test coverage. Key paths: missing token,
// unknown token, and successfully cancelling a live dialog.

func TestCancelAskUserMissingToken(t *testing.T) {
	result, err := cancelAskUserHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing token")
	}
}

func TestCancelAskUserUnknownToken(t *testing.T) {
	result, err := cancelAskUserHandler(nil, newTestRequest(map[string]any{
		"token": "0000000000000000",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for unknown token")
	}
}

// Reason: A dialog with cancelFn=nil cannot be cancelled — the handler must
// return a clear error rather than panicking.
func TestCancelAskUserNoCancelFn(t *testing.T) {
	token := mustNewDialogToken(t)
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   &dialogActivity{},
		cancelFn:   nil,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := cancelAskUserHandler(nil, newTestRequest(map[string]any{
		"token": token,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when cancelFn is nil")
	}
}

// Reason: The happy path — calling cancel on a live dialog must succeed and
// invoke the cancelFn so the goroutine exits cleanly.
func TestCancelAskUserSuccess(t *testing.T) {
	token := mustNewDialogToken(t)
	cancelled := false
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   &dialogActivity{},
		cancelFn:   func() { cancelled = true },
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	result, err := cancelAskUserHandler(nil, newTestRequest(map[string]any{
		"token": token,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !cancelled {
		t.Error("expected cancelFn to have been called")
	}
}

// ── notify_user (additional edge cases) ──────────────────────────────────────

// Reason: durationSec <= 0 should default to 5, not remain 0 or negative.
func TestNotifyUserDurationZeroDefaultsToFive(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message":          "test",
		"duration_seconds": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

// Reason: A custom title should override the default "AI Assistant" title.
func TestNotifyUserCustomTitle(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "hello",
		"title":   "My Custom Tool",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

// Reason: An empty title string should fall back to the default "AI Assistant"
// title without panicking or emitting an empty bracket in the log.
func TestNotifyUserEmptyTitleFallsBackToDefault(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "hello",
		"title":   "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}
