package app

import (
	"log/slog"
	"net/http"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

// RouterOption adds a route that needs an outside dependency.
type RouterOption func(*routes)

type routes struct {
	freshness ports.DecayApplier
}

// WithFreshness adds the internal freshness-decay endpoint (SCRUM-74). Without
// it the route does not exist and the path answers 404.
func WithFreshness(applier ports.DecayApplier) RouterOption {
	return func(r *routes) { r.freshness = applier }
}

func NewRouter(cfg Config, logger *slog.Logger, opts ...RouterOption) http.Handler {
	var wired routes
	for _, opt := range opts {
		opt(&wired)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if wired.freshness != nil {
		mux.Handle("POST "+DecayPath, decayHandler(cfg.WorkerSecret, wired.freshness, logger))
	}
	mux.Handle("/api/v1/", http.NotFoundHandler())

	// The first middleware is the outermost. Recover sits inside Logging so a
	// panic is logged as a 500 request, and outside CORS and the handlers.
	return httpx.Chain(
		mux,
		httpx.RequestID,
		httpx.Logging(logger),
		httpx.Recover(logger),
		httpx.CORS(cfg.CORSOrigins),
	)
}
