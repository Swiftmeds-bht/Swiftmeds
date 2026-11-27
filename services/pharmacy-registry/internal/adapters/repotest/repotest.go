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
	t.Run("get many returns only matching pharmacies once", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		for _, p := range []domain.Pharmacy{pharmacy("ph-b", "LIC-B"), pharmacy("ph-a", "LIC-A")} {
			if err := r.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		got, err := r.GetMany(ctx, []string{"ph-b", "missing", "ph-a", "ph-b"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != "ph-a" || got[1].ID != "ph-b" {
			t.Fatalf("get many=%+v", got)
		}
		empty, err := r.GetMany(ctx, nil)
		if err != nil || len(empty) != 0 {
			t.Fatalf("empty get many=%+v err=%v", empty, err)
		}
	})
	t.Run("status list pages in creation order without duplicates", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		a := pharmacy("ph-a", "LIC-A")
		b := pharmacy("ph-b", "LIC-B")
		b.CreatedAt = t0.Add(time.Minute)
		c := pharmacy("ph-c", "LIC-C")
		c.CreatedAt = t0.Add(2 * time.Minute)
		for _, p := range []domain.Pharmacy{c, b, a} {
			if err := r.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := r.SetStatus(ctx, b.ID, domain.StatusVerified, "", t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		first, token, err := r.ListPage(ctx, "", 2, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 2 || first[0].ID != "ph-a" || first[1].ID != "ph-b" || token == "" {
			t.Fatalf("first=%+v token=%q", first, token)
		}
		second, token, err := r.ListPage(ctx, "", 2, token)
		if err != nil {
			t.Fatal(err)
		}
		if len(second) != 1 || second[0].ID != "ph-c" || token != "" {
			t.Fatalf("second=%+v token=%q", second, token)
		}
		verified, token, err := r.ListPage(ctx, domain.StatusVerified, 2, "")
		if err != nil || len(verified) != 1 || verified[0].ID != "ph-b" || token != "" {
			t.Fatalf("verified=%+v token=%q err=%v", verified, token, err)
		}
		if _, _, err := r.ListPage(ctx, "", 2, "bad-token"); err == nil {
			t.Fatal("invalid token accepted")
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
	t.Run("phone and address round trip", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		want := pharmacy("ph-1", "LIC-1")
		want.Phone, want.Address = "+975 17 123 456", "Norzin Lam, Thimphu"
		if err := r.Create(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := r.Get(ctx, want.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Phone != want.Phone || got.Address != want.Address {
			t.Fatalf("got phone=%q address=%q", got.Phone, got.Address)
		}
	})
	t.Run("a second pharmacy with the same licence number is refused", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		if err := r.Create(ctx, pharmacy("ph-1", "LIC-1")); err != nil {
			t.Fatal(err)
		}
		if err := r.Create(ctx, pharmacy("ph-2", "LIC-1")); !errors.Is(err, domain.ErrLicenseTaken) {
			t.Fatalf("duplicate licence=%v", err)
		}
	})
	t.Run("find by owner returns the owner's oldest pharmacy or not found", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		if _, err := r.FindByOwner(ctx, "owner-1"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("empty=%v", err)
		}
		newer := pharmacy("ph-a", "LIC-A")
		newer.CreatedAt = t0.Add(time.Hour)
		older := pharmacy("ph-b", "LIC-B")
		other := pharmacy("ph-c", "LIC-C")
		other.OwnerID = "owner-2"
		for _, p := range []domain.Pharmacy{newer, older, other} {
			if err := r.Create(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		got, err := r.FindByOwner(ctx, "owner-1")
		if err != nil || got.ID != "ph-b" {
			t.Fatalf("got=%+v err=%v", got, err)
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
	t.Run("owner updates details and a rejected pharmacy returns to pending", func(t *testing.T) {
		r := factory(t)
		ctx := context.Background()
		p := pharmacy("ph-update", "LIC-UPD")
		if err := r.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		if _, err := r.SetStatus(ctx, p.ID, domain.StatusRejected, "fix the address", t0); err != nil {
			t.Fatal(err)
		}
		changes := domain.Pharmacy{Name: "Updated", Phone: "+975 17 111 222", Address: "New address", Dzongkhag: "Paro", Location: domain.GeoPoint{Latitude: 28, Longitude: 90}, Hours: []domain.OpeningHours{{Day: 2, OpensAt: "08:30", ClosesAt: "17:15"}}}
		if _, err := r.Update(ctx, p.ID, "other-owner", changes); !errors.Is(err, domain.ErrOwnerMismatch) {
			t.Fatalf("wrong owner update=%v", err)
		}
		got, err := r.Update(ctx, p.ID, "owner-1", changes)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.StatusPending || got.StatusReason != "" || got.Name != changes.Name || got.Location != changes.Location || !reflect.DeepEqual(got.Hours, changes.Hours) {
			t.Fatalf("updated=%+v", got)
		}
		stored, err := r.Get(ctx, p.ID)
		if err != nil || !same(stored, got) {
			t.Fatalf("stored=%+v err=%v", stored, err)
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
