package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

var errDBDown = errors.New("connection refused")

// registerAndLogin creates a patient and returns their tokens.
func registerAndLogin(t *testing.T, svc *Service) (domain.User, TokenPair) {
	t.Helper()
	user, err := svc.Register(context.Background(), "patient@example.com", "correct horse", domain.RolePatient)
	if err != nil {
		t.Fatal(err)
	}
	_, tokens, err := svc.Login(context.Background(), user.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	return user, tokens
}

func TestRegisterRejectsPasswordLongerThanBcryptAllows(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)

	_, err := svc.Register(context.Background(), "patient@example.com", strings.Repeat("p", 73), domain.RolePatient)
	if !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("err = %v, want ErrInvalidPassword (not a bcrypt failure)", err)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	ctx := context.Background()

	if _, err := svc.Register(ctx, "patient@example.com", "correct horse", domain.RolePatient); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Register(ctx, " PATIENT@example.com", "another password", domain.RolePatient)
	if !errors.Is(err, domain.ErrUserExists) {
		t.Fatalf("err = %v, want ErrUserExists (email compared after normalising)", err)
	}
}

func TestLoginUnknownEmailIsInvalidCredentials(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)

	_, _, err := svc.Login(context.Background(), "nobody@example.com", "correct horse")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginDatabaseErrorIsNotReportedAsBadCredentials(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	repo.failWith = errDBDown

	_, _, err := svc.Login(context.Background(), "patient@example.com", "correct horse")
	if errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatal("a database outage was reported as invalid credentials")
	}
	if !errors.Is(err, errDBDown) {
		t.Fatalf("err = %v, want the wrapped database error", err)
	}
}

func TestRefreshDatabaseErrorIsNotReportedAsBadToken(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	_, tokens := registerAndLogin(t, svc)
	repo.failWith = errDBDown

	_, err := svc.Refresh(context.Background(), tokens.RefreshToken)
	if errors.Is(err, domain.ErrInvalidToken) || errors.Is(err, domain.ErrRefreshRevoked) {
		t.Fatalf("a database outage was reported as a bad token: %v", err)
	}
	if !errors.Is(err, errDBDown) {
		t.Fatalf("err = %v, want the wrapped database error", err)
	}
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	_, tokens := registerAndLogin(t, svc)

	if _, err := svc.Refresh(context.Background(), tokens.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken for an access token", err)
	}
}

func TestValidateTokenRejectsRefreshToken(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	_, tokens := registerAndLogin(t, svc)

	if _, err := svc.ValidateToken(context.Background(), tokens.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken for a refresh token", err)
	}
}

func TestValidateTokenRejectsTokenFromAnotherIssuerOrSecret(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	_, tokens := registerAndLogin(t, svc)

	otherSecret := NewService(repo, []byte("a-completely-different-secret"), "test", time.Minute, time.Hour, time.Now)
	if _, err := otherSecret.ValidateToken(context.Background(), tokens.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("other secret: err = %v, want ErrInvalidToken", err)
	}
	otherIssuer := NewService(repo, []byte("test-secret-that-is-long-enough"), "someone-else", time.Minute, time.Hour, time.Now)
	if _, err := otherIssuer.ValidateToken(context.Background(), tokens.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("other issuer: err = %v, want ErrInvalidToken", err)
	}
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	_, tokens := registerAndLogin(t, svc)

	if err := svc.Logout(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(context.Background(), tokens.RefreshToken); !errors.Is(err, domain.ErrRefreshRevoked) {
		t.Fatalf("refresh after logout: err = %v, want ErrRefreshRevoked", err)
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	_, tokens := registerAndLogin(t, svc)

	for i := 0; i < 2; i++ {
		if err := svc.Logout(context.Background(), tokens.RefreshToken); err != nil {
			t.Fatalf("logout #%d: %v", i+1, err)
		}
	}
}

func TestLogoutRejectsInvalidTokens(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	_, tokens := registerAndLogin(t, svc)

	for name, token := range map[string]string{
		"garbage":      "not-a-jwt",
		"access token": tokens.AccessToken,
		"tampered":     tokens.RefreshToken + "x",
	} {
		if err := svc.Logout(context.Background(), token); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("%s: err = %v, want ErrInvalidToken", name, err)
		}
	}
}

func TestLogoutDatabaseErrorIsReturned(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo, time.Now)
	_, tokens := registerAndLogin(t, svc)
	repo.failWith = errDBDown

	if err := svc.Logout(context.Background(), tokens.RefreshToken); !errors.Is(err, errDBDown) {
		t.Fatalf("err = %v, want the wrapped database error", err)
	}
}

func TestNewServiceAppliesDefaults(t *testing.T) {
	svc := NewService(newFakeRepository(), []byte("s"), "test", 0, -1, nil)

	if svc.accessTTL != defaultAccessTTL || svc.refreshTTL != defaultRefreshTTL {
		t.Errorf("TTLs = %v / %v, want defaults %v / %v", svc.accessTTL, svc.refreshTTL, defaultAccessTTL, defaultRefreshTTL)
	}
	if svc.now == nil {
		t.Error("now is nil, want time.Now")
	}
}
