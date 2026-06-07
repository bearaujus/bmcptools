package user

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

const defaultUserResponseMaxBytes = 0

func makeAskUserHandler(htmlSource string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		question := req.GetString("question", "")
		if strings.TrimSpace(question) == "" {
			return mcp.NewToolResultError("question is required"), nil
		}

		details := req.GetString("details", "")
		if strings.TrimSpace(details) == "" {
			return mcp.NewToolResultError("details is required; include the context/options the user needs to answer"), nil
		}
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

		token, err := newDialogToken()
		if err != nil {
			return mcp.NewToolResultError("failed to create dialog session: " + err.Error()), nil
		}
		act := &dialogActivity{}
		ctx, cancel := context.WithCancel(context.Background())
		state := &pendingDialogState{
			responseCh: make(chan string, 1),
			activity:   act,
			cancelFn:   cancel,
		}
		storePendingDialog(token, state)

		go func() {
			answer := runDialogBlocking(ctx, htmlSource, question, details, title, subtitle, choices, notify, timeout, token, act, state)
			cancel() // release context resources
			select {
			case state.responseCh <- answer:
			default:
			}
			state.scheduleCleanup(token, dialogResultRetention)
		}()

		return mcp.NewToolResultText(
			"{\n" +
				"  \"status\": \"PENDING\",\n" +
				"  \"token\": \"" + token + "\",\n" +
				"  \"instructions\": \"The browser dialog is already visible to the user; do not repeat the question in chat or a CLI. Call get_user_response(token=\\\"" + token + "\\\") to retrieve the answer. Each call waits up to wait_seconds (default 55) before returning PENDING again. Keep polling indefinitely — the user may take a long time to reply.\"\n" +
				"}",
		), nil
	}
}

func runDialogBlocking(ctx context.Context, htmlSource, question, details, title, subtitle string, choices []string, notify bool, timeout time.Duration, dialogToken string, activity *dialogActivity, state *pendingDialogState) string {
	if notify {
		msg := question
		if len(msg) > 120 {
			msg = truncateUTF8Bytes(msg, 120) + "..."
		}
		go sendNotificationFn(msg, title, "info", 10)
	}

	deadline := time.Now().Add(timeout)

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return "[User did not respond within the allotted time]"
	}

	answer, err := promptUser(ctx, htmlSource, question, details, title, subtitle, choices, remaining, dialogToken, activity, state)
	if err != nil {
		return fmt.Sprintf("[Failed to get user input: %v]", err)
	}

	if strings.TrimSpace(answer) != "" {
		return answer
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
	maxResponseBytes := int(req.GetFloat("max_response_bytes", defaultUserResponseMaxBytes))
	if maxResponseBytes < 0 {
		maxResponseBytes = defaultUserResponseMaxBytes
	}

	state := loadPendingDialog(token)
	if state == nil {
		return mcp.NewToolResultError(
			"unknown or expired token — the dialog may have already been answered or timed out",
		), nil
	}

	select {
	case answer := <-state.responseCh:
		releasePendingDialog(token, dialogResultRetention)
		return mcp.NewToolResultText(limitUserResponse(answer, maxResponseBytes)), nil
	case <-time.After(time.Duration(waitSec) * time.Second):
		return mcp.NewToolResultText(buildPendingMessage(token, state.activity)), nil
	}
}

func limitUserResponse(answer string, maxBytes int) string {
	if maxBytes <= 0 || len(answer) <= maxBytes {
		return answer
	}
	prefix := truncateUTF8Bytes(answer, maxBytes)
	return fmt.Sprintf("%s\n\n[User response truncated at %d/%d bytes. Increase max_response_bytes or set max_response_bytes=0 for unlimited.]",
		prefix, maxBytes, len(answer))
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	n := maxBytes
	for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
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

	outcome := state.activity.broadcastUpdate(message, replaceLast, dialogUpdateDeliveryWait)
	return mcp.NewToolResultText(formatDialogDeliveryOutcome(outcome)), nil
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

func formatDialogDeliveryOutcome(outcome dialogDeliveryOutcome) string {
	switch outcome.State {
	case dialogDeliveryNoSubscribers:
		return "message accepted, but no live browser client is currently connected"
	case dialogDeliveryDropped:
		return fmt.Sprintf("message dropped for %s", formatClientCount(outcome.Subscribers))
	case dialogDeliveryDelivered:
		msg := fmt.Sprintf("message delivered to %s", formatClientCount(outcome.Delivered))
		if outcome.Dropped > 0 {
			msg += fmt.Sprintf("; dropped for %s", formatClientCount(outcome.Dropped))
		}
		return msg
	default:
		msg := fmt.Sprintf("message queued for %s", formatClientCount(outcome.Queued))
		if outcome.Dropped > 0 {
			msg += fmt.Sprintf("; dropped for %s", formatClientCount(outcome.Dropped))
		}
		return msg
	}
}

func formatClientCount(n int) string {
	if n == 1 {
		return "1 live dialog client"
	}
	return fmt.Sprintf("%d live dialog clients", n)
}
