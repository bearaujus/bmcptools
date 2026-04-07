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
	if !strings.Contains(text, "test notification") {
		t.Errorf("expected message in result: %q", text)
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
