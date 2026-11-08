package memory

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

func TestRepositoryContract(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.FeedbackRepository { return NewRepository() })
}
