package httpx

import (
	"context"
	"net/http"
	"strings"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type contextKey uint8

const (
	userIDKey contextKey = iota
	userRoleKey
	pharmacyIDKey
)

func WithIdentity(ctx context.Context, identity ports.Identity) context.Context {
	ctx = context.WithValue(ctx, userIDKey, identity.UserID)
	ctx = context.WithValue(ctx, userRoleKey, identity.Role)
	return context.WithValue(ctx, pharmacyIDKey, identity.PharmacyID)
}

func UserIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(userIDKey).(string)
	return value
}

func RoleFromContext(ctx context.Context) authv1.Role {
	value, _ := ctx.Value(userRoleKey).(authv1.Role)
	return value
}

func PharmacyIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(pharmacyIDKey).(string)
	return value
}

// Auth protects API routes while leaving explicitly public routes and the
// worker-authenticated internal endpoints available without a bearer token.
func Auth(authenticator ports.Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicRoute(r.URL.Path) || isPublicLookup(r) || r.URL.Path == "/healthz" || isWorkerRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		accessToken, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if authenticator == nil {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth service unavailable"})
			return
		}

		identity, err := authenticator.ValidateToken(r.Context(), accessToken)
		if err != nil {
			code := status.Code(err)
			if code == codes.Unauthenticated {
				WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			} else {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth service unavailable"})
			}
			return
		}
		if identity.UserID == "" || identity.Role == authv1.Role_ROLE_UNSPECIFIED {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth service unavailable"})
			return
		}
		if requiredRole := roleForRoute(r.URL.Path); !roleAllowed(identity.Role, requiredRole) {
			WriteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), identity)))
	})
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	returned := len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != ""
	if !returned {
		return "", false
	}
	return parts[1], true
}

// isWorkerRoute reports whether the path is called by a Cloudflare Worker. These
// routes check the shared secret in X-Worker-Secret instead of a user token.
// Only these exact paths are exempt.
func isWorkerRoute(path string) bool {
	return path == "/api/v1/internal/freshness/decay" || path == "/api/v1/internal/expiry/sweep"
}

func isPublicRoute(path string) bool {
	return routeMatches(path, "/api/v1/search") ||
		routeMatches(path, "/api/v1/pharmacies/nearby") ||
		routeMatches(path, "/api/v1/nearby") ||
		routeMatches(path, "/api/v1/medicines") ||
		routeMatches(path, "/api/v1/login") ||
		path == "/api/v1/auth/register" || path == "/api/v1/auth/login" || path == "/api/v1/auth/refresh"
}

// isPublicLookup reports whether the request reads one medicine or one
// pharmacy by ID (getProduct, getPharmacy in docs/api/openapi.yaml, SCRUM-138).
// Only GET of exactly one path segment is public: /api/v1/pharmacies/{id}/stock
// and every other pharmacy route still need a staff token.
func isPublicLookup(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	for _, prefix := range []string{"/api/v1/products/", "/api/v1/pharmacies/"} {
		if id, ok := strings.CutPrefix(r.URL.Path, prefix); ok && id != "" && !strings.Contains(id, "/") && id != "me" {
			return true
		}
	}
	return false
}

func roleForRoute(path string) authv1.Role {
	switch {
	case routeMatches(path, "/api/v1/pharmacy"), routeMatches(path, "/api/v1/pharmacies"):
		return authv1.Role_ROLE_PHARMACY_STAFF
	case routeMatches(path, "/api/v1/admin"):
		return authv1.Role_ROLE_ADMIN
	default:
		return authv1.Role_ROLE_UNSPECIFIED
	}
}

func roleAllowed(actual, required authv1.Role) bool {
	if required == authv1.Role_ROLE_UNSPECIFIED {
		return true
	}
	return actual == required || (required == authv1.Role_ROLE_PHARMACY_STAFF && actual == authv1.Role_ROLE_ADMIN)
}

func routeMatches(path, route string) bool {
	return path == route || strings.HasPrefix(path, route+"/")
}
