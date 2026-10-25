package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.LocationRepository { return NewRepository() })
}
