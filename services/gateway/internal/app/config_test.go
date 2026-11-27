package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(config.NewFromMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		HTTPPort:       DefaultHTTPPort,
		CORSOrigins:    defaultCORSOrigins,
		TraefikTarget:  DefaultTraefikTarget,
		LogLevel:       "info",
		AuthRateLimit:  DefaultAuthRateLimit,
		AuthRateWindow: DefaultAuthRateWindow,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, want)
	}
	if got := cfg.HTTPAddress(); got != ":8080" {
		t.Errorf("HTTPAddress() = %q, want :8080", got)
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	cfg, err := loadConfig(config.NewFromMap(map[string]string{
		"GATEWAY_HTTP_PORT":    "9000",
		"CORS_ALLOWED_ORIGINS": " https://app.swiftmed.bt , ,https://dash.swiftmed.bt ",
		"TRAEFIK_GRPC_TARGET":  " traefik:6000 ",
		"LOG_LEVEL":            "debug",
		"AUTH_RATE_LIMIT":      "5",
		"AUTH_RATE_WINDOW":     "30s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		HTTPPort:       9000,
		CORSOrigins:    []string{"https://app.swiftmed.bt", "https://dash.swiftmed.bt"},
		TraefikTarget:  "traefik:6000",
		LogLevel:       "debug",
		AuthRateLimit:  5,
		AuthRateWindow: 30 * time.Second,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, want)
	}
}

// Port boundaries: 0 and 65536 are invalid, 1 and 65535 are the valid edges.
func TestLoadConfigPortBoundaries(t *testing.T) {
	cases := []struct {
		port    string
		wantErr bool
	}{
		{"0", true},
		{"1", false},
		{"65535", false},
		{"65536", true},
		{"-1", true},
		{"http", true},
	}
	for _, tc := range cases {
		_, err := loadConfig(config.NewFromMap(map[string]string{"GATEWAY_HTTP_PORT": tc.port}))
		if (err != nil) != tc.wantErr {
			t.Errorf("port %q: err = %v, wantErr %v", tc.port, err, tc.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "GATEWAY_HTTP_PORT") {
			t.Errorf("port %q: error does not name the variable: %v", tc.port, err)
		}
	}
}

func TestLoadConfigDefaultOriginsAreACopy(t *testing.T) {
	cfg, err := loadConfig(config.NewFromMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	cfg.CORSOrigins[0] = "changed"
	if defaultCORSOrigins[0] == "changed" {
		t.Fatal("modifying a loaded config changed the package defaults")
	}
}
