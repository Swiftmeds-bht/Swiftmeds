package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

// AlertHeartbeat is how often an idle alert stream sends a comment line, so
// proxies and browsers do not close a connection that has nothing to say.
const AlertHeartbeat = 15 * time.Second

// alertsHandler bridges Watchlist.StreamAlerts to the browser as server-sent
// events (GET /api/v1/watchlist/alerts, SCRUM-71). Each alert is one event:
//
//	id: <subscriptionId>
//	event: alert
//	data: <StockAlert as JSON>
//
// The gRPC stream lives on the request's context, so when the browser goes
// away the stream is cancelled and both goroutines end. The gateway server has
// no write timeout (cmd/server), so the response may stay open for hours.
func alertsHandler(client ports.WatchlistClient, logger *slog.Logger, heartbeat time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !patientOnly(w, identity) {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		stream, err := client.StreamAlerts(ctx, &watchlistv1.StreamAlertsRequest{UserId: identity.UserID})
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}

		rc := http.NewResponseController(w)
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no") // tell nginx-style proxies not to buffer the stream
		w.WriteHeader(http.StatusOK)
		if !writeEvent(w, rc, ": connected\n\n") {
			return
		}

		// Recv blocks, so it runs on its own goroutine. It stops when the stream
		// ends, which cancelling ctx guarantees.
		alerts := make(chan *watchlistv1.StockAlert)
		ended := make(chan error, 1)
		go func() {
			for {
				a, err := stream.Recv()
				if err != nil {
					ended <- err
					return
				}
				select {
				case alerts <- a:
				case <-ctx.Done():
					ended <- ctx.Err()
					return
				}
			}
		}()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				waitForAlertReceiver(ended, logger, httpx.RequestIDFromContext(r.Context()))
				return
			case a := <-alerts:
				data, err := protojson.Marshal(a)
				if err != nil {
					logger.Error("encode stock alert", "request_id", httpx.RequestIDFromContext(r.Context()), "error", err.Error())
					continue
				}
				if !writeEvent(w, rc, fmt.Sprintf("id: %s\nevent: alert\ndata: %s\n\n", a.GetSubscriptionId(), data)) {
					cancel()
					waitForAlertReceiver(ended, logger, httpx.RequestIDFromContext(r.Context()))
					return
				}
			case err := <-ended:
				if !errors.Is(err, io.EOF) && status.Code(err) != codes.Canceled && ctx.Err() == nil {
					code := httpx.StatusCode(err)
					logRPCError(logger, "alert stream failed", httpx.RequestIDFromContext(r.Context()), err)
					writeEvent(w, rc, fmt.Sprintf("event: error\ndata: {\"error\":%q}\n\n", errorMessage(code)))
				}
				return
			case <-ticker.C:
				if !writeEvent(w, rc, ": ping\n\n") {
					cancel()
					waitForAlertReceiver(ended, logger, httpx.RequestIDFromContext(r.Context()))
					return
				}
			}
		}
	}
}

func waitForAlertReceiver(ended <-chan error, logger *slog.Logger, requestID string) {
	err := <-ended
	if err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		status.Code(err) == codes.Canceled {
		return
	}

	logger.Error("alert stream failed during shutdown",
		"request_id", requestID,
		"error", err.Error(),
	)
}

// writeEvent writes and flushes one chunk; false means the client is gone.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, chunk string) bool {
	if _, err := io.WriteString(w, chunk); err != nil {
		return false
	}
	return rc.Flush() == nil
}
