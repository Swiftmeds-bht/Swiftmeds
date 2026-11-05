package domain

import (
	"crypto/subtle"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// PickupCodeLength is how many characters a pickup code has.
const PickupCodeLength = 6

// pickupAlphabet has exactly 32 characters: the digits 2-9 and the letters
// A-Z without I and O. 0/O and 1/I are left out because they are easy to
// mix up when a code is read aloud or typed. A random byte masked with 31
// picks one of the 32 characters with no bias.
const pickupAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// NewPickupCode makes a code of PickupCodeLength characters from random bytes.
// Pass crypto/rand.Reader in production and a fixed reader in tests.
func NewPickupCode(random io.Reader) (string, error) {
	var raw [PickupCodeLength]byte
	if _, err := io.ReadFull(random, raw[:]); err != nil {
		return "", fmt.Errorf("generate pickup code: %w", err)
	}
	code := make([]byte, PickupCodeLength)
	for i, b := range raw {
		code[i] = pickupAlphabet[b&31]
	}
	return string(code), nil
}

// NormalizePickupCode removes spaces and dashes and upper-cases the text, so
// "abc 234" and "ABC-234" are read as "ABC234".
func NormalizePickupCode(s string) string {
	dropped := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, s)
	return strings.ToUpper(dropped)
}

// PickupCodeMatches reports whether given is the stored code. The comparison
// takes the same time wherever the codes differ, and an empty code never matches.
func PickupCodeMatches(stored, given string) bool {
	given = NormalizePickupCode(given)
	if stored == "" || given == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(given)) == 1
}
