package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(t *testing.T) ports.SubscriptionRepository { return NewRepository() })
}

func TestAlertsAreKept(t *testing.T) {
	repo := NewRepository()
	if got := repo.Alerts(); len(got) != 0 {
		t.Fatalf("new repository has %d alerts", len(got))
	}
}
