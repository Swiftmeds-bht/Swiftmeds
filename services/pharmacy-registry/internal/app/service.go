package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
)

var ErrNotFound = domain.ErrNotFound

type RegisterInput struct {
	OwnerID       string
	Name          string
	LicenseNumber string
	Phone         string
	Address       string
	Dzongkhag     string
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

// Register creates a pending pharmacy. It is idempotent per owner (SCRUM-141):
// if the owner already has a pharmacy, that one is returned unchanged, so a
// client that lost the answer can safely send the same request again.
func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.Pharmacy, error) {
	p, err := domain.NewPharmacy(s.newID(), in.OwnerID, in.Name, in.LicenseNumber, in.Dzongkhag, in.Location, in.Hours, s.now())
	if err != nil {
		return domain.Pharmacy{}, err
	}
	p.Phone, p.Address = strings.TrimSpace(in.Phone), strings.TrimSpace(in.Address)
	existing, err := s.repo.FindByOwner(ctx, in.OwnerID)
	switch {
	case err == nil:
		return existing, nil
	case !errors.Is(err, domain.ErrNotFound):
		return domain.Pharmacy{}, fmt.Errorf("find owner's pharmacy: %w", err)
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
