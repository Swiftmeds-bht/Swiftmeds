package app

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// A version 4 UUID: the version nibble is 4 and the variant bits are 10xx, so
// the first character of the fourth group is 8, 9, a or b.
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Hold IDs are random version 4 UUIDs. The bytes are random, so the test makes
// many of them: a wrong version or variant mask shows up in almost every one.
func TestNewUUIDMakesDistinctVersion4UUIDs(t *testing.T) {
	seen := make(map[string]bool)
	for range 200 {
		id, err := newUUID()
		if err != nil {
			t.Fatalf("newUUID: %v", err)
		}
		if !uuidV4.MatchString(id) {
			t.Fatalf("id %q is not a version 4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("id %q was made twice", id)
		}
		seen[id] = true
	}
}

func TestNewPickupCodeUsesTheReadableAlphabet(t *testing.T) {
	for range 50 {
		code, err := newPickupCode()
		if err != nil {
			t.Fatalf("newPickupCode: %v", err)
		}
		if len(code) != domain.PickupCodeLength {
			t.Fatalf("code %q has %d characters, want %d", code, len(code), domain.PickupCodeLength)
		}
		if strings.ContainsAny(code, "01IO") {
			t.Fatalf("code %q uses a character that is easy to misread", code)
		}
	}
}
