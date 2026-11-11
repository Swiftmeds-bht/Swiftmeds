package app

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

// fakeAlertStream plays the client side of StreamAlerts: it returns the queued
// alerts, then end (if set), else blocks until the context is cancelled, like a
// real gRPC stream. recvDone closes when Recv has returned for the last time.
type fakeAlertStream struct {
	grpc.ClientStream
	ctx      context.Context
	queue    chan *watchlistv1.StockAlert
	end      error
	recvDone chan struct{}
}

func newFakeAlertStream(alerts ...*watchlistv1.StockAlert) *fakeAlertStream {
	q := make(chan *watchlistv1.StockAlert, len(alerts)+4)
	for _, a := range alerts {
		q <- a
	}
	return &fakeAlertStream{queue: q, recvDone: make(chan struct{})}
}

func (f *fakeAlertStream) Recv() (*watchlistv1.StockAlert, error) {
	select {
	case a := <-f.queue:
		return a, nil
	default:
	}
	if f.end != nil {
		close(f.recvDone)
		return nil, f.end
	}
	select {
	case a := <-f.queue:
		return a, nil
	case <-f.ctx.Done():
		close(f.recvDone)
		return nil, status.FromContextError(f.ctx.Err()).Err()
	}
}

func (f *fakeAlertStream) Context() context.Context { return f.ctx }

var patient = ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT}

func alert(sub string, qty int32) *watchlistv1.StockAlert {
	return &watchlistv1.StockAlert{
		SubscriptionId: sub, ProductId: "pr-1", PharmacyId: "ph-1", QuantityAvailable: qty,
		OccurredAt: timestamppb.New(time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)),
	}
}

// serveAlerts runs the handler on a real HTTP server, so flushing and client
// disconnects behave as in production.
func serveAlerts(t *testing.T, fake *fakeWatchlistClient, identity ports.Identity, heartbeat time.Duration) (*http.Response, func()) {
	t.Helper()
	handler := alertsHandler(fake, slog.New(slog.NewTextHandler(io.Discard, nil)), heartbeat)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, withIdentity(r, identity))
	}))
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/watchlist/alerts", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		cancel()
		srv.Close()
		t.Fatal(err)
	}
	return resp, func() {
		cancel()
		_ = resp.Body.Close()
		srv.Close()
	}
}

// readEvents reads SSE blocks (separated by a blank line) until n are read.
func readEvents(t *testing.T, r *bufio.Reader, n int) []string {
	t.Helper()
	var events []string
	var block strings.Builder
	deadline := time.Now().Add(3 * time.Second)
	for len(events) < n {
		if time.Now().After(deadline) {
			t.Fatalf("read %d of %d events: %q", len(events), n, events)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v (got %q)", err, events)
		}
		if line == "\n" {
			events = append(events, block.String())
			block.Reset()
			continue
		}
		block.WriteString(line)
	}
	return events
}

func TestAlertStreamSendsEachAlertAsAnEvent(t *testing.T) {
	fake := &fakeWatchlistClient{stream: newFakeAlertStream(alert("sub-1", 4), alert("sub-2", 9))}
	resp, stop := serveAlerts(t, fake, patient, time.Hour)
	defer stop()

	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" ||
		resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("status %d, headers %v", resp.StatusCode, resp.Header)
	}
	if fake.streamReq.GetUserId() != "user-1" {
		t.Errorf("stream opened for %q, want the token's user", fake.streamReq.GetUserId())
	}
	events := readEvents(t, bufio.NewReader(resp.Body), 3)
	if events[0] != ": connected\n" {
		t.Errorf("first block = %q, want the connected comment", events[0])
	}
	want := "id: sub-1\nevent: alert\ndata: {"
	if !strings.HasPrefix(events[1], want) || !strings.Contains(events[1], `"quantityAvailable":4`) ||
		!strings.Contains(events[1], `"occurredAt":"2026-11-11T09:00:00Z"`) {
		t.Errorf("event = %q", events[1])
	}
	if !strings.HasPrefix(events[2], "id: sub-2\nevent: alert\n") {
		t.Errorf("second event = %q", events[2])
	}
}

func TestAlertStreamSendsHeartbeats(t *testing.T) {
	fake := &fakeWatchlistClient{stream: newFakeAlertStream()}
	resp, stop := serveAlerts(t, fake, patient, 20*time.Millisecond)
	defer stop()
	events := readEvents(t, bufio.NewReader(resp.Body), 3)
	if events[1] != ": ping\n" || events[2] != ": ping\n" {
		t.Fatalf("events = %q, want pings", events)
	}
}

// No goroutine is left behind: when the browser leaves, the gRPC stream's
// context is cancelled and the receiving goroutine returns.
func TestAlertStreamStopsWhenTheBrowserLeaves(t *testing.T) {
	fake := &fakeWatchlistClient{stream: newFakeAlertStream()}
	resp, stop := serveAlerts(t, fake, patient, time.Hour)
	readEvents(t, bufio.NewReader(resp.Body), 1)

	stop()
	select {
	case <-fake.stream.recvDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the gRPC stream was not cancelled after the browser left")
	}
	if fake.stream.ctx.Err() == nil {
		t.Error("stream context still live after the browser left")
	}
}

func TestAlertStreamEndsWithAnErrorEventWhenWatchlistFails(t *testing.T) {
	s := newFakeAlertStream(alert("sub-1", 1))
	s.end = status.Error(codes.Unavailable, "watchlist down")
	resp, stop := serveAlerts(t, &fakeWatchlistClient{stream: s}, patient, time.Hour)
	defer stop()
	events := readEvents(t, bufio.NewReader(resp.Body), 3)
	if events[2] != "event: error\ndata: {\"error\":\"service unavailable\"}\n" {
		t.Errorf("last event = %q", events[2])
	}
}

func TestAlertStreamEndsQuietlyWhenWatchlistCloses(t *testing.T) {
	s := newFakeAlertStream()
	s.end = io.EOF
	resp, stop := serveAlerts(t, &fakeWatchlistClient{stream: s}, patient, time.Hour)
	defer stop()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != ": connected\n\n" {
		t.Errorf("body = %q, want only the connected comment", body)
	}
}

func TestAlertStreamRefusesNonPatients(t *testing.T) {
	staff := ports.Identity{UserID: "u-2", Role: authv1.Role_ROLE_PHARMACY_STAFF}
	rec := httptest.NewRecorder()
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/watchlist/alerts", nil), staff)
	alertsHandler(&fakeWatchlistClient{}, nil, time.Hour)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAlertStreamReportsAFailedOpen(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/watchlist/alerts", nil), patient)
	fake := &fakeWatchlistClient{openErr: status.Error(codes.Unavailable, "down")}
	alertsHandler(fake, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Hour)(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// The logging middleware's wrapper must not stop the handler from flushing.
func TestStreamingFlushesThroughTheLoggingMiddleware(t *testing.T) {
	fake := &fakeWatchlistClient{stream: newFakeAlertStream(alert("sub-1", 2))}
	inner := alertsHandler(fake, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Hour)
	logged := httpx.Logging(slog.New(slog.NewTextHandler(io.Discard, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner(w, withIdentity(r, patient))
	}))
	srv := httptest.NewServer(logged)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	events := readEvents(t, bufio.NewReader(resp.Body), 2)
	if !strings.HasPrefix(events[1], "id: sub-1\n") {
		t.Fatalf("events = %q", events)
	}
}
