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
	freshness  ports.DecayApplier
	expiry     ports.HoldExpirySweeper
	auth       ports.Authenticator
	search     ports.Searcher
	geo        ports.NearbyPharmacyFinder
	substitute ports.SubstituteFinder
}

// WithFreshness adds the internal freshness-decay endpoint (SCRUM-74). Without
// it the route does not exist and the path answers 404.
func WithFreshness(applier ports.DecayApplier) RouterOption {
	return func(r *routes) { r.freshness = applier }
}

// WithExpiry adds the internal hold-expiry sweep endpoint (SCRUM-62). Without
// it the route does not exist and the path answers 404.
func WithExpiry(sweeper ports.HoldExpirySweeper) RouterOption {
	return func(r *routes) { r.expiry = sweeper }
}

// WithAuth adds the Auth service client used by protected API routes.
func WithAuth(authenticator ports.Authenticator) RouterOption {
	return func(r *routes) { r.auth = authenticator }
}

func WithSearch(searcher ports.Searcher) RouterOption {
	return func(r *routes) { r.search = searcher }
}

func WithGeo(finder ports.NearbyPharmacyFinder) RouterOption {
	return func(r *routes) { r.geo = finder }
}

func WithSubstitution(finder ports.SubstituteFinder) RouterOption {
	return func(r *routes) { r.substitute = finder }
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
	if wired.expiry != nil {
		mux.Handle("POST "+SweepPath, sweepHandler(cfg.WorkerSecret, wired.expiry, logger))
	}
	if wired.search != nil {
		mux.Handle("GET /api/v1/search", searchHandler(wired.search, logger))
	}
	if wired.geo != nil {
		mux.Handle("GET /api/v1/pharmacies/nearby", nearbyHandler(wired.geo, logger))
	}
	if wired.substitute != nil {
		mux.Handle("GET /api/v1/medicines/{id}/alternatives", alternativesHandler(wired.substitute, logger))
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
		func(next http.Handler) http.Handler { return httpx.Auth(wired.auth, next) },
	)
}
