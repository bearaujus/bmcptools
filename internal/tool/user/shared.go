package user

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// pendingDialogs stores state for pending ask_user sessions.
var pendingDialogs sync.Map

var dialogTokenReader io.Reader = cryptorand.Reader

const (
	dialogResultRetention    = 5 * time.Minute
	dialogUpdateDeliveryWait = 150 * time.Millisecond
)

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
	val, ok := pendingDialogs.LoadAndDelete(token)
	if !ok {
		return
	}
	if state, ok := val.(*pendingDialogState); ok {
		state.dispose()
	}
}

func releasePendingDialog(token string, attachmentGrace time.Duration) {
	val, ok := pendingDialogs.LoadAndDelete(token)
	if !ok {
		return
	}
	if state, ok := val.(*pendingDialogState); ok {
		state.stopCleanup()
		state.scheduleAttachmentCleanup(attachmentGrace)
	}
}

// dialogActivity tracks real-time user activity reported by the browser dialog's heartbeat.
type dialogActivity struct {
	mu           sync.Mutex
	connected    bool
	typing       bool
	idleSec      float64
	lastBeat     time.Time
	outboundSubs []chan dialogOutboundEvent
}

type dialogEvent struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Replace bool   `json:"replace,omitempty"`
}

type dialogOutboundEvent struct {
	dialogEvent
	tracker *dialogDeliveryTracker
}

func (e dialogOutboundEvent) markDelivered() {
	if e.tracker != nil {
		e.tracker.markDelivered()
	}
}

func (e dialogOutboundEvent) markDropped() {
	if e.tracker != nil {
		e.tracker.markDropped()
	}
}

type dialogDeliveryState string

const (
	dialogDeliveryNoSubscribers dialogDeliveryState = "no_subscribers"
	dialogDeliveryQueued        dialogDeliveryState = "queued"
	dialogDeliveryDelivered     dialogDeliveryState = "delivered"
	dialogDeliveryDropped       dialogDeliveryState = "dropped"
)

type dialogDeliveryOutcome struct {
	State       dialogDeliveryState
	Subscribers int
	Queued      int
	Delivered   int
	Dropped     int
}

type dialogDeliveryTracker struct {
	mu          sync.Mutex
	subscribers int
	queued      int
	delivered   int
	dropped     int
	deliveredCh chan struct{}
	deliverOnce sync.Once
}

const (
	dialogEventUpdate  = "update"
	dialogEventDismiss = "dismiss"
)

func (a *dialogActivity) update(typing bool, idleSec float64) {
	a.mu.Lock()
	a.connected = true
	a.typing = typing
	a.idleSec = idleSec
	a.lastBeat = time.Now()
	a.mu.Unlock()
}

func (a *dialogActivity) hasSubscribers() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.outboundSubs) > 0
}

func (a *dialogActivity) subscribe() (chan dialogOutboundEvent, func()) {
	ch := make(chan dialogOutboundEvent, 8)
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

func newDialogDeliveryTracker(subscribers int) *dialogDeliveryTracker {
	return &dialogDeliveryTracker{
		subscribers: subscribers,
		deliveredCh: make(chan struct{}),
	}
}

func (t *dialogDeliveryTracker) noteQueued() {
	t.mu.Lock()
	t.queued++
	t.mu.Unlock()
}

func (t *dialogDeliveryTracker) markDelivered() {
	t.mu.Lock()
	t.delivered++
	t.mu.Unlock()
	t.deliverOnce.Do(func() { close(t.deliveredCh) })
}

func (t *dialogDeliveryTracker) markDropped() {
	t.mu.Lock()
	t.dropped++
	t.mu.Unlock()
}

func (t *dialogDeliveryTracker) snapshot() dialogDeliveryOutcome {
	t.mu.Lock()
	defer t.mu.Unlock()
	outcome := dialogDeliveryOutcome{
		Subscribers: t.subscribers,
		Queued:      t.queued,
		Delivered:   t.delivered,
		Dropped:     t.dropped,
	}
	switch {
	case outcome.Subscribers == 0:
		outcome.State = dialogDeliveryNoSubscribers
	case outcome.Delivered > 0:
		outcome.State = dialogDeliveryDelivered
	case outcome.Queued > 0:
		outcome.State = dialogDeliveryQueued
	default:
		outcome.State = dialogDeliveryDropped
	}
	return outcome
}

func (t *dialogDeliveryTracker) await(wait time.Duration) dialogDeliveryOutcome {
	if wait > 0 {
		select {
		case <-t.deliveredCh:
		case <-time.After(wait):
		}
	}
	return t.snapshot()
}

func (a *dialogActivity) broadcast(evt dialogEvent) {
	a.mu.Lock()
	for _, ch := range a.outboundSubs {
		select {
		case ch <- dialogOutboundEvent{dialogEvent: evt}:
		default:
		}
	}
	a.mu.Unlock()
}

func (a *dialogActivity) broadcastUpdate(msg string, replace bool, wait time.Duration) dialogDeliveryOutcome {
	a.mu.Lock()
	subscribers := len(a.outboundSubs)
	tracker := newDialogDeliveryTracker(subscribers)
	for _, ch := range a.outboundSubs {
		outbound := dialogOutboundEvent{
			dialogEvent: dialogEvent{Type: dialogEventUpdate, Message: msg, Replace: replace},
			tracker:     tracker,
		}
		select {
		case ch <- outbound:
			tracker.noteQueued()
		default:
			tracker.markDropped()
		}
	}
	a.mu.Unlock()
	return tracker.await(wait)
}

func (a *dialogActivity) broadcastDismiss() {
	a.broadcast(dialogEvent{Type: dialogEventDismiss})
}

type pendingDialogState struct {
	responseCh chan string
	activity   *dialogActivity
	cancelFn   context.CancelFunc // non-nil for ask_user dialogs; call to cancel the open dialog
	cleanupMu  sync.Mutex
	cleanup    *time.Timer
	attachMu   sync.Mutex
	attachDirs []string
	attachGC   *time.Timer
}

func (s *pendingDialogState) scheduleCleanup(token string, delay time.Duration) {
	if s == nil {
		return
	}
	if delay <= 0 {
		deletePendingDialog(token)
		return
	}
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanup != nil {
		s.cleanup.Stop()
	}
	s.cleanup = time.AfterFunc(delay, func() {
		deletePendingDialog(token)
	})
}

func (s *pendingDialogState) stopCleanup() {
	if s == nil {
		return
	}
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanup != nil {
		s.cleanup.Stop()
		s.cleanup = nil
	}
}

func (s *pendingDialogState) registerAttachmentDir(dir string) {
	if s == nil || dir == "" {
		return
	}
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	for _, existing := range s.attachDirs {
		if existing == dir {
			return
		}
	}
	s.attachDirs = append(s.attachDirs, dir)
}

func (s *pendingDialogState) scheduleAttachmentCleanup(delay time.Duration) {
	if s == nil {
		return
	}
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	if len(s.attachDirs) == 0 {
		return
	}
	if s.attachGC != nil {
		s.attachGC.Stop()
	}
	if delay <= 0 {
		dirs := append([]string(nil), s.attachDirs...)
		s.attachDirs = nil
		s.attachGC = nil
		go removeAttachmentDirs(dirs)
		return
	}
	s.attachGC = time.AfterFunc(delay, func() {
		s.cleanupAttachmentDirs()
	})
}

func (s *pendingDialogState) cleanupAttachmentDirs() {
	if s == nil {
		return
	}
	s.attachMu.Lock()
	dirs := append([]string(nil), s.attachDirs...)
	s.attachDirs = nil
	if s.attachGC != nil {
		s.attachGC.Stop()
		s.attachGC = nil
	}
	s.attachMu.Unlock()
	removeAttachmentDirs(dirs)
}

func removeAttachmentDirs(dirs []string) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		_ = os.RemoveAll(dir)
	}
}

func (s *pendingDialogState) dispose() {
	if s == nil {
		return
	}
	s.stopCleanup()
	s.cleanupAttachmentDirs()
}

func newDialogToken() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(dialogTokenReader, b); err != nil {
		return "", fmt.Errorf("failed to generate dialog token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
