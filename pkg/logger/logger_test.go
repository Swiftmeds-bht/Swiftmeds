package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewWritesJSONWithServiceField(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, "pricing", "info")

	log.Info("hello", slog.String("key", "value"))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, buf.String())
	}
	if entry["service"] != "pricing" {
		t.Errorf("service = %v, want pricing", entry["service"])
	}
	if entry["msg"] != "hello" || entry["key"] != "value" {
		t.Errorf("unexpected entry: %v", entry)
	}
}

func TestLevelFiltersLowerEntries(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, "pricing", "warn")

	log.Info("dropped")
	if buf.Len() != 0 {
		t.Errorf("info logged at warn level: %q", buf.String())
	}
	log.Warn("kept")
	if buf.Len() == 0 {
		t.Error("warn not logged at warn level")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		" error ": slog.LevelError,
		"":        slog.LevelInfo,
		"verbose": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
