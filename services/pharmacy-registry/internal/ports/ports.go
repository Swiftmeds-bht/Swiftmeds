package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
)

type PharmacyRepository interface {
	Create(context.Context, domain.Pharmacy) error
	Get(context.Context, string) (domain.Pharmacy, error)
	// FindByOwner returns the owner's pharmacy (the oldest, if several), or
	// domain.ErrNotFound.
	FindByOwner(context.Context, string) (domain.Pharmacy, error)
	List(context.Context, bool) ([]domain.Pharmacy, error)
	SetStatus(context.Context, string, domain.Status, string, time.Time) (domain.Pharmacy, error)
	Update(context.Context, string, string, domain.Pharmacy) (domain.Pharmacy, error)
}
