package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func makeAskUserHandler(htmlSource string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question := req.GetString("question", "")
	if strings.TrimSpace(question) == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	details := req.GetString("details", "")
	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	subtitle := req.GetString("subtitle", "")

	choices := req.GetStringSlice("choices", nil)
	// Strip empty strings — empty chip choices produce unclickable buttons
	// and cause silent empty-answer retry loops.
	filtered := choices[:0]
	for _, c := range choices {
		if strings.TrimSpace(c) != "" {
			filtered = append(filtered, c)
		}
	}
	choices = filtered
	timeoutSec := req.GetFloat("timeout_seconds", 600)
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	timeout := time.Duration(timeoutSec) * time.Second

	notify := req.GetBool("notify", true)

	token := newDialogToken()
	act := &dialogActivity{}
	ctx, cancel := context.WithCancel(context.Background())
	state := &pendingDialogState{
		responseCh: make(chan string, 1),
		activity:   act,
		cancelFn:   cancel,
	}
	storePendingDialog(token, state)

	go func() {
			answer := runDialogBlocking(ctx, htmlSource, question, details, title, subtitle, choices, notify, timeout, act)
		cancel() // release context resources
		select {
		case state.responseCh <- answer:
		default:
		}
		time.Sleep(5 * time.Minute)
		deletePendingDialog(token)
	}()

		return mcp.NewToolResultText(
			"{\n" +
				"  \"status\": \"PENDING\",\n" +
				"  \"token\": \"" + token + "\",\n" +
				"  \"instructions\": \"Call get_user_response(token=\\\"" + token + "\\\") to retrieve the answer. Each call waits up to wait_seconds (default 55) before returning PENDING again. Keep polling indefinitely — the user may take a long time to reply.\"\n" +
				"}",
		), nil
	}
}

func runDialogBlocking(ctx context.Context, htmlSource, question, details, title, subtitle string, choices []string, notify bool, timeout time.Duration, activity *dialogActivity) string {
	if notify {
		msg := question
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		go sendNotificationFn(msg, title, "info", 10)
	}

	deadline := time.Now().Add(timeout)

	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "[User did not respond within the allotted time]"
		}

		answer, err := promptUser(ctx, htmlSource, question, details, title, subtitle, choices, remaining, activity)
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

	state := loadPendingDialog(token)
	if state == nil {
		return mcp.NewToolResultError(
			"unknown or expired token — the dialog may have already been answered or timed out",
		), nil
	}

	select {
	case answer := <-state.responseCh:
		deletePendingDialog(token)
		return mcp.NewToolResultText(answer), nil
	case <-time.After(time.Duration(waitSec) * time.Second):
		return mcp.NewToolResultText(buildPendingMessage(token, state.activity)), nil
	}
}

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
	if !lastBeat.IsZero() && beatAge > 20 {
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

func updateDialogHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}
	message := req.GetString("message", "")

	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}
	replaceLast := req.GetBool("replace_last", false)

	state := loadPendingDialog(token)
	if state == nil {
		return mcp.NewToolResultError("unknown or expired token — dialog may have been answered or timed out"), nil
	}
	if state.activity == nil {
		return mcp.NewToolResultError("this dialog does not support live updates"), nil
	}

	if replaceLast {
		state.activity.broadcast("__REPLACE__" + message)
	} else {
		state.activity.broadcast(message)
	}
	return mcp.NewToolResultText("message delivered to dialog"), nil
}

func cancelAskUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token := req.GetString("token", "")
	if strings.TrimSpace(token) == "" {
		return mcp.NewToolResultError("token is required"), nil
	}

	state := loadPendingDialog(token)
	if state == nil {
		return mcp.NewToolResultError("unknown or expired token — dialog may have been answered or timed out"), nil
	}
	if state.cancelFn == nil {
		return mcp.NewToolResultError("this dialog does not support cancellation"), nil
	}

	state.cancelFn()
	return mcp.NewToolResultText("dialog cancelled — browser will dismiss and get_user_response will return a cancellation message"), nil
}
