package main

import (
	cryptorand "crypto/rand"
	"fmt"
	"sync"
	"time"
)

// askUserMu ensures at most one ask_user dialog is visible at a time.
// Concurrent calls queue and are served in arrival order.
var askUserMu sync.Mutex

// pendingDialogs stores state for non-blocking ask_user sessions.
// Key: token string → Value: *pendingDialogState
var pendingDialogs sync.Map

// dialogActivity tracks real-time user activity reported by the browser dialog's heartbeat.
type dialogActivity struct {
	mu           sync.Mutex
	connected    bool         // browser has loaded the page at least once
	typing       bool         // user has non-empty text in the input
	idleSec      float64      // seconds since last browser interaction
	lastBeat     time.Time    // time of last /heartbeat POST
	outboundSubs []chan string // live SSE subscriber channels (AI → browser)
}

func (a *dialogActivity) update(typing bool, idleSec float64) {
	a.mu.Lock()
	a.connected = true
	a.typing = typing
	a.idleSec = idleSec
	a.lastBeat = time.Now()
	a.mu.Unlock()
}

// subscribe registers a new SSE listener and returns the channel plus an unsubscribe func.
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

// broadcast delivers msg to every active SSE subscriber. Slow/full subscribers are skipped.
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

// pendingDialogState holds everything needed to track a non-blocking dialog.
type pendingDialogState struct {
	responseCh chan string
	activity   *dialogActivity // non-nil only for browser-based dialogs
}

// newDialogToken generates a random hex token for a pending dialog session.
func newDialogToken() string {
	b := make([]byte, 8)
	if _, err := cryptorand.Read(b); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%016x", b)
}
