package app

import (
	"testing"
	"time"
)

// The defaults are product rules (README "Rules"): a hold lasts 2 hours and a
// patient may have 3 open holds. Gremlins cannot count a constant as covered,
// so this test is what fails if one of them changes.
func TestDefaultsMatchTheRules(t *testing.T) {
	if DefaultHoldDuration != 2*time.Hour {
		t.Errorf("DefaultHoldDuration = %v, want 2h", DefaultHoldDuration)
	}
	if DefaultMaxActivePerUser != 3 {
		t.Errorf("DefaultMaxActivePerUser = %d, want 3", DefaultMaxActivePerUser)
	}
	if DefaultPageSize != 20 || MaxPageSize != 100 {
		t.Errorf("page sizes = %d default, %d max; want 20 and 100", DefaultPageSize, MaxPageSize)
	}
}
