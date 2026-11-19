package app

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type fakeRepository struct {
	users   map[string]domain.User
	byEmail map[string]string
	refresh map[string]string

	// failWith, when set, is returned by every read and token method to
	// simulate the database being unavailable.
	failWith error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{users: map[string]domain.User{}, byEmail: map[string]string{}, refresh: map[string]string{}}
}

func (r *fakeRepository) RevokeRefreshToken(_ context.Context, tokenHash string) error {
	if r.failWith != nil {
		return r.failWith
	}
	delete(r.refresh, tokenHash)
	return nil
}

func (r *fakeRepository) CreateUser(_ context.Context, user domain.User) error {
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.ErrUserExists
	}
	r.users[user.ID] = user
	r.byEmail[user.Email] = user.ID
	return nil
}

func (r *fakeRepository) FindUserByEmail(_ context.Context, email string) (domain.User, error) {
	if r.failWith != nil {
		return domain.User{}, r.failWith
	}
	id, ok := r.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.users[id], nil
}

func (r *fakeRepository) FindUserByID(_ context.Context, id string) (domain.User, error) {
	user, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}

func (r *fakeRepository) SaveRefreshToken(_ context.Context, tokenHash, userID string, _ time.Time) error {
	r.refresh[tokenHash] = userID
	return nil
}

func (r *fakeRepository) ConsumeRefreshToken(_ context.Context, tokenHash string) (string, error) {
	if r.failWith != nil {
		return "", r.failWith
	}
	userID, ok := r.refresh[tokenHash]
	if !ok {
		return "", domain.ErrRefreshRevoked
	}
	delete(r.refresh, tokenHash)
	return userID, nil
}

func newTestService(repo *fakeRepository, now func() time.Time) *Service {
	return NewService(repo, []byte("test-secret-that-is-long-enough"), "test", time.Minute, time.Hour, now)
}

func TestRegisterAndLoginHashPassword(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)

	user, err := svc.Register(context.Background(), " Test@Example.com ", "correct horse", domain.RolePatient)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "test@example.com" || user.PasswordHash == "correct horse" {
		t.Fatalf("user was not normalized and hashed: %+v", user)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(user.ID) {
		t.Fatalf("user ID %q is not an RFC 4122 version 4 UUID", user.ID)
	}
	_, _, err = svc.Login(context.Background(), user.Email, "wrong password")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want invalid credentials", err)
	}
	loggedIn, tokens, err := svc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if loggedIn.ID != user.ID || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("login result = %+v, %+v", loggedIn, tokens)
	}
}

func TestValidateTokenRejectsTamperedAndExpiredTokens(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	user, err := svc.Register(context.Background(), "patient@example.com", "correct horse", domain.RolePatient)
	if err != nil {
		t.Fatal(err)
	}
	_, tokens, err := svc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := svc.ValidateToken(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != user.ID || identity.Role != domain.RolePatient {
		t.Fatalf("identity = %+v", identity)
	}
	if _, err := svc.ValidateToken(context.Background(), tokens.AccessToken+"tampered"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("tampered token error = %v", err)
	}

	expiredSvc := newTestService(repo, func() time.Time { return time.Now().Add(-2 * time.Minute) })
	_, expired, err := expiredSvc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(context.Background(), expired.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestLinkedStaffPharmacyIsIncludedInTokenIdentity(t *testing.T) {
	repo := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{ID: "32000000-0000-4000-8000-000000000001", Email: "staff@example.com", PasswordHash: string(hash), Role: domain.RolePharmacist, PharmacyID: "pharmacy-123", CreatedAt: time.Now()}
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(repo, time.Now)
	_, tokens, err := svc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := svc.ValidateToken(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Role != domain.RolePharmacist || identity.PharmacyID != user.PharmacyID {
		t.Fatalf("identity = %+v, want linked pharmacy %q", identity, user.PharmacyID)
	}
}

func TestRefreshTokenIsRotatedAndCannotBeReused(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	user, err := svc.Register(context.Background(), "patient@example.com", "correct horse", domain.RolePatient)
	if err != nil {
		t.Fatal(err)
	}
	_, tokens, err := svc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := svc.Refresh(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := svc.Refresh(context.Background(), tokens.RefreshToken); !errors.Is(err, domain.ErrRefreshRevoked) {
		t.Fatalf("reused refresh token error = %v", err)
	}
}
