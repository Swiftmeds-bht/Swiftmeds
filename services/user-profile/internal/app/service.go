package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
)

type Input struct {
	UserID            string
	Name              string
	Phone             string
	HomeLocation      string
	PreferredLanguage string
}

type Service struct {
	repo ports.ProfileRepository
	now  func() time.Time
}

func NewService(repo ports.ProfileRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

func (s *Service) CreateProfile(ctx context.Context, input Input) (domain.Profile, error) {
	profile, err := domain.NewProfile(input.UserID, input.Name, input.Phone, input.HomeLocation, input.PreferredLanguage, s.now())
	if err != nil {
		return domain.Profile{}, err
	}
	if err := s.repo.Create(ctx, profile); err != nil {
		return domain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	return profile, nil
}

func (s *Service) GetProfile(ctx context.Context, userID string) (domain.Profile, error) {
	if userID == "" {
		return domain.Profile{}, domain.ErrMissingUserID
	}
	profile, err := s.repo.Get(ctx, userID)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return profile, nil
}

func (s *Service) UpdateProfile(ctx context.Context, input Input) (domain.Profile, error) {
	if input.UserID == "" {
		return domain.Profile{}, domain.ErrMissingUserID
	}
	existing, err := s.repo.Get(ctx, input.UserID)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("get profile for update: %w", err)
	}
	updated, err := domain.NewProfile(input.UserID, input.Name, input.Phone, input.HomeLocation, input.PreferredLanguage, s.now())
	if err != nil {
		return domain.Profile{}, err
	}
	updated.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(ctx, updated); err != nil {
		return domain.Profile{}, fmt.Errorf("update profile: %w", err)
	}
	return updated, nil
}
