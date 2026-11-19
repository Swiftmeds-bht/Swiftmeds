// Package repotest defines the behavior required of every pharmacy repository.
package repotest

import (
	"context"
	"errors"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
	"reflect"
	"testing"
	"time"
)

type Factory func(*testing.T) ports.PharmacyRepository

func Run(t *testing.T, factory Factory) {
	t.Run("create and get preserve fields including hours and district", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		want := pharmacy("ph-1", "LIC-1")
		if err := r.Create(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := r.Get(ctx, want.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !same(got, want) {
			t.Fatalf("got %+v want %+v", got, want)
		}
	})
	t.Run("missing pharmacy maps to not found", func(t *testing.T) {
		r := factory(t)
		if _, err := r.Get(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("get=%v", err)
		}
		if _, err := r.SetStatus(context.Background(), "missing", domain.StatusVerified, "", time.Now()); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("status=%v", err)
		}
	})
	t.Run("list is ordered and verified filter excludes pending", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		a := pharmacy("ph-b", "LIC-B")
		b := pharmacy("ph-a", "LIC-A")
		if err := r.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := r.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
		if _, err := r.SetStatus(ctx, a.ID, domain.StatusVerified, "", t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		all, err := r.List(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 || all[0].ID != "ph-a" || all[1].ID != "ph-b" {
			t.Fatalf("all=%+v", all)
		}
		verified, err := r.List(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(verified) != 1 || verified[0].ID != "ph-b" {
			t.Fatalf("verified=%+v", verified)
		}
	})
	t.Run("status transition persists reason and verification time", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		p := pharmacy("ph-1", "LIC-1")
		if err := r.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		at := t0.Add(time.Hour)
		got, err := r.SetStatus(ctx, p.ID, domain.StatusVerified, "", at)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.StatusVerified || got.VerifiedAt == nil || !got.VerifiedAt.Equal(at) {
			t.Fatalf("status result=%+v", got)
		}
		stored, err := r.Get(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !same(stored, got) {
			t.Fatalf("stored=%+v want=%+v", stored, got)
		}
	})
	t.Run("invalid transition and missing rejection reason are returned", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		p := pharmacy("ph-1", "LIC-1")
		if err := r.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		if _, err := r.SetStatus(ctx, p.ID, domain.StatusSuspended, "", t0); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("transition=%v", err)
		}
		if _, err := r.SetStatus(ctx, p.ID, domain.StatusRejected, "", t0); !errors.Is(err, domain.ErrMissingReason) {
			t.Fatalf("reason=%v", err)
		}
	})
}

var t0 = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

func pharmacy(id, license string) domain.Pharmacy {
	at := t0
	p, _ := domain.NewPharmacy(id, "owner-1", "Paro Pharmacy", license, "Paro", domain.GeoPoint{Latitude: 27.43, Longitude: 89.41}, []domain.OpeningHours{{Day: 1, OpensAt: "09:00", ClosesAt: "18:00"}}, at)
	return p
}
func same(a, b domain.Pharmacy) bool {
	if a.ID != b.ID || a.OwnerID != b.OwnerID || a.Name != b.Name || a.LicenseNumber != b.LicenseNumber || a.Dzongkhag != b.Dzongkhag || a.Location != b.Location || a.Status != b.Status || a.StatusReason != b.StatusReason || !a.CreatedAt.Equal(b.CreatedAt) || !reflect.DeepEqual(a.Hours, b.Hours) {
		return false
	}
	if a.VerifiedAt == nil || b.VerifiedAt == nil {
		return a.VerifiedAt == nil && b.VerifiedAt == nil
	}
	return a.VerifiedAt.Equal(*b.VerifiedAt)
}
