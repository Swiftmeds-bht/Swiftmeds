// Package repotest defines the behavior required of every profile repository.
package repotest

import (
	"context"
	"errors"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
	"testing"
	"time"
)

type Factory func(*testing.T) ports.ProfileRepository

func Run(t *testing.T, factory Factory) {
	t.Run("create and get preserve every field", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		want := profile("u1", "Ada", time.Now().UTC().Truncate(time.Microsecond))
		if err := r.Create(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := r.Get(ctx, want.UserID)
		if err != nil {
			t.Fatal(err)
		}
		if !sameProfile(got, want) {
			t.Fatalf("got %+v want %+v", got, want)
		}
	})
	t.Run("duplicate user id maps to profile exists", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		p := profile("u1", "Ada", time.Now().UTC())
		if err := r.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		p.Name = "Grace"
		if err := r.Create(ctx, p); !errors.Is(err, domain.ErrProfileExists) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("missing get and update map to not found", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		if _, err := r.Get(ctx, "missing"); !errors.Is(err, domain.ErrProfileNotFound) {
			t.Fatalf("get=%v", err)
		}
		if err := r.Update(ctx, profile("missing", "Ada", time.Now())); !errors.Is(err, domain.ErrProfileNotFound) {
			t.Fatalf("update=%v", err)
		}
	})
	t.Run("update changes editable fields and timestamp", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		p := profile("u1", "Ada", time.Now().UTC())
		if err := r.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		p.Name = "Grace"
		p.Phone = "+12025550123"
		p.HomeLocation = "Thimphu"
		p.PreferredLanguage = "dz"
		p.UpdatedAt = p.UpdatedAt.Add(time.Hour)
		if err := r.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := r.Get(ctx, p.UserID)
		if err != nil {
			t.Fatal(err)
		}
		if !sameProfile(got, p) {
			t.Fatalf("got %+v want %+v", got, p)
		}
	})
}

func sameProfile(a, b domain.Profile) bool {
	return a.UserID == b.UserID && a.Name == b.Name && a.Phone == b.Phone &&
		a.HomeLocation == b.HomeLocation && a.PreferredLanguage == b.PreferredLanguage &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

func profile(id, name string, at time.Time) domain.Profile {
	at = at.UTC().Truncate(time.Microsecond)
	return domain.Profile{UserID: id, Name: name, Phone: "+97517123456", HomeLocation: "Paro", PreferredLanguage: "en", CreatedAt: at, UpdatedAt: at}
}
