package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(t *testing.T) ports.TrustRepository { return NewRepository() })
}
