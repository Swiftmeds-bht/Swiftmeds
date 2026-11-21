package app

import (
	"testing"
	"time"
)

// The defaults are product rules (README "Rules"): a hold lasts 2 hours and a
// patient may have 3 open holds. This test keeps the runtime duration defaults
// covered by both statement and mutation testing.
func TestDefaultsMatchTheRules(t *testing.T) {
	if DefaultHoldDuration() != 2*time.Hour {
		t.Errorf("DefaultHoldDuration() = %v, want 2h", DefaultHoldDuration())
	}
	if DefaultPendingTimeout() != 10*time.Minute {
		t.Errorf("DefaultPendingTimeout() = %v, want 10m", DefaultPendingTimeout())
	}
	if DefaultMaxActivePerUser != 3 {
		t.Errorf("DefaultMaxActivePerUser = %d, want 3", DefaultMaxActivePerUser)
	}
	if DefaultPageSize != 20 || MaxPageSize != 100 {
		t.Errorf("page sizes = %d default, %d max; want 20 and 100", DefaultPageSize, MaxPageSize)
	}
}
