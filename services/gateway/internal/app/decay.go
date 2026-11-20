package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

const (
	// DecayPath is called by the freshness-decay Cloudflare Worker (SCRUM-74).
	// It is the route runFreshnessDecay in docs/api/openapi.yaml.
	DecayPath = "/api/v1/internal/freshness/decay"

	// WorkerSecretHeader carries the shared secret. It is not "Authorization",
	// which belongs to user tokens on the other routes.
	WorkerSecretHeader = "X-Worker-Secret"

	decayTimeout  = 10 * time.Second
	maxDecayBytes = 1 << 10
)

// decayRequest is the body from the API contract: {"asOf": "<RFC 3339>"}.
type decayRequest struct {
	AsOf *string `json:"asOf"`
}

// decayHandler lets the Worker trigger a freshness decay. Every call needs the
// shared secret in X-Worker-Secret. With no secret configured, every call is
// rejected, so the endpoint can never be open by mistake. The secret is checked
// before the body is read.
func decayHandler(secret string, applier ports.DecayApplier, logger *slog.Logger) http.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(r.Context(), decayTimeout)
		defer cancel()

		updated, err := applier.ApplyDecay(ctx, asOf)
		if err != nil {
			logRPCError(logger, "freshness decay failed", httpx.RequestIDFromContext(r.Context()), err)
			// The caller gets a status and a fixed message, never the internal error text.
			status := httpx.StatusCode(err)
			writeError(w, status, errorMessage(status))
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"updatedCount": updated})
	}
}

// parseAsOf reads the body. It returns the time in UTC, or a message that says
// what to fix.
func parseAsOf(w http.ResponseWriter, r *http.Request) (time.Time, string) {
	var req decayRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDecayBytes)).Decode(&req); err != nil {
		return time.Time{}, `request body must be JSON like {"asOf": "2026-11-05T07:00:00Z"}`
	}
	if req.AsOf == nil || strings.TrimSpace(*req.AsOf) == "" {
		return time.Time{}, "asOf is required"
	}
	asOf, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.AsOf))
	// The zero time is rejected because Freshness reads it as "now".
	if err != nil || asOf.IsZero() {
		return time.Time{}, "asOf must be an RFC 3339 timestamp, for example 2026-11-05T07:00:00Z"
	}
	return asOf.UTC(), ""
}

func writeError(w http.ResponseWriter, status int, message string) {
	httpx.WriteJSON(w, status, map[string]string{"error": message})
}

// errorMessage is the fixed text for a failed downstream call. A server fault
// always reads "internal error", as the API contract says.
func errorMessage(status int) string {
	switch status {
	case http.StatusInternalServerError:
		return "internal error"
	case http.StatusServiceUnavailable:
		return "service unavailable"
	case http.StatusGatewayTimeout:
		return "request timed out"
	default:
		return strings.ToLower(http.StatusText(status))
	}
}

// secretMatches compares in constant time. Both values are hashed first so the
// comparison time does not reveal the length of the real secret. An empty
// secret on either side never matches.
func secretMatches(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	wantSum := sha256.Sum256([]byte(want))
	gotSum := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(wantSum[:], gotSum[:]) == 1
}
