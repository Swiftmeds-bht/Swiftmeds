package app

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
)

const (
	DefaultHTTPPort       = 8080
	DefaultTraefikTarget  = "traefik:50051"
	DefaultAuthRateLimit  = 10
	DefaultAuthRateWindow = time.Minute

	// MinWorkerSecretLength is the shortest WORKER_SHARED_SECRET the gateway accepts, in bytes.
	MinWorkerSecretLength = 32
)

var defaultCORSOrigins = []string{"http://localhost:3000"}

type Config struct {
	HTTPPort      int
	CORSOrigins   []string
	TraefikTarget string
	LogLevel      string

	// WorkerSecret is the shared secret the Cloudflare Workers send to the
	// internal endpoints. Empty means those endpoints reject every call.
	WorkerSecret   string
	AuthRateLimit  int
	AuthRateWindow time.Duration
}

// LoadConfig reads the gateway configuration from the environment.
func LoadConfig() (Config, error) {
	return loadConfig(config.New())
}

// loadConfig reads every setting first, then reports all problems at once.
func loadConfig(l *config.Loader) (Config, error) {
	cfg := Config{
		HTTPPort:       l.Int("GATEWAY_HTTP_PORT", DefaultHTTPPort),
		CORSOrigins:    splitCSV(l.String("CORS_ALLOWED_ORIGINS", "")),
		TraefikTarget:  strings.TrimSpace(l.String("TRAEFIK_GRPC_TARGET", DefaultTraefikTarget)),
		LogLevel:       l.String("LOG_LEVEL", "info"),
		WorkerSecret:   strings.TrimSpace(l.String("WORKER_SHARED_SECRET", "")),
		AuthRateLimit:  l.Int("AUTH_RATE_LIMIT", DefaultAuthRateLimit),
		AuthRateWindow: l.Duration("AUTH_RATE_WINDOW", DefaultAuthRateWindow),
	}
	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 {
		return Config{}, fmt.Errorf("GATEWAY_HTTP_PORT must be a valid TCP port (1-65535): %d", cfg.HTTPPort)
	}
	if cfg.AuthRateLimit < 1 {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT must be at least 1: %d", cfg.AuthRateLimit)
	}
	if cfg.AuthRateWindow <= 0 {
		return Config{}, fmt.Errorf("AUTH_RATE_WINDOW must be positive: %s", cfg.AuthRateWindow)
	}
	if cfg.WorkerSecret != "" && len([]byte(cfg.WorkerSecret)) < MinWorkerSecretLength {
		return Config{}, fmt.Errorf("WORKER_SHARED_SECRET must be at least %d characters", MinWorkerSecretLength)
	}
	if cfg.WorkerSecret != "" && isPlaceholderWorkerSecret(cfg.WorkerSecret) {
		return Config{}, fmt.Errorf("WORKER_SHARED_SECRET must not be a placeholder")
	}
	if len(cfg.CORSOrigins) == 0 {
		cfg.CORSOrigins = append([]string(nil), defaultCORSOrigins...)
	}
	return cfg, nil
}

func isPlaceholderWorkerSecret(secret string) bool {
	value := strings.ToLower(secret)
	return strings.Contains(value, "change-me") || strings.Contains(value, "changeme") ||
		strings.Contains(value, "replace-with") || strings.Contains(value, "placeholder") ||
		value == "secret" || value == "password"
}

func (c Config) HTTPAddress() string {
	return net.JoinHostPort("", strconv.Itoa(c.HTTPPort))
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
