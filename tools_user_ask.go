package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func askUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question := req.GetString("question", "")
	if strings.TrimSpace(question) == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	subtitle := req.GetString("subtitle", "")

	choices := req.GetStringSlice("choices", nil)
	allowFreeform := req.GetBool("allow_freeform", true)

	timeoutSec := req.GetFloat("timeout_seconds", 600)
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	timeout := time.Duration(timeoutSec) * time.Second

	notify := req.GetBool("notify", true)
	nonBlocking := req.GetBool("non_blocking", false)

	// Non-blocking mode: open the dialog in a goroutine and return a poll token immediately.
	// This avoids MCP client request timeouts (typically 30–120 s) when waiting for user input.
	if nonBlocking {
		token := newDialogToken()
		act := &dialogActivity{}
		state := &pendingDialogState{
			responseCh: make(chan string, 1),
			activity:   act,
		}
		pendingDialogs.Store(token, state)

		go func() {
			answer := runDialogBlocking(question, title, subtitle, choices, allowFreeform, notify, timeout, act)
			select {
			case state.responseCh <- answer:
			default:
			}
			// Clean up after a grace period in case get_user_response is never called.
			time.Sleep(5 * time.Minute)
			pendingDialogs.Delete(token)
		}()

		return mcp.NewToolResultText(
			"PENDING — dialog opened in background.\n" +
				"Token: " + token + "\n" +
				"Call get_user_response(token=\"" + token + "\") to retrieve the answer.\n" +
				"Each get_user_response call waits up to wait_seconds (default 55) before returning PENDING again.",
		), nil
	}

	// Blocking mode: serialize so at most one dialog is visible at a time.
	askUserMu.Lock()
	defer askUserMu.Unlock()

	return mcp.NewToolResultText(runDialogBlocking(question, title, subtitle, choices, allowFreeform, notify, timeout, nil)), nil
}

// runDialogBlocking drives the dialog loop and returns the user's answer (or a timeout message).
// activity is non-nil only for non-blocking dialogs — it gets threaded into promptBrowser
// so the local HTTP server can populate it from JavaScript heartbeats.
func runDialogBlocking(question, title, subtitle string, choices []string, allowFreeform, notify bool, timeout time.Duration, activity *dialogActivity) string {
	if notify {
		msg := question
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		go sendNotification(msg, title, "info", 10)
	}

	deadline := time.Now().Add(timeout)

	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "[User did not respond within the allotted time]"
		}

		var answer string
		var err error
		if len(choices) > 0 {
			answer, err = promptUserChoice(question, title, subtitle, allowFreeform, choices, remaining, activity)
		} else {
			answer, err = promptUser(question, title, subtitle, remaining, activity)
		}
		if err != nil {
			return fmt.Sprintf("[Failed to get user input: %v]", err)
		}

		if strings.TrimSpace(answer) != "" {
			return answer
		}

		if attempt < maxRetries-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return "[User did not respond within the allotted time]"
}

// getUserResponseHandler polls for the result of a non-blocking ask_user dialog.
func getUserResponseHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}

	waitSec := req.GetFloat("wait_seconds", 55)
	if waitSec <= 0 {
		waitSec = 55
	}
	if waitSec > 115 {
		waitSec = 115
	}

	val, ok := pendingDialogs.Load(token)
	if !ok {
		return mcp.NewToolResultError(
			"unknown or expired token — the dialog may have already been answered or timed out",
		), nil
	}
	state := val.(*pendingDialogState)

	select {
	case answer := <-state.responseCh:
		pendingDialogs.Delete(token)
		return mcp.NewToolResultText(answer), nil
	case <-time.After(time.Duration(waitSec) * time.Second):
		return mcp.NewToolResultText(buildPendingMessage(token, state.activity)), nil
	}
}

// buildPendingMessage composes a status-aware PENDING message for get_user_response.
func buildPendingMessage(token string, act *dialogActivity) string {
	callAgain := fmt.Sprintf("Call get_user_response(token=%q) to continue waiting.", token)

	if act == nil {
		return "PENDING — user has not responded yet.\n" + callAgain
	}

	act.mu.Lock()
	connected := act.connected
	typing := act.typing
	idleSec := act.idleSec
	lastBeat := act.lastBeat
	act.mu.Unlock()

	if !connected {
		return "PENDING — dialog has been opened. Waiting for user to load the page.\n" + callAgain
	}

	beatAge := time.Since(lastBeat).Seconds()
	if beatAge > 20 {
		return fmt.Sprintf(
			"PENDING — browser connection appears lost (last heartbeat was %.0fs ago). "+
				"The user may have closed the tab. "+
				"Consider calling notify_user to re-alert them, then %s",
			beatAge, callAgain,
		)
	}

	if typing {
		return fmt.Sprintf("PENDING — user is actively composing a reply. %s", callAgain)
	}

	if idleSec > 90 {
		return fmt.Sprintf(
			"PENDING — user has been idle for %.0fs (dialog open but no interaction). "+
				"Consider calling notify_user to remind them, then %s",
			idleSec, callAgain,
		)
	}

	return fmt.Sprintf(
		"PENDING — user has the dialog open (last activity %.0fs ago). %s",
		idleSec, callAgain,
	)
}

// updateDialogHandler broadcasts a live message into an open non-blocking ask_user dialog.
func updateDialogHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}
	message := req.GetString("message", "")
	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	val, ok := pendingDialogs.Load(token)
	if !ok {
		return mcp.NewToolResultError("unknown or expired token — dialog may have been answered or timed out"), nil
	}
	state := val.(*pendingDialogState)
	if state.activity == nil {
		return mcp.NewToolResultError("this dialog does not support live updates"), nil
	}

	state.activity.broadcast(message)
	return mcp.NewToolResultText("message delivered to dialog"), nil
}
