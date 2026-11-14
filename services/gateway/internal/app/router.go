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
	freshness        ports.DecayApplier
	expiry           ports.HoldExpirySweeper
	auth             ports.Authenticator
	authRoutes       ports.AuthRoutes
	search           ports.Searcher
	geo              ports.NearbyPharmacyFinder
	substitute       ports.SubstituteFinder
	holds            ports.HoldsClient
	watchlist        ports.WatchlistClient
	feedback         ports.FeedbackClient
	notifications    ports.NotificationClient
	pharmacyRegistry ports.PharmacyRegistryClient
	analytics        ports.AnalyticsClient
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
	return func(r *routes) { r.auth = authenticator; r.authRoutes, _ = authenticator.(ports.AuthRoutes) }
}

func WithAuthRoutes(client ports.AuthRoutes) RouterOption {
	return func(r *routes) { r.authRoutes = client }
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

func WithHolds(client ports.HoldsClient) RouterOption {
	return func(r *routes) { r.holds = client }
}

func WithPharmacyRegistry(client ports.PharmacyRegistryClient) RouterOption {
	return func(r *routes) { r.pharmacyRegistry = client }
}

func WithAnalytics(client ports.AnalyticsClient) RouterOption {
	return func(r *routes) { r.analytics = client }
}

func WithWatchlist(client ports.WatchlistClient) RouterOption {
	return func(r *routes) { r.watchlist = client }
}

func WithFeedback(client ports.FeedbackClient) RouterOption {
	return func(r *routes) { r.feedback = client }
}

func WithNotifications(client ports.NotificationClient) RouterOption {
	return func(r *routes) { r.notifications = client }
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
	if wired.authRoutes != nil {
		mux.Handle("POST /api/v1/auth/register", authHandler(wired.authRoutes, logger))
		mux.Handle("POST /api/v1/auth/login", authHandler(wired.authRoutes, logger))
		mux.Handle("POST /api/v1/auth/refresh", authHandler(wired.authRoutes, logger))
		mux.Handle("POST /api/v1/auth/logout", authHandler(wired.authRoutes, logger))
	}
	if wired.geo != nil {
		mux.Handle("GET /api/v1/pharmacies/nearby", nearbyHandler(wired.geo, logger))
	}
	if wired.substitute != nil {
		mux.Handle("GET /api/v1/medicines/{id}/alternatives", alternativesHandler(wired.substitute, logger))
	}
	if wired.holds != nil {
		mux.Handle("POST /api/v1/holds", holdsHandler(wired.holds, logger))
		mux.Handle("GET /api/v1/holds", holdsHandler(wired.holds, logger))
		mux.Handle("GET /api/v1/holds/{holdId}", holdsHandler(wired.holds, logger))
		mux.Handle("POST /api/v1/holds/{holdId}/cancel", holdsHandler(wired.holds, logger))
		mux.Handle("POST /api/v1/holds/{holdId}/collect", holdsHandler(wired.holds, logger))
	}
	if wired.watchlist != nil {
		mux.Handle("POST /api/v1/watchlist", watchlistHandler(wired.watchlist, logger))
		mux.Handle("GET /api/v1/watchlist", watchlistHandler(wired.watchlist, logger))
		mux.Handle("DELETE /api/v1/watchlist/{subscriptionId}", watchlistHandler(wired.watchlist, logger))
		mux.Handle("GET /api/v1/watchlist/alerts", alertsHandler(wired.watchlist, logger, AlertHeartbeat))
	}
	if wired.feedback != nil {
		mux.Handle("POST /api/v1/holds/{holdId}/feedback", feedbackHandler(wired.feedback, logger))
	}
	if wired.notifications != nil {
		mux.Handle("GET /api/v1/notifications", notificationsHandler(wired.notifications, logger))
	}
	mux.Handle("/api/v1/", http.NotFoundHandler())

	if wired.pharmacyRegistry != nil && wired.analytics != nil {
		mux.Handle(
			"GET /api/v1/pharmacies/me/analytics/unmet-demand",
			unmetDemandHandler(wired.analytics, wired.pharmacyRegistry, logger))
	}

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
