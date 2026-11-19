package memory

import (
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.PharmacyRepository { return NewRepository() })
}
