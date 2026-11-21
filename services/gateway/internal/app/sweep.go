package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

const (
	// SweepPath is called by the hold-expiry Cloudflare Worker (SCRUM-62).
	// It is the route runHoldExpirySweep in docs/api/openapi.yaml.
	SweepPath = "/api/v1/internal/expiry/sweep"

	// sweepTimeout bounds one sweep. A sweep that does not finish keeps the work
	// it did; the next scheduled call takes up the rest.
	sweepTimeout = 10 * time.Second

	// SweepCallTimeout is the limit for the gRPC call to Expiry, longer than the
	// gateway's usual 800 ms because a sweep calls Holds once per due hold. It
	// stays below sweepTimeout.
	SweepCallTimeout = 8 * time.Second
)

// sweepHandler lets the Worker trigger a hold-expiry sweep. It is guarded like
// the freshness decay route: every call needs the shared secret in
// X-Worker-Secret, with no secret configured every call is rejected, and the
// secret is checked before the body is read. The body is the same as for decay:
// {"asOf": "<RFC 3339>"}.
func sweepHandler(secret string, sweeper ports.HoldExpirySweeper, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !secretMatches(secret, r.Header.Get(WorkerSecretHeader)) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		asOf, problem := parseAsOf(w, r)
		if problem != "" {
			writeError(w, http.StatusBadRequest, problem)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), sweepTimeout)
		defer cancel()

		expired, err := sweeper.RunSweep(ctx, asOf)
		if err != nil {
			logRPCError(logger, "hold expiry sweep failed", httpx.RequestIDFromContext(r.Context()), err)
			// The caller gets a status and a fixed message, never the internal error text.
			status := httpx.StatusCode(err)
			writeError(w, status, errorMessage(status))
			return
		}
		if expired == nil {
			expired = []string{}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string][]string{"expiredHoldIds": expired})
	}
}
