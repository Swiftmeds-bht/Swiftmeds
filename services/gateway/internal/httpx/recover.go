package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic in a handler into a JSON 500 and logs it with a stack
// trace. Place it inside Logging so the request log records the 500.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// net/http uses this sentinel to abort a response on purpose.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "panic in http handler",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
				)
				// Never send panic details to the client.
				WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
