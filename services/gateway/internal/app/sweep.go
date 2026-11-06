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
	// SweepPath is called by the hold-expiry Cloudflare Worker (SCRUM-63).
	// It is the route runHoldExpirySweep in docs/api/openapi.yaml.
	SweepPath = "/api/v1/internal/expiry/sweep"

	sweepTimeout = 20 * time.Second
)

// sweepHandler lets the Worker trigger a hold expiry sweep. It follows the same
// rules as decayHandler: every call needs the shared secret in X-Worker-Secret,
// with no secret configured every call is rejected, and the secret is checked
// before the body is read. The body is {"asOf": "<RFC 3339>"}.
func sweepHandler(secret string, sweeper ports.HoldSweeper, logger *slog.Logger) http.HandlerFunc {
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
			logger.Error("hold expiry sweep failed",
				"request_id", httpx.RequestIDFromContext(r.Context()),
				"error", err.Error(),
			)
			// The caller gets a status and a fixed message, never the internal error text.
			status := httpx.StatusCode(err)
			writeError(w, status, errorMessage(status))
			return
		}
		if expired == nil {
			// An empty list is "[]", never null.
			expired = []string{}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string][]string{"expiredHoldIds": expired})
	}
}
