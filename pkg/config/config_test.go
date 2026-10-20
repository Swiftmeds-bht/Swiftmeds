package config

import (
	"strings"
	"testing"
	"time"
)

func TestStringUsesValueOrDefault(t *testing.T) {
	l := NewFromMap(map[string]string{"SET": "value", "EMPTY": ""})

	if got := l.String("SET", "def"); got != "value" {
		t.Errorf("String(SET) = %q, want %q", got, "value")
	}
	if got := l.String("MISSING", "def"); got != "def" {
		t.Errorf("String(MISSING) = %q, want %q", got, "def")
	}
	if got := l.String("EMPTY", "def"); got != "def" {
		t.Errorf("String(EMPTY) = %q, want default for empty value", got)
	}
	if err := l.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
}

func TestRequiredRecordsMissing(t *testing.T) {
	l := NewFromMap(map[string]string{"DB_URL": "postgres://x"})

	if got := l.Required("DB_URL"); got != "postgres://x" {
		t.Errorf("Required(DB_URL) = %q", got)
	}
	l.Required("JWT_SECRET")

	err := l.Err()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Err() = %v, want error naming JWT_SECRET", err)
	}
}

func TestParsersReturnValuesAndDefaults(t *testing.T) {
	l := NewFromMap(map[string]string{
		"PORT":    "50063",
		"DEBUG":   "true",
		"TIMEOUT": "15m",
	})

	if got := l.Int("PORT", 1); got != 50063 {
		t.Errorf("Int = %d, want 50063", got)
	}
	if got := l.Int("NO_PORT", 1); got != 1 {
		t.Errorf("Int default = %d, want 1", got)
	}
	if got := l.Bool("DEBUG", false); !got {
		t.Errorf("Bool = false, want true")
	}
	if got := l.Bool("NO_DEBUG", true); !got {
		t.Errorf("Bool default = false, want true")
	}
	if got := l.Duration("TIMEOUT", time.Second); got != 15*time.Minute {
		t.Errorf("Duration = %v, want 15m", got)
	}
	if got := l.Duration("NO_TIMEOUT", time.Second); got != time.Second {
		t.Errorf("Duration default = %v, want 1s", got)
	}
	if err := l.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
}

func TestInvalidValuesAreAllReported(t *testing.T) {
	l := NewFromMap(map[string]string{
		"PORT":    "abc",
		"DEBUG":   "maybe",
		"TIMEOUT": "soon",
	})

	if got := l.Int("PORT", 7); got != 7 {
		t.Errorf("Int on invalid = %d, want default 7", got)
	}
	l.Bool("DEBUG", false)
	l.Duration("TIMEOUT", time.Second)
	l.Required("MISSING")

	err := l.Err()
	if err == nil {
		t.Fatal("Err() = nil, want errors")
	}
	for _, key := range []string{"PORT", "DEBUG", "TIMEOUT", "MISSING"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("Err() does not mention %s: %v", key, err)
		}
	}
}
