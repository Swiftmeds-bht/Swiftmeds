package app

import (
	"log/slog"
	"net/http"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
)

func NewRouter(cfg Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
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
