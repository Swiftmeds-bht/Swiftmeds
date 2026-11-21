package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	userprofilev1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/user-profile/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
)

// memoryRepository is a small ProfileRepository; set err to make every call
// fail, or createErr to make only Create fail.
type memoryRepository struct {
	profiles  map[string]domain.Profile
	err       error
	createErr error
}

func (r *memoryRepository) Create(_ context.Context, p domain.Profile) error {
	if r.err != nil {
		return r.err
	}
	if r.createErr != nil {
		return r.createErr
	}
	if _, ok := r.profiles[p.UserID]; ok {
		return domain.ErrProfileExists
	}
	r.profiles[p.UserID] = p
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id string) (domain.Profile, error) {
	if r.err != nil {
		return domain.Profile{}, r.err
	}
	p, ok := r.profiles[id]
	if !ok {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	return p, nil
}

func (r *memoryRepository) Update(_ context.Context, p domain.Profile) error {
	if r.err != nil {
		return r.err
	}
	r.profiles[p.UserID] = p
	return nil
}

var (
	t1 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	t2 = t1.Add(time.Hour)
)

func newHandler(repo *memoryRepository, clock *time.Time) *Handler {
	if repo.profiles == nil {
		repo.profiles = map[string]domain.Profile{}
	}
	return NewHandler(app.NewService(repo, func() time.Time { return *clock }))
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (%v), want %s", got, err, want)
	}
}

// UpdateProfile is create-or-update: the first call creates the profile, the
// next one changes it and keeps created_at.
func TestUpdateProfileCreatesThenUpdates(t *testing.T) {
	repo := &memoryRepository{}
	clock := t1
	h := newHandler(repo, &clock)
	ctx := context.Background()

	first, err := h.UpdateProfile(ctx, &userprofilev1.UpdateProfileRequest{UserId: "u-1", DisplayName: "Tashi", Phone: "+97517123456"})
	if err != nil {
		t.Fatalf("first UpdateProfile: %v", err)
	}
	p := first.GetProfile()
	if p.GetUserId() != "u-1" || p.GetDisplayName() != "Tashi" || p.GetPhone() != "+97517123456" ||
		!p.GetCreatedAt().AsTime().Equal(t1) || !p.GetUpdatedAt().AsTime().Equal(t1) {
		t.Fatalf("created profile = %v", p)
	}

	clock = t2
	second, err := h.UpdateProfile(ctx, &userprofilev1.UpdateProfileRequest{UserId: "u-1", DisplayName: "Pema", Phone: "+97517123456"})
	if err != nil {
		t.Fatalf("second UpdateProfile: %v", err)
	}
	p = second.GetProfile()
	if p.GetDisplayName() != "Pema" || !p.GetCreatedAt().AsTime().Equal(t1) || !p.GetUpdatedAt().AsTime().Equal(t2) {
		t.Fatalf("updated profile = %v, want name Pema, created %v, updated %v", p, t1, t2)
	}
	if len(repo.profiles) != 1 {
		t.Fatalf("stored %d profiles, want 1", len(repo.profiles))
	}
}

func TestGetProfile(t *testing.T) {
	repo := &memoryRepository{profiles: map[string]domain.Profile{
		"u-1": {UserID: "u-1", Name: "Tashi", Phone: "+97517123456", CreatedAt: t1, UpdatedAt: t2},
	}}
	clock := t2
	h := newHandler(repo, &clock)

	resp, err := h.GetProfile(context.Background(), &userprofilev1.GetProfileRequest{UserId: "u-1"})
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p := resp.GetProfile(); p.GetDisplayName() != "Tashi" || !p.GetUpdatedAt().AsTime().Equal(t2) {
		t.Fatalf("profile = %v", p)
	}

	_, err = h.GetProfile(context.Background(), &userprofilev1.GetProfileRequest{UserId: "ghost"})
	wantCode(t, err, codes.NotFound)

	// A nil request has no user ID, which is INVALID_ARGUMENT, not a crash.
	_, err = h.GetProfile(context.Background(), nil)
	wantCode(t, err, codes.InvalidArgument)
}

func TestUpdateProfileMaskPreservesUnselectedFields(t *testing.T) {
	repo := &memoryRepository{profiles: map[string]domain.Profile{
		"u-1": {UserID: "u-1", Name: "Tashi", Phone: "+97517123456", HomeLocation: "Thimphu", PreferredLanguage: "dz", CreatedAt: t1, UpdatedAt: t1},
	}}
	clock := t2
	h := newHandler(repo, &clock)
	resp, err := h.UpdateProfile(context.Background(), &userprofilev1.UpdateProfileRequest{
		UserId: "u-1", DisplayName: "Pema", UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got := resp.GetProfile()
	if got.GetDisplayName() != "Pema" || got.GetPhone() != "+97517123456" || got.GetHomeLocation() != "Thimphu" || got.GetPreferredLanguage() != "dz" {
		t.Fatalf("partial update lost unselected fields: %v", got)
	}
	stored := repo.profiles["u-1"]
	if stored.HomeLocation != "Thimphu" || stored.PreferredLanguage != "dz" {
		t.Fatalf("stored profile lost unselected fields: %+v", stored)
	}
}

func TestUpdateProfileValidationIsInvalidArgument(t *testing.T) {
	clock := t1
	h := newHandler(&memoryRepository{}, &clock)
	for name, req := range map[string]*userprofilev1.UpdateProfileRequest{
		"no user":   {DisplayName: "Tashi", Phone: "+97517123456"},
		"no name":   {UserId: "u-1", Phone: "+97517123456"},
		"bad phone": {UserId: "u-1", DisplayName: "Tashi", Phone: "12345"},
		"nil":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.UpdateProfile(context.Background(), req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}

// Two first-time UpdateProfile calls can race; the loser's insert finds the
// row and the caller gets ALREADY_EXISTS.
func TestUpdateProfileCreateRaceIsAlreadyExists(t *testing.T) {
	clock := t1
	h := newHandler(&memoryRepository{createErr: domain.ErrProfileExists}, &clock)
	_, err := h.UpdateProfile(context.Background(), &userprofilev1.UpdateProfileRequest{
		UserId: "u-1", DisplayName: "Tashi", Phone: "+97517123456",
	})
	wantCode(t, err, codes.AlreadyExists)
}

func TestRepositoryFailureIsInternalAndHidesTheCause(t *testing.T) {
	clock := t1
	h := newHandler(&memoryRepository{err: errors.New("pq: password authentication failed")}, &clock)

	_, err1 := h.GetProfile(context.Background(), &userprofilev1.GetProfileRequest{UserId: "u-1"})
	_, err2 := h.UpdateProfile(context.Background(), &userprofilev1.UpdateProfileRequest{
		UserId: "u-1", DisplayName: "Tashi", Phone: "+97517123456",
	})
	for i, err := range []error{err1, err2} {
		wantCode(t, err, codes.Internal)
		if strings.Contains(err.Error(), "password") {
			t.Fatalf("rpc %d: error %q leaks the internal cause", i+1, err)
		}
	}
}
