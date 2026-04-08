package user

import (
	cryptorand "crypto/rand"
	"context"
	"fmt"
	"sync"
	"time"
)

// pendingDialogs stores state for pending ask_user sessions.
var pendingDialogs sync.Map

func storePendingDialog(token string, state *pendingDialogState) {
	pendingDialogs.Store(token, state)
}

func loadPendingDialog(token string) *pendingDialogState {
	val, ok := pendingDialogs.Load(token)
	if !ok {
		return nil
	}
	return val.(*pendingDialogState)
}

func deletePendingDialog(token string) {
	pendingDialogs.Delete(token)
}

// dialogActivity tracks real-time user activity reported by the browser dialog's heartbeat.
type dialogActivity struct {
	mu           sync.Mutex
	connected    bool
	typing       bool
	idleSec      float64
	lastBeat     time.Time
	outboundSubs []chan string
}

func (a *dialogActivity) update(typing bool, idleSec float64) {
	a.mu.Lock()
	a.connected = true
	a.typing = typing
	a.idleSec = idleSec
	a.lastBeat = time.Now()
	a.mu.Unlock()
}

func (a *dialogActivity) subscribe() (chan string, func()) {
	ch := make(chan string, 8)
	a.mu.Lock()
	a.outboundSubs = append(a.outboundSubs, ch)
	a.mu.Unlock()
	return ch, func() {
		a.mu.Lock()
		for i, s := range a.outboundSubs {
			if s == ch {
				a.outboundSubs = append(a.outboundSubs[:i], a.outboundSubs[i+1:]...)
				break
			}
		}
		a.mu.Unlock()
	}
}

func (a *dialogActivity) broadcast(msg string) {
	a.mu.Lock()
	for _, ch := range a.outboundSubs {
		select {
		case ch <- msg:
		default:
		}
	}
	a.mu.Unlock()
}

type pendingDialogState struct {
	responseCh chan string
	activity   *dialogActivity
	cancelFn   context.CancelFunc // non-nil for ask_user dialogs; call to cancel the open dialog
}

func newDialogToken() string {
	b := make([]byte, 8)
	if _, err := cryptorand.Read(b); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%016x", b)
}
