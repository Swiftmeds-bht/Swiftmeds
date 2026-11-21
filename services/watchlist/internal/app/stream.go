package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

// DefaultAlertPoll is how often an open stream also checks the database. It
// catches alerts saved by another Watchlist replica, which cannot wake this one.
const DefaultAlertPoll = 5 * time.Second

// WithAlertPoll changes how often open streams check the database. It returns s.
func (s *Service) WithAlertPoll(every time.Duration) *Service {
	if every > 0 {
		s.poll = every
	}
	return s
}

// StreamAlerts sends the user's alerts as they happen: first the ones waiting,
// then each new one. An alert is marked delivered only after send succeeds, so
// one that could not be sent is sent again on the next connection. It returns
// nil when ctx ends (the client went away) and send's error if sending fails.
func (s *Service) StreamAlerts(ctx context.Context, userID string, send func(domain.Alert) error) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ErrMissingUserID
	}
	wake, stop := s.hub.subscribe(userID)
	defer stop()
	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()

	for {
		if err := s.deliverPending(ctx, userID, send); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) deliverPending(ctx context.Context, userID string, send func(domain.Alert) error) error {
	alerts, err := s.repo.PendingAlerts(ctx, userID)
	if err != nil {
		return fmt.Errorf("read pending alerts: %w", err)
	}
	for _, a := range alerts {
		if err := send(a); err != nil {
			return fmt.Errorf("send alert %s: %w", a.ID, err)
		}
		if err := s.repo.MarkDelivered(ctx, a.ID, s.now()); err != nil {
			return fmt.Errorf("mark alert %s delivered: %w", a.ID, err)
		}
	}
	return nil
}

// hub wakes the open streams of a user when a new alert is saved for them.
// Each listener has a one-slot channel: a wake-up while one is already pending
// is dropped, which is fine because the stream reads all pending alerts anyway.
type hub struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func newHub() *hub { return &hub{subs: map[string]map[chan struct{}]struct{}{}} }

func (h *hub) subscribe(userID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan struct{}]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subs[userID], ch)
			if len(h.subs[userID]) == 0 {
				delete(h.subs, userID)
			}
		})
	}
}

func (h *hub) notify(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// listeners counts open streams (for tests and leak checks).
func (h *hub) listeners() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, set := range h.subs {
		n += len(set)
	}
	return n
}
