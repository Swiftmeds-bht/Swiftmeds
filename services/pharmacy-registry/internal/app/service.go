package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
)

var ErrNotFound = domain.ErrNotFound

type RegisterInput struct {
	OwnerID       string
	Name          string
	LicenseNumber string
	Location      domain.GeoPoint
	Hours         []domain.OpeningHours
}

type Service struct {
	repo  ports.PharmacyRepository
	now   func() time.Time
	newID func() string
}

func NewService(repo ports.PharmacyRepository, now func() time.Time, newID func() string) *Service {
	return &Service{repo: repo, now: now, newID: newID}
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.Pharmacy, error) {
	p, err := domain.NewPharmacy(s.newID(), in.OwnerID, in.Name, in.LicenseNumber, in.Location, in.Hours, s.now())
	if err != nil {
		return domain.Pharmacy{}, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("create pharmacy: %w", err)
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Pharmacy, error) {
	if id == "" {
		return domain.Pharmacy{}, domain.ErrMissingPharmacyID
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("get pharmacy: %w", err)
	}
	return p, nil
}

func (s *Service) List(ctx context.Context, verifiedOnly bool) ([]domain.Pharmacy, error) {
	pharmacies, err := s.repo.List(ctx, verifiedOnly)
	if err != nil {
		return nil, fmt.Errorf("list pharmacies: %w", err)
	}
	return pharmacies, nil
}

func (s *Service) SetVerificationStatus(ctx context.Context, id string, status domain.Status, reason string) (domain.Pharmacy, error) {
	if id == "" {
		return domain.Pharmacy{}, domain.ErrMissingPharmacyID
	}
	p, err := s.repo.SetStatus(ctx, id, status, reason, s.now())
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("set verification status: %w", err)
	}
	return p, nil
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("generate pharmacy id: %v", err))
	}
	return hex.EncodeToString(b[:])
}
