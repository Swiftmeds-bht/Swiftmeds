package memory

import (
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.UserRepository { return NewRepository() })
}
