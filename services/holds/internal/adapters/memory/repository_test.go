package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.HoldRepository { return NewRepository() })
}
