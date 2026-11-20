package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

// memRepo is a minimal in-memory ports.UserRepository for handler tests.
type memRepo struct {
	users   map[string]domain.User
	byEmail map[string]string
	refresh map[string]string
}

func newMemRepo() *memRepo {
	return &memRepo{users: map[string]domain.User{}, byEmail: map[string]string{}, refresh: map[string]string{}}
}

func (r *memRepo) CreateUser(_ context.Context, u domain.User) error {
	if _, ok := r.byEmail[u.Email]; ok {
		return domain.ErrUserExists
	}
	r.users[u.ID], r.byEmail[u.Email] = u, u.ID
	return nil
}

func (r *memRepo) FindUserByEmail(_ context.Context, email string) (domain.User, error) {
	id, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.users[id], nil
}

func (r *memRepo) LinkPharmacy(_ context.Context, userID, pharmacyID string) (domain.User, error) {
	u, ok := r.users[userID]
	switch {
	case !ok:
		return domain.User{}, domain.ErrUserNotFound
	case u.Role == domain.RoleAdmin:
		return domain.User{}, domain.ErrAdminLink
	case u.PharmacyID != "" && u.PharmacyID != pharmacyID:
		return domain.User{}, domain.ErrAlreadyLinked
	}
	u.Role, u.PharmacyID = domain.RolePharmacist, pharmacyID
	r.users[userID] = u
	return u, nil
}

func (r *memRepo) FindUserByID(_ context.Context, id string) (domain.User, error) {
	u, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *memRepo) SaveRefreshToken(_ context.Context, hash, userID string, _ time.Time) error {
	r.refresh[hash] = userID
	return nil
}

func (r *memRepo) ConsumeRefreshToken(_ context.Context, hash string) (string, error) {
	id, ok := r.refresh[hash]
	if !ok {
		return "", domain.ErrRefreshRevoked
	}
	delete(r.refresh, hash)
	return id, nil
}

func (r *memRepo) RevokeRefreshToken(_ context.Context, hash string) error {
	delete(r.refresh, hash)
	return nil
}

func newHandler() *Handler {
	svc := app.NewService(newMemRepo(), []byte("handler-test-secret"), "test", time.Minute, time.Hour, time.Now)
	return NewHandler(svc)
}

func TestToStatusMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrInvalidEmail, codes.InvalidArgument},
		{domain.ErrInvalidPassword, codes.InvalidArgument},
		{domain.ErrInvalidRole, codes.InvalidArgument},
		{domain.ErrAdminRegistration, codes.InvalidArgument},
		{domain.ErrStaffRegistration, codes.InvalidArgument},
		{domain.ErrUserExists, codes.AlreadyExists},
		{domain.ErrInvalidCredentials, codes.Unauthenticated},
		{domain.ErrInvalidToken, codes.Unauthenticated},
		{domain.ErrRefreshRevoked, codes.Unauthenticated},
		{fmt.Errorf("find user: %w", errors.New("connection refused")), codes.Internal},
	}
	for _, tc := range cases {
		if got := status.Code(toStatus(tc.err)); got != tc.want {
			t.Errorf("toStatus(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestToStatusHidesInternalErrorDetails(t *testing.T) {
	err := toStatus(errors.New("pq: password authentication failed for user swiftmed"))
	if msg := status.Convert(err).Message(); msg != "internal error" {
		t.Errorf("message = %q, want a generic message", msg)
	}
}

func TestRoleMappingRoundTrips(t *testing.T) {
	for _, role := range []authv1.Role{authv1.Role_ROLE_PATIENT, authv1.Role_ROLE_PHARMACY_STAFF, authv1.Role_ROLE_ADMIN} {
		d, ok := fromProtoRole(role)
		if !ok {
			t.Fatalf("fromProtoRole(%v) not ok", role)
		}
		if back := toProtoRole(d); back != role {
			t.Errorf("round trip %v -> %v -> %v", role, d, back)
		}
	}
	if _, ok := fromProtoRole(authv1.Role_ROLE_UNSPECIFIED); ok {
		t.Error("ROLE_UNSPECIFIED was accepted")
	}
	if got := toProtoRole(domain.Role("UNKNOWN")); got != authv1.Role_ROLE_UNSPECIFIED {
		t.Errorf("toProtoRole(UNKNOWN) = %v, want UNSPECIFIED", got)
	}
}

func TestRegisterWithUnspecifiedRoleIsInvalidArgument(t *testing.T) {
	_, err := newHandler().Register(context.Background(), &authv1.RegisterRequest{
		Email: "patient@example.com", Password: "correct horse",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRegisterRejectsPharmacyStaffRole(t *testing.T) {
	_, err := newHandler().Register(context.Background(), &authv1.RegisterRequest{
		Email: "staff@example.com", Password: "correct horse", Role: authv1.Role_ROLE_PHARMACY_STAFF,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// TestFullFlow exercises every RPC once through the handler.
func TestFullFlow(t *testing.T) {
	h := newHandler()
	ctx := context.Background()

	reg, err := h.Register(ctx, &authv1.RegisterRequest{
		Email: "patient@example.com", Password: "correct horse", Role: authv1.Role_ROLE_PATIENT,
	})
	if err != nil {
		t.Fatal(err)
	}

	login, err := h.Login(ctx, &authv1.LoginRequest{Email: "patient@example.com", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if login.GetUserId() != reg.GetUserId() || login.GetRole() != authv1.Role_ROLE_PATIENT {
		t.Fatalf("login = %+v", login)
	}
	if login.GetTokens().GetAccessTokenExpiresAt() == nil {
		t.Error("access token expiry missing")
	}

	who, err := h.ValidateToken(ctx, &authv1.ValidateTokenRequest{AccessToken: login.GetTokens().GetAccessToken()})
	if err != nil {
		t.Fatal(err)
	}
	if who.GetUserId() != reg.GetUserId() || who.GetRole() != authv1.Role_ROLE_PATIENT {
		t.Fatalf("validate = %+v", who)
	}

	refreshed, err := h.RefreshToken(ctx, &authv1.RefreshTokenRequest{RefreshToken: login.GetTokens().GetRefreshToken()})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.Logout(ctx, &authv1.LogoutRequest{RefreshToken: refreshed.GetTokens().GetRefreshToken()}); err != nil {
		t.Fatal(err)
	}
	_, err = h.RefreshToken(ctx, &authv1.RefreshTokenRequest{RefreshToken: refreshed.GetTokens().GetRefreshToken()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("refresh after logout: code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestLoginWrongPasswordIsUnauthenticated(t *testing.T) {
	h := newHandler()
	ctx := context.Background()
	if _, err := h.Register(ctx, &authv1.RegisterRequest{
		Email: "patient@example.com", Password: "correct horse", Role: authv1.Role_ROLE_PATIENT,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := h.Login(ctx, &authv1.LoginRequest{Email: "patient@example.com", Password: "wrong password"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}
