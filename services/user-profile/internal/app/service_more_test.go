package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
)

// countingRepository wraps fakeRepository and counts writes.
type countingRepository struct {
	fakeRepository
	creates, updates int
}

func (r *countingRepository) Create(ctx context.Context, p domain.Profile) error {
	r.creates++
	return r.fakeRepository.Create(ctx, p)
}

func (r *countingRepository) Update(ctx context.Context, p domain.Profile) error {
	r.updates++
	return r.fakeRepository.Update(ctx, p)
}

func TestUpdateProfileKeepsCreatedAtAndMovesUpdatedAt(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	later := created.Add(48 * time.Hour)
	clock := created
	repo := &countingRepository{fakeRepository: fakeRepository{profiles: map[string]domain.Profile{}}}
	svc := NewService(repo, func() time.Time { return clock })

	if _, err := svc.CreateProfile(context.Background(), Input{
		UserID: "u-1", Name: "Tashi", Phone: "+97517123456", HomeLocation: "Paro", PreferredLanguage: "dz",
	}); err != nil {
		t.Fatal(err)
	}
	clock = later
	updated, err := svc.UpdateProfile(context.Background(), Input{
		UserID: "u-1", Name: " Pema ", Phone: "+97577123456", HomeLocation: "Thimphu", PreferredLanguage: "en",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	want := domain.Profile{
		UserID: "u-1", Name: "Pema", Phone: "+97577123456", HomeLocation: "Thimphu",
		PreferredLanguage: "en", CreatedAt: created, UpdatedAt: later,
	}
	if updated != want {
		t.Fatalf("updated = %+v\nwant      %+v", updated, want)
	}
	if stored := repo.profiles["u-1"]; stored != want {
		t.Fatalf("stored = %+v, want %+v", stored, want)
	}
	if repo.creates != 1 || repo.updates != 1 {
		t.Fatalf("creates %d updates %d, want 1 and 1", repo.creates, repo.updates)
	}
}

func TestUpdateProfileOfAnUnknownUserIsNotFound(t *testing.T) {
	repo := &countingRepository{fakeRepository: fakeRepository{profiles: map[string]domain.Profile{}}}
	_, err := NewService(repo, nil).UpdateProfile(context.Background(), Input{UserID: "ghost", Name: "X", Phone: "+97517123456"})
	if !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatalf("error = %v, want ErrProfileNotFound (the gRPC handler creates the profile then)", err)
	}
	if repo.updates != 0 {
		t.Fatalf("updates = %d, want 0", repo.updates)
	}
}

func TestUpdateProfileWithInvalidInputChangesNothing(t *testing.T) {
	repo := &countingRepository{fakeRepository: fakeRepository{profiles: map[string]domain.Profile{
		"u-1": {UserID: "u-1", Name: "Tashi", Phone: "+97517123456"},
	}}}
	svc := NewService(repo, nil)

	for _, in := range []Input{
		{UserID: "u-1", Name: " ", Phone: "+97517123456"},
		{UserID: "u-1", Name: "Pema", Phone: "017123456"},
	} {
		if _, err := svc.UpdateProfile(context.Background(), in); !errors.Is(err, domain.ErrMissingName) && !errors.Is(err, domain.ErrInvalidPhone) {
			t.Fatalf("UpdateProfile(%+v) error = %v, want a validation error", in, err)
		}
	}
	if repo.updates != 0 || repo.profiles["u-1"].Name != "Tashi" {
		t.Fatalf("profile changed: updates %d, stored %+v", repo.updates, repo.profiles["u-1"])
	}
}

func TestCreateProfileTwiceIsRejected(t *testing.T) {
	svc := NewService(&fakeRepository{profiles: map[string]domain.Profile{}}, nil)
	in := Input{UserID: "u-1", Name: "Tashi", Phone: "+97517123456"}
	if _, err := svc.CreateProfile(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProfile(context.Background(), in); !errors.Is(err, domain.ErrProfileExists) {
		t.Fatalf("second create error = %v, want ErrProfileExists", err)
	}
}

func TestNewServiceDefaultsTheClock(t *testing.T) {
	before := time.Now()
	p, err := NewService(&fakeRepository{profiles: map[string]domain.Profile{}}, nil).
		CreateProfile(context.Background(), Input{UserID: "u-1", Name: "Tashi", Phone: "+97517123456"})
	if err != nil {
		t.Fatal(err)
	}
	if p.CreatedAt.Before(before) || p.CreatedAt.After(time.Now()) {
		t.Fatalf("created_at = %v, want the current time", p.CreatedAt)
	}
}

func TestGetProfileWrapsRepositoryErrors(t *testing.T) {
	dbErr := errors.New("db down")
	_, err := NewService(&fakeRepository{err: dbErr}, nil).GetProfile(context.Background(), "u-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want the wrapped repository error", err)
	}
}
