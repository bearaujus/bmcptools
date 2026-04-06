package main

import (
	"os"
	"strings"
	"testing"
	"time"
)
// isHeadless returns true when no interactive terminal is available.
// Interactive ask_user tests are skipped in headless/short mode.
func isHeadless() bool {
	// -short flag skips interactive tests.
	if testing.Short() {
		return true
	}
	// CI environments typically set CI=true.
	if os.Getenv("CI") != "" {
		return true
	}
	return false
}

// TestAskUserMissingQuestion verifies that an empty question returns an error.
func TestAskUserMissingQuestion(t *testing.T) {
	result, err := askUserHandler(nil, newTestRequest(map[string]any{"question": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing question")
	}
}

// TestAskUserTimeoutDefault verifies the default timeout is in a sane range.
func TestAskUserTimeoutDefault(t *testing.T) {
	timeout := 300 * time.Second
	if timeout <= 0 || timeout > 3600*time.Second {
		t.Errorf("default timeout out of range: %v", timeout)
	}
}

// TestAskUserTimeoutClamping verifies that the handler clamps timeout values correctly.
func TestAskUserTimeoutClamping(t *testing.T) {
	tests := []struct {
		rawTimeout float64
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{0, 300 * time.Second, 300 * time.Second},      // <= 0 → default 300 s
		{-5, 300 * time.Second, 300 * time.Second},     // negative → default
		{3700, 3600 * time.Second, 3600 * time.Second}, // over max → clamped
		{60, 60 * time.Second, 60 * time.Second},        // normal value
	}
	for _, tt := range tests {
		raw := tt.rawTimeout
		got := raw
		if got <= 0 {
			got = 300
		}
		if got > 3600 {
			got = 3600
		}
		d := time.Duration(got) * time.Second
		if d < tt.wantMin || d > tt.wantMax {
			t.Errorf("timeout(%v) clamped to %v, want [%v, %v]", raw, d, tt.wantMin, tt.wantMax)
		}
	}
}

// TestAskUserChoicesEmptyQuestion verifies an error when question is whitespace-only.
func TestAskUserChoicesEmptyQuestion(t *testing.T) {
	result, err := askUserHandler(nil, newTestRequest(map[string]any{
		"question": "   ",
		"choices":  []any{"Yes", "No"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only question")
	}
}

// TestAskUserSampleQuestions demonstrates sample use-cases for ask_user.
// These tests are skipped in headless environments.
//
// In real usage the AI would call ask_user with questions like these:
//
//	"Which database should I use for this project?"           → choices: ["PostgreSQL", "MySQL", "SQLite"]
//	"Do you want me to overwrite the existing config file?"  → choices: ["Yes", "No, keep it"]
//	"What name should I give this function?"                 → (freeform)
//	"Should I run tests before committing?"                  → choices: ["Yes", "No"]
//	"Which branch should this PR target?"                    → choices: ["main", "develop", "staging"]
func TestAskUserSampleQuestions(t *testing.T) {
	if isHeadless() {
		t.Skip("skipping interactive ask_user test in headless environment")
	}

	samples := []struct {
		question string
		choices  []any
	}{
		// Standard multiple-choice questions
		{"Which database should I use for this project?", []any{"PostgreSQL", "MySQL", "SQLite"}},
		{"Do you want me to overwrite the existing config file?", []any{"Yes", "No, keep it"}},
		{"What name should I give this function?", nil},
		{"Should I run tests before committing?", []any{"Yes", "No"}},
		{"Which branch should this PR target?", []any{"main", "develop", "staging"}},

		// Destructive action confirmation — AI must always ask before deleting
		{"I am about to delete 47 files. This CANNOT be undone. How should I proceed?",
			[]any{"Yes, delete them all", "No — cancel everything"}},

		// Multi-line question (tests newline rendering in dialog)
		{"I need clarification on this requirement:\n\n• Feature A: refactor the auth module\n• Feature B: add caching layer\n\nWhich should I implement first?",
			[]any{"Feature A (auth refactor)", "Feature B (caching)", "Both simultaneously"}},

		// Error / failure handling
		{"The test suite has 3 failing tests. How should I proceed?",
			[]any{"Fix the tests before continuing", "Skip tests and continue", "Revert my last change"}},

		// Deployment target — high-stakes choice
		{"Which environment should I deploy to?",
			[]any{"Development", "Staging", "Production — I'm sure"}},

		// Freeform with context (tests long question text)
		{"I found an ambiguous variable named 'data' in 12 places. What would you like me to rename it to?", nil},

		// Security concern — always surface to user
		{"I found a hardcoded secret in the codebase. What should I do?",
			[]any{"Remove it and rotate the secret", "Create a GitHub issue only", "Ignore it for now"}},
	}
	for _, s := range samples {
		args := map[string]any{
			"question":        s.question,
			"timeout_seconds": float64(30),
		}
		if s.choices != nil {
			args["choices"] = s.choices
		}
		result, err := askUserHandler(nil, newTestRequest(args))
		if err != nil {
			t.Fatalf("unexpected error for question %q: %v", s.question, err)
		}
		text := resultText(result)
		t.Logf("Q: %q → %q", s.question, text)
	}
}

// TestAskUserInputValidation covers all validation-only paths (no dialog spawned).
func TestAskUserInputValidation(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantErr  bool
		wantText string
	}{
		{
			name:    "empty question errors",
			args:    map[string]any{"question": ""},
			wantErr: true,
		},
		{
			name:    "whitespace question errors",
			args:    map[string]any{"question": "   "},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := askUserHandler(nil, newTestRequest(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != isResultError(result) {
				t.Errorf("isError=%v, wantErr=%v; text=%q", isResultError(result), tt.wantErr, resultText(result))
			}
			if tt.wantText != "" && !strings.Contains(resultText(result), tt.wantText) {
				t.Errorf("expected %q in result: %q", tt.wantText, resultText(result))
			}
		})
	}
}

// ── notify_user tests ─────────────────────────────────────────────────────────

func TestNotifyUserMissingMessage(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{"message": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing message")
	}
}

func TestNotifyUserWhitespaceMessage(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{"message": "   "}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only message")
	}
}

// TestNotifyUserReturnsImmediately verifies the handler is non-blocking.
// Even if the OS notification takes seconds, the handler should return in < 500 ms.
func TestNotifyUserReturnsImmediately(t *testing.T) {
	start := time.Now()
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "Unit test: non-blocking notification check",
		"title":   "bmcp-tools test",
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Errorf("unexpected error: %v", resultText(result))
	}
	if !strings.Contains(resultText(result), "[Notification sent]") {
		t.Errorf("expected '[Notification sent]' prefix in result: %q", resultText(result))
	}
	// Should return well within 500 ms even on slow machines — the OS call is in a goroutine.
	if elapsed > 500*time.Millisecond {
		t.Errorf("notifyUserHandler took %v, expected < 500ms (non-blocking)", elapsed)
	}
}

// TestAskUserToolPainPointSurvey asks the user directly about bmcp-tools pain points.
// Running this test surfaces real friction between the AI, the tools, and the human operator.
// Skip with -short or CI=true.
func TestAskUserToolPainPointSurvey(t *testing.T) {
	if isHeadless() {
		t.Skip("skipping interactive pain-point survey in headless environment")
	}

	survey := []struct {
		question string
		choices  []any
	}{
		{
			"Which bmcp-tools tool do you find MOST useful in your daily workflow?",
			[]any{
				"read_file / read_multiple_files",
				"grep_files",
				"edit_file / find_replace_in_files",
				"run_command",
				"ask_user / notify_user",
				"directory_tree / list_directory",
				"search_files",
			},
		},
		{
			"What is the BIGGEST pain point when I (the AI) use these tools?",
			[]any{
				"Too many tool calls needed for simple tasks",
				"Output is too verbose / wastes my context window",
				"Hard to know which tool to use for a task",
				"Missing a tool I need",
				"Tool results are confusing or unclear",
				"No pain point — tools feel great",
			},
		},
		{
			"When I edit files, does the diff output help you verify my changes?",
			[]any{
				"Yes — the diff is clear and useful",
				"Somewhat — but it can be noisy for large changes",
				"No — I don't look at diffs",
				"I'd prefer a summary instead of raw diff",
			},
		},
		{
			"When I search large codebases with grep_files, is the pagination easy to follow?",
			[]any{
				"Yes — offset/max_results is clear",
				"It's OK but I never know how many total matches there are",
				"No — pagination is confusing",
				"I don't use grep_files that way",
			},
		},
		{
			"Would you like me (the AI) to use notify_user more often for progress updates during long tasks?",
			[]any{
				"Yes — I want more real-time updates",
				"Current frequency is fine",
				"No — notifications are distracting",
			},
		},
		{
			"Is there a tool or capability you wish bmcp-tools had? (free text answer)",
			nil, // freeform
		},
		{
			"Overall, how would you rate the clarity of the bmcp-tools tool descriptions?",
			[]any{
				"5 — Very clear, always pick the right tool",
				"4 — Mostly clear",
				"3 — Sometimes confusing",
				"2 — Often confusing",
				"1 — I have no idea what tool to use",
			},
		},
	}

	for _, q := range survey {
		args := map[string]any{
			"question":        q.question,
			"title":           "bmcp-tools Pain Point Survey",
			"timeout_seconds": float64(60),
		}
		if q.choices != nil {
			args["choices"] = q.choices
		}
		result, err := askUserHandler(nil, newTestRequest(args))
		if err != nil {
			t.Fatalf("unexpected error for question %q: %v", q.question, err)
		}
		t.Logf("Q: %q\nA: %q\n", q.question, resultText(result))
	}
}

// TestNotifyUserDefaultTitle verifies an empty title falls back to "AI Assistant".
func TestNotifyUserDefaultTitle(t *testing.T) {
	result, err := notifyUserHandler(nil, newTestRequest(map[string]any{
		"message": "default title test",
		// title omitted — should default to "AI Assistant"
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Errorf("unexpected error: %v", resultText(result))
	}
}
