package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

// collector is a send function that records alerts and can be told to fail.
type collector struct {
	mu    sync.Mutex
	got   []domain.Alert
	fail  error
	recvd chan struct{}
}

func newCollector() *collector { return &collector{recvd: make(chan struct{}, 16)} }

func (c *collector) send(a domain.Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.got = append(c.got, a)
	c.recvd <- struct{}{}
	return nil
}

func (c *collector) ids() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.got))
	for _, a := range c.got {
		out = append(out, a.SubscriptionID)
	}
	return out
}

func (c *collector) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.recvd:
	case <-time.After(2 * time.Second):
		t.Fatal("no alert arrived")
	}
}

// stream runs StreamAlerts in the background and returns a stop function that
// cancels it and waits for it to return, failing the test if it hangs.
func stream(t *testing.T, f fixture, user string, c *collector) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.svc.StreamAlerts(ctx, user, c.send) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("StreamAlerts did not return after the client went away")
			return nil
		}
	}
}

func waitListeners(t *testing.T, f fixture, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for f.svc.hub.listeners() != want {
		if time.Now().After(deadline) {
			t.Fatalf("hub has %d listeners, want %d", f.svc.hub.listeners(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStreamSendsAlertsThatWereWaitingThenMarksThemDelivered(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	s := f.subscribe(t, "user-1", "pr-1", thimphu)
	if _, err := f.svc.HandleStockIncreased(ctx, increase("ph-thimphu", "pr-1", 4)); err != nil {
		t.Fatal(err)
	}

	c := newCollector()
	stop := stream(t, f, "user-1", c)
	c.wait(t)
	if err := stop(); err != nil {
		t.Fatalf("StreamAlerts: %v", err)
	}
	if got := c.ids(); len(got) != 1 || got[0] != s.ID {
		t.Fatalf("streamed %v, want [%s]", got, s.ID)
	}
	if pending, _ := f.repo.PendingAlerts(ctx, "user-1"); len(pending) != 0 {
		t.Errorf("alert still pending after it was sent: %+v", pending)
	}

	// A second connection does not get the same alert again.
	c2 := newCollector()
	stop2 := stream(t, f, "user-1", c2)
	waitListeners(t, f, 1)
	if err := stop2(); err != nil {
		t.Fatal(err)
	}
	if got := c2.ids(); len(got) != 0 {
		t.Errorf("second connection got %v again", got)
	}
}

func TestStreamPushesANewAlertAtOnce(t *testing.T) {
	f := newFixture()
	f.svc.WithAlertPoll(time.Hour) // only the push can deliver it in time
	s := f.subscribe(t, "user-1", "pr-1", thimphu)

	c := newCollector()
	stop := stream(t, f, "user-1", c)
	waitListeners(t, f, 1)

	if _, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4)); err != nil {
		t.Fatal(err)
	}
	c.wait(t)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if got := c.ids(); len(got) != 1 || got[0] != s.ID {
		t.Fatalf("streamed %v, want [%s]", got, s.ID)
	}
}

func TestStreamOnlyGetsTheUsersOwnAlerts(t *testing.T) {
	f := newFixture()
	f.subscribe(t, "someone-else", "pr-1", thimphu)
	c := newCollector()
	stop := stream(t, f, "user-1", c)
	waitListeners(t, f, 1)
	if _, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if got := c.ids(); len(got) != 0 {
		t.Fatalf("user-1 got someone else's alert: %v", got)
	}
}

// The poll catches alerts saved by another Watchlist replica, which cannot wake this one.
func TestStreamPollsForAlertsSavedElsewhere(t *testing.T) {
	f := newFixture()
	f.svc.WithAlertPoll(20 * time.Millisecond)
	s := f.subscribe(t, "user-1", "pr-1", thimphu)
	c := newCollector()
	stop := stream(t, f, "user-1", c)
	waitListeners(t, f, 1)

	// Saved straight into the repository, as another replica would: no wake-up.
	if _, err := f.repo.Notify(context.Background(), domain.Alert{ID: "al-x", SubscriptionID: s.ID, UserID: "user-1", ProductID: "pr-1", PharmacyID: "ph-1", QuantityAvailable: 2, OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	c.wait(t)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

// No goroutine or listener is left behind when the client disconnects.
func TestStreamCleansUpWhenTheClientLeaves(t *testing.T) {
	f := newFixture()
	stops := make([]func() error, 0, 3)
	for range 3 {
		stops = append(stops, stream(t, f, "user-1", newCollector()))
	}
	waitListeners(t, f, 3)
	for _, stop := range stops {
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	}
	waitListeners(t, f, 0)
}

func TestStreamEndsWhenSendingFails(t *testing.T) {
	f := newFixture()
	f.subscribe(t, "user-1", "pr-1", thimphu)
	if _, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4)); err != nil {
		t.Fatal(err)
	}
	c := newCollector()
	c.fail = errors.New("connection reset")

	err := f.svc.StreamAlerts(context.Background(), "user-1", c.send)
	if err == nil || !errors.Is(err, c.fail) {
		t.Fatalf("err = %v, want the send error", err)
	}
	if pending, _ := f.repo.PendingAlerts(context.Background(), "user-1"); len(pending) != 1 {
		t.Errorf("an alert that was not sent was marked delivered: %d pending", len(pending))
	}
	if n := f.svc.hub.listeners(); n != 0 {
		t.Errorf("%d listeners left after the stream ended", n)
	}
}

func TestStreamNeedsAUser(t *testing.T) {
	err := newFixture().svc.StreamAlerts(context.Background(), " ", newCollector().send)
	if !errors.Is(err, domain.ErrMissingUserID) {
		t.Fatalf("err = %v, want ErrMissingUserID", err)
	}
}

func TestHubWakesEveryListenerOfThatUserOnly(t *testing.T) {
	h := newHub()
	a, stopA := h.subscribe("user-1")
	b, stopB := h.subscribe("user-1")
	other, stopOther := h.subscribe("user-2")
	defer stopOther()

	h.notify("user-1")
	h.notify("user-1") // a second wake-up while one is pending is dropped, not blocked
	for name, ch := range map[string]<-chan struct{}{"a": a, "b": b} {
		select {
		case <-ch:
		default:
			t.Errorf("listener %s was not woken", name)
		}
	}
	select {
	case <-other:
		t.Error("user-2's listener was woken for user-1")
	default:
	}

	stopA()
	stopA() // stopping twice is harmless
	stopB()
	if n := h.listeners(); n != 1 {
		t.Errorf("listeners = %d, want 1", n)
	}
}
