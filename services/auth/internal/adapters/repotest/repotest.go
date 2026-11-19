// Package repotest defines the behavior required of every auth user repository.
package repotest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
)

type Factory func(*testing.T) ports.UserRepository

func Run(t *testing.T, factory Factory) {
	t.Run("users round trip by id and email", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		want := domain.User{ID: "u1", Email: "a@example.com", PasswordHash: "hash", Role: domain.RolePatient, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
		if err := r.CreateUser(ctx, want); err != nil {
			t.Fatal(err)
		}
		gotID, err := r.FindUserByID(ctx, want.ID)
		if err != nil {
			t.Fatal(err)
		}
		gotEmail, err := r.FindUserByEmail(ctx, want.Email)
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range []domain.User{gotID, gotEmail} {
			if got.ID != want.ID || got.Email != want.Email || got.PasswordHash != want.PasswordHash || got.Role != want.Role || got.PharmacyID != want.PharmacyID || !got.CreatedAt.Equal(want.CreatedAt) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		}
	})
	t.Run("duplicate id and email map to user exists", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		u := domain.User{ID: "u1", Email: "a@example.com", PasswordHash: "hash", Role: domain.RolePatient, CreatedAt: time.Now().UTC()}
		if err := r.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		byID := u
		byID.Email = "b@example.com"
		if err := r.CreateUser(ctx, byID); !errors.Is(err, domain.ErrUserExists) {
			t.Fatalf("duplicate id error=%v", err)
		}
		byEmail := u
		byEmail.ID = "u2"
		if err := r.CreateUser(ctx, byEmail); !errors.Is(err, domain.ErrUserExists) {
			t.Fatalf("duplicate email error=%v", err)
		}
	})
	t.Run("missing user maps to not found", func(t *testing.T) {
		r := factory(t)
		if _, err := r.FindUserByID(context.Background(), "missing"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("id error=%v", err)
		}
		if _, err := r.FindUserByEmail(context.Background(), "missing@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("email error=%v", err)
		}
	})
	t.Run("refresh tokens are consumed once and revocation is idempotent", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		u := domain.User{ID: "u1", Email: "a@example.com", PasswordHash: "hash", Role: domain.RolePatient, CreatedAt: time.Now().UTC()}
		if err := r.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		exp := time.Now().Add(time.Hour)
		if err := r.SaveRefreshToken(ctx, "token", u.ID, exp); err != nil {
			t.Fatal(err)
		}
		if id, err := r.ConsumeRefreshToken(ctx, "token"); err != nil || id != u.ID {
			t.Fatalf("consume=(%q,%v)", id, err)
		}
		if _, err := r.ConsumeRefreshToken(ctx, "token"); !errors.Is(err, domain.ErrRefreshRevoked) {
			t.Fatalf("second consume=%v", err)
		}
		if err := r.RevokeRefreshToken(ctx, "unknown"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("expired refresh token cannot be consumed", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		u := domain.User{ID: "u1", Email: "a@example.com", PasswordHash: "hash", Role: domain.RolePatient, CreatedAt: time.Now().UTC()}
		if err := r.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := r.SaveRefreshToken(ctx, "expired", u.ID, time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := r.ConsumeRefreshToken(ctx, "expired"); !errors.Is(err, domain.ErrRefreshRevoked) {
			t.Fatalf("consume=%v", err)
		}
	})
}
