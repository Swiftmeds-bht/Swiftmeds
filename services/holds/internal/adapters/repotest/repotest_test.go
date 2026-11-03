package repotest_test

import (
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// The contract tests are run here too, so that this package's own test run
// executes them (and `go test -cover` counts them). Each adapter also runs
// them from its own package.
func TestContractAgainstTheMemoryRepository(t *testing.T) {
	repotest.Run(t, func(*testing.T) ports.HoldRepository { return memory.NewRepository() })
}
