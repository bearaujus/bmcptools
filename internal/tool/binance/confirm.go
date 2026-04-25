package binance

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/bearaujus/bmcptools/pkg/confirm"
)

// editParam is a local alias so handlers can build editable-param slices without
// importing the confirm package directly.
type editParam = confirm.EditableParam

type confirmResult int8

const (
	confirmHumanApproved confirmResult = iota
	confirmAutoApproved
	confirmHumanRejected
	confirmTimedOut
)

// prefix returns the string to prepend to a tool result so the AI knows how
// the confirmation was resolved. Empty for rejection/timeout (callers return
// errors instead).
func (r confirmResult) prefix() string {
	switch r {
	case confirmHumanApproved:
		return "Confirmed by user (human approval).\n\n"
	case confirmAutoApproved:
		return "Auto-approved (BINANCE_SKIP_ASK_USER=true — no human confirmation was required).\n\n"
	default:
		return ""
	}
}

// confirmOrSkip opens the confirmation dialog and returns the resolution:
//   - (confirmHumanApproved, nil)  — user clicked Confirm
//   - (confirmAutoApproved,  nil)  — BINANCE_SKIP_ASK_USER bypassed dialog
//   - (confirmHumanRejected, nil)  — user clicked Cancel
//   - (confirmTimedOut,      nil)  — dialog auto-cancelled after timeout
//   - (0,                    err)  — platform unsupported or infra failure
func confirmOrSkip(ctx context.Context, title, brief string) (confirmResult, error) {
	if skipAskUser() {
		return confirmAutoApproved, nil
	}
	if runtime.GOOS == "linux" {
		return 0, fmt.Errorf("confirmation dialog is not supported on linux; set %s=true to bypass (at your own risk)", envSkipAskUser)
	}
	ok, _, err := confirm.Ask(ctx, title, brief, confirm.WithTheme(confirm.ThemeBinance))
	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			return confirmTimedOut, nil
		}
		return 0, err
	}
	if ok {
		return confirmHumanApproved, nil
	}
	return confirmHumanRejected, nil
}

// confirmOrSkipEditable is like confirmOrSkip but shows user-editable parameter
// fields in the dialog. The returned map contains final values (possibly modified
// by the user) keyed by editParam.Key. When auto-approved, the original values
// from params are returned unchanged.
func confirmOrSkipEditable(ctx context.Context, title, brief string, params []editParam) (confirmResult, map[string]string, error) {
	if skipAskUser() {
		orig := make(map[string]string, len(params))
		for _, p := range params {
			orig[p.Key] = p.Value
		}
		return confirmAutoApproved, orig, nil
	}
	if runtime.GOOS == "linux" {
		return 0, nil, fmt.Errorf("confirmation dialog is not supported on linux; set %s=true to bypass (at your own risk)", envSkipAskUser)
	}
	ok, editedParams, err := confirm.Ask(ctx, title, brief,
		confirm.WithTheme(confirm.ThemeBinance),
		confirm.WithEditableParams(params),
	)
	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			return confirmTimedOut, nil, nil
		}
		return 0, nil, err
	}
	if ok {
		return confirmHumanApproved, editedParams, nil
	}
	return confirmHumanRejected, nil, nil
}

// editedFloat returns the user-edited float for key from params, falling back
// to original when the key is absent, empty, non-numeric, or not positive.
func editedFloat(params map[string]string, key string, original float64) float64 {
	v, ok := params[key]
	if !ok || strings.TrimSpace(v) == "" {
		return original
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 {
		return original
	}
	return f
}

// editedFloatStr returns the user-edited numeric string for key from params,
// falling back to original when absent, empty, or non-numeric.
func editedFloatStr(params map[string]string, key, original string) string {
	v, ok := params[key]
	if !ok || strings.TrimSpace(v) == "" {
		return original
	}
	v = strings.TrimSpace(v)
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return original
	}
	return v
}

type tradeBrief struct {
	Action    string
	Leverage  string
	Notional  string
	Risk      []string
	Market    []string
	Reasoning string
}

func (b tradeBrief) Render() string {
	var sb strings.Builder
	sb.WriteString("### Proposed action\n")
	sb.WriteString(b.Action)
	sb.WriteString("\n")
	if b.Leverage != "" || b.Notional != "" {
		sb.WriteString(fmt.Sprintf("Leverage: %s   Notional: %s\n", dash(b.Leverage), dash(b.Notional)))
	}
	if len(b.Risk) > 0 {
		sb.WriteString("\n### Risk\n")
		for _, r := range b.Risk {
			sb.WriteString(r)
			sb.WriteString("\n")
		}
	}
	if len(b.Market) > 0 {
		sb.WriteString("\n### Market snapshot\n")
		for _, m := range b.Market {
			sb.WriteString(m)
			sb.WriteString("\n")
		}
	}
	if strings.TrimSpace(b.Reasoning) != "" {
		sb.WriteString("\n### AI reasoning\n")
		sb.WriteString(b.Reasoning)
		sb.WriteString("\n")
	}
	return sb.String()
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "\u2014"
	}
	return s
}
