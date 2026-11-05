package domain

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestPickupAlphabetHas32EasyToReadCharacters(t *testing.T) {
	if len(pickupAlphabet) != 32 {
		t.Fatalf("alphabet has %d characters, want 32 (the byte mask relies on it)", len(pickupAlphabet))
	}
	seen := map[rune]bool{}
	for _, r := range pickupAlphabet {
		if strings.ContainsRune("01IO", r) {
			t.Errorf("alphabet contains %q, which is easy to mix up", r)
		}
		if seen[r] {
			t.Errorf("alphabet contains %q twice", r)
		}
		seen[r] = true
	}
}

func TestNewPickupCodeMapsEachByteToACharacter(t *testing.T) {
	// 0 -> '2', 1 -> '3', 2 -> '4', 31 -> 'Z', 32 wraps to 0 -> '2', 255 -> 31 -> 'Z'.
	random := bytes.NewReader([]byte{0, 1, 2, 31, 32, 255})

	got, err := NewPickupCode(random)

	if err != nil {
		t.Fatal(err)
	}
	if got != "234Z2Z" {
		t.Errorf("code = %q, want %q", got, "234Z2Z")
	}
}

func TestNewPickupCodeFailsWhenRandomBytesRunOut(t *testing.T) {
	got, err := NewPickupCode(bytes.NewReader([]byte{1, 2}))

	if err == nil || got != "" {
		t.Fatalf("got (%q, %v), want an error and no code", got, err)
	}
}

func TestNewPickupCodeFromTheSystemRandomSource(t *testing.T) {
	codes := map[string]bool{}
	for range 20 {
		code, err := NewPickupCode(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != PickupCodeLength {
			t.Fatalf("code %q has %d characters, want %d", code, len(code), PickupCodeLength)
		}
		for _, r := range code {
			if !strings.ContainsRune(pickupAlphabet, r) {
				t.Fatalf("code %q has %q, which is not in the alphabet", code, r)
			}
		}
		codes[code] = true
	}
	if len(codes) < 2 {
		t.Error("20 codes were all the same, so they are not random")
	}
}

func TestNormalizePickupCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ABC234", "ABC234"},
		{"abc234", "ABC234"},
		{" abc-234 ", "ABC234"},
		{"a b\tc", "ABC"},
		{"", ""},
		{" - ", ""},
	}
	for _, c := range cases {
		if got := NormalizePickupCode(c.in); got != c.want {
			t.Errorf("NormalizePickupCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPickupCodeMatches(t *testing.T) {
	cases := []struct {
		name          string
		stored, given string
		want          bool
	}{
		{"same code", "ABC234", "ABC234", true},
		{"lower case", "ABC234", "abc234", true},
		{"with a dash and spaces", "ABC234", " abc-234 ", true},
		{"one character different", "ABC234", "ABC235", false},
		{"too short", "ABC234", "ABC23", false},
		{"too long", "ABC234", "ABC2345", false},
		{"empty given code", "ABC234", "", false},
		{"empty stored code never matches", "", "", false},
		{"empty stored code and some given code", "", "ABC234", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PickupCodeMatches(c.stored, c.given); got != c.want {
				t.Errorf("PickupCodeMatches(%q, %q) = %v, want %v", c.stored, c.given, got, c.want)
			}
		})
	}
}
