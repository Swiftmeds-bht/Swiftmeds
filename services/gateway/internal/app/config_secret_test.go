package app

import (
	"strings"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
)

func TestLoadConfigWorkerSecret(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{"not set means the endpoint rejects every call", "", "", false},
		{"31 bytes is too short", strings.Repeat("a", MinWorkerSecretLength-1), "", true},
		{"32 bytes is the shortest allowed", strings.Repeat("a", MinWorkerSecretLength), strings.Repeat("a", MinWorkerSecretLength), false},
		{"surrounding spaces are trimmed", "  " + strings.Repeat("b", 40) + "  ", strings.Repeat("b", 40), false},
		{"placeholder is rejected", "replace-with-at-least-32-random-bytes", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfig(config.NewFromMap(map[string]string{"WORKER_SHARED_SECRET": tc.value}))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "WORKER_SHARED_SECRET") {
					t.Fatalf("err = %v, want an error naming WORKER_SHARED_SECRET", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.WorkerSecret != tc.want {
				t.Errorf("WorkerSecret = %q, want %q", cfg.WorkerSecret, tc.want)
			}
		})
	}
}
