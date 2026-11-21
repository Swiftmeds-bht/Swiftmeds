package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// AuthCallTimeout is the limit for an auth route and for each gRPC call to
// Auth (cmd/server), longer than the usual 2 s per request and 800 ms per call.
// Register and Login hash or check a bcrypt password, which is slow on purpose:
// they normally take 0.1-0.15 s, but on a cold path (the first call after a
// restart) a sign-up went past 800 ms and failed as "took too long" (SCRUM-136).
const AuthCallTimeout = 3 * time.Second

type authRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	RefreshToken string `json:"refreshToken"`
}

func authHandler(client ports.AuthRoutes, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body authRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), AuthCallTimeout)
		defer cancel()
		var response interface{ ProtoReflect() protoreflect.Message }
		var err error
		code := http.StatusOK
		switch r.URL.Path {
		case "/api/v1/auth/register":
			role := authv1.Role_ROLE_PATIENT
			if strings.EqualFold(body.Role, "ROLE_PHARMACY_STAFF") {
				role = authv1.Role_ROLE_PHARMACY_STAFF
			}
			if body.Email == "" || body.Password == "" || (body.Role != "" && role == authv1.Role_ROLE_PATIENT && body.Role != "ROLE_PATIENT") {
				writeError(w, http.StatusBadRequest, "invalid registration request")
				return
			}
			response, err = client.Register(ctx, &authv1.RegisterRequest{Email: body.Email, Password: body.Password, Role: role})
			code = http.StatusCreated
		case "/api/v1/auth/login":
			if body.Email == "" || body.Password == "" {
				writeError(w, http.StatusBadRequest, "email and password are required")
				return
			}
			response, err = client.Login(ctx, &authv1.LoginRequest{Email: body.Email, Password: body.Password})
		case "/api/v1/auth/refresh":
			if body.RefreshToken == "" {
				writeError(w, http.StatusBadRequest, "refreshToken is required")
				return
			}
			response, err = client.RefreshToken(ctx, &authv1.RefreshTokenRequest{RefreshToken: body.RefreshToken})
		case "/api/v1/auth/logout":
			if body.RefreshToken == "" {
				writeError(w, http.StatusBadRequest, "refreshToken is required")
				return
			}
			response, err = client.Logout(ctx, &authv1.LogoutRequest{RefreshToken: body.RefreshToken})
			if err == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		writeProtoStatus(w, code, response)
	}
}
