package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.DeadlineRepository { return NewRepository() })
}
