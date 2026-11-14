package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/protobuf/reflect/protoreflect"
)

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
		ctx, cancel := callContext(r)
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
