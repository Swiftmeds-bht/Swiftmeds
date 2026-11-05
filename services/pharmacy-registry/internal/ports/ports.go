package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
)

type PharmacyRepository interface {
	Create(context.Context, domain.Pharmacy) error
	Get(context.Context, string) (domain.Pharmacy, error)
	List(context.Context, bool) ([]domain.Pharmacy, error)
	SetStatus(context.Context, string, domain.Status, string, time.Time) (domain.Pharmacy, error)
}
