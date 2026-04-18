package binance

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/bearaujus/bmcptools/pkg/confirm"
)

// confirmOrSkip opens the confirmation dialog with the given markdown brief.
//   - returns (true, nil)  to proceed
//   - returns (false, nil) when the user rejects / the dialog times out
//   - returns (false, err) on platform / infra failure
// When BINANCE_SKIP_ASK_USER=true the dialog is bypassed and (true, nil) is returned.
func confirmOrSkip(ctx context.Context, title, brief string) (bool, error) {
	if skipAskUser() {
		return true, nil
	}
	if runtime.GOOS == "linux" {
		return false, fmt.Errorf("confirmation dialog is not supported on linux; set %s=true to bypass (at your own risk)", envSkipAskUser)
	}
	return confirm.Ask(ctx, title, brief)
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
