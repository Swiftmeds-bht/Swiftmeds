package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.PriceRepository { return NewRepository() })
}
