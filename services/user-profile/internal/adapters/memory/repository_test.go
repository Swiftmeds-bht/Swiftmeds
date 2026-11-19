package memory

import (
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.ProfileRepository { return NewRepository() })
}
