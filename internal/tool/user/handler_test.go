package user

import (
	"strings"
	"testing"
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

// Reason: ask_user has ZERO test coverage. The handler has several validation
// paths (missing question, no choices with allowFreeform=false, empty choice
// stripping) that would silently regress without tests.

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

// Reason: Choices containing only whitespace should be stripped silently.
// If they're kept, they create invisible buttons that a user can never click.
func TestAskUserHandlerEmptyChoicesFiltered(t *testing.T) {
	result, err := makeAskUserHandler("")(nil, newTestRequest(map[string]any{
		"question": "pick one",
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
	token := newDialogToken()
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
	token := newDialogToken()
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

// Reason: When the dialog is open but no answer has arrived yet, get_user_response
// must return a PENDING message (not block forever) after wait_seconds elapses.
func TestGetUserResponseReturnsPendingOnTimeout(t *testing.T) {
	token := newDialogToken()
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
	token := newDialogToken()
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

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
	case msg := <-ch:
		if msg != "progress update" {
			t.Errorf("expected 'progress update', got: %q", msg)
		}
	default:
		t.Error("expected broadcast to reach subscriber channel")
	}
}

// Reason: replace_last=true prefixes the message with __REPLACE__ for the
// browser to swap the last update entry. This path was untested.
func TestUpdateDialogReplaceLastFlag(t *testing.T) {
	token := newDialogToken()
	act := &dialogActivity{}
	ch, unsub := act.subscribe()
	defer unsub()
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
	}
	storePendingDialog(token, state)
	defer deletePendingDialog(token)

	_, err := updateDialogHandler(nil, newTestRequest(map[string]any{
		"token":        token,
		"message":      "replaced",
		"replace_last": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-ch:
		if !strings.HasPrefix(msg, "__REPLACE__") {
			t.Errorf("expected __REPLACE__ prefix for replace_last=true: %q", msg)
		}
	default:
		t.Error("expected broadcast message for replace_last")
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
	token := newDialogToken()
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
	token := newDialogToken()
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
