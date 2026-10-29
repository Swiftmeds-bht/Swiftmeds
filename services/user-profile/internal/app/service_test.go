package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
)

type fakeRepository struct {
	profiles map[string]domain.Profile
	err      error
}

func (f *fakeRepository) Create(_ context.Context, p domain.Profile) error {
	if f.err != nil {
		return f.err
	}
	if f.profiles == nil {
		f.profiles = map[string]domain.Profile{}
	}
	if _, ok := f.profiles[p.UserID]; ok {
		return domain.ErrProfileExists
	}
	f.profiles[p.UserID] = p
	return nil
}
func (f *fakeRepository) Get(_ context.Context, id string) (domain.Profile, error) {
	if f.err != nil {
		return domain.Profile{}, f.err
	}
	p, ok := f.profiles[id]
	if !ok {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	return p, nil
}
func (f *fakeRepository) Update(_ context.Context, p domain.Profile) error {
	if f.err != nil {
		return f.err
	}
	f.profiles[p.UserID] = p
	return nil
}

func TestCreateGetUpdateProfile(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepository{profiles: map[string]domain.Profile{}}
	svc := NewService(repo, func() time.Time { return now })
	input := Input{UserID: "u1", Name: "Tashi", Phone: "+97517123456", HomeLocation: "Thimphu", PreferredLanguage: "en"}
	created, err := svc.CreateProfile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetProfile(context.Background(), "u1")
	if err != nil || got != created {
		t.Fatalf("get = %+v, %v; want %+v", got, err, created)
	}
	input.Name = "Pema"
	updated, err := svc.UpdateProfile(context.Background(), input)
	if err != nil || updated.Name != "Pema" || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("update = %+v, %v", updated, err)
	}
}

func TestCreateProfileRejectsInvalidInputAndWrapsRepositoryErrors(t *testing.T) {
	svc := NewService(&fakeRepository{profiles: map[string]domain.Profile{}}, time.Now)
	_, err := svc.CreateProfile(context.Background(), Input{UserID: "u1", Phone: "+97517123456"})
	if !errors.Is(err, domain.ErrMissingName) {
		t.Fatalf("error = %v, want missing name", err)
	}
	dbErr := errors.New("db down")
	svc = NewService(&fakeRepository{err: dbErr}, time.Now)
	_, err = svc.CreateProfile(context.Background(), Input{UserID: "u1", Name: "Tashi", Phone: "+97517123456"})
	if !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want wrapped db error", err)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	svc := NewService(&fakeRepository{profiles: map[string]domain.Profile{}}, time.Now)
	_, err := svc.GetProfile(context.Background(), "missing")
	if !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
	_, err = svc.GetProfile(context.Background(), "")
	if !errors.Is(err, domain.ErrMissingUserID) {
		t.Fatalf("error = %v, want missing user id", err)
	}
}

func TestUpdateProfileValidationAndRepositoryError(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepository{profiles: map[string]domain.Profile{}}
	svc := NewService(repo, func() time.Time { return now })
	if _, err := svc.UpdateProfile(context.Background(), Input{}); !errors.Is(err, domain.ErrMissingUserID) {
		t.Fatalf("error = %v, want missing user id", err)
	}

	created, err := svc.CreateProfile(context.Background(), Input{UserID: "u1", Name: "Tashi", Phone: "+97517123456"})
	if err != nil {
		t.Fatal(err)
	}
	repo.err = errors.New("db down")
	_, err = svc.UpdateProfile(context.Background(), Input{UserID: created.UserID, Name: "Pema", Phone: "+97517123456"})
	if !errors.Is(err, repo.err) {
		t.Fatalf("error = %v, want wrapped repository error", err)
	}
}
