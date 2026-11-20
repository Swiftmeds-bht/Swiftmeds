package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

func linkHandler(t *testing.T) (*Handler, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	for _, u := range []domain.User{
		{ID: "u1", Email: "owner@example.bt", PasswordHash: "h", Role: domain.RolePatient, CreatedAt: at},
		{ID: "a1", Email: "admin@example.bt", PasswordHash: "h", Role: domain.RoleAdmin, CreatedAt: at},
	} {
		if err := repo.CreateUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	svc := app.NewService(repo, []byte("test-secret-at-least-32-bytes-long!"), "swiftmed-auth", time.Minute, time.Hour, func() time.Time { return at })
	return NewHandler(svc), repo
}

// SCRUM-141: registering a pharmacy makes the account its staff account.
func TestLinkPharmacyMakesTheAccountStaffOfThePharmacy(t *testing.T) {
	h, repo := linkHandler(t)
	resp, err := h.LinkPharmacy(context.Background(), &authv1.LinkPharmacyRequest{UserId: "u1", PharmacyId: "ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetRole() != authv1.Role_ROLE_PHARMACY_STAFF || resp.GetPharmacyId() != "ph-1" || resp.GetUserId() != "u1" {
		t.Fatalf("response = %+v", resp)
	}
	if u := repo.users["u1"]; u.Role != domain.RolePharmacist || u.PharmacyID != "ph-1" {
		t.Fatalf("stored user = %+v", u)
	}
	// The same call again (a retried request) succeeds.
	if _, err := h.LinkPharmacy(context.Background(), &authv1.LinkPharmacyRequest{UserId: "u1", PharmacyId: "ph-1"}); err != nil {
		t.Fatalf("repeat: %v", err)
	}
}

func TestLinkPharmacyErrors(t *testing.T) {
	cases := []struct {
		name string
		req  *authv1.LinkPharmacyRequest
		want codes.Code
	}{
		{"missing pharmacy", &authv1.LinkPharmacyRequest{UserId: "u1"}, codes.InvalidArgument},
		{"missing user", &authv1.LinkPharmacyRequest{PharmacyId: "ph-1"}, codes.InvalidArgument},
		{"unknown user", &authv1.LinkPharmacyRequest{UserId: "nobody", PharmacyId: "ph-1"}, codes.NotFound},
		{"admin", &authv1.LinkPharmacyRequest{UserId: "a1", PharmacyId: "ph-1"}, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := linkHandler(t)
			_, err := h.LinkPharmacy(context.Background(), tc.req)
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v, want %v (err %v)", status.Code(err), tc.want, err)
			}
		})
	}
}

func TestLinkPharmacyRefusesASecondPharmacy(t *testing.T) {
	h, _ := linkHandler(t)
	ctx := context.Background()
	if _, err := h.LinkPharmacy(ctx, &authv1.LinkPharmacyRequest{UserId: "u1", PharmacyId: "ph-1"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.LinkPharmacy(ctx, &authv1.LinkPharmacyRequest{UserId: "u1", PharmacyId: "ph-2"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", status.Code(err))
	}
}
