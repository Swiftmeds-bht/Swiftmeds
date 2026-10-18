package app

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort      int
	CORSOrigins   []string
	TraefikTarget string
}

func LoadConfig() (Config, error) {
	port := 8080
	if value := os.Getenv("GATEWAY_HTTP_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("GATEWAY_HTTP_PORT must be a valid TCP port: %q", value)
		}
		port = parsed
	}

	origins := splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000", "http://localhost:3001"}
	}

	return Config{
		HTTPPort:      port,
		CORSOrigins:   origins,
		TraefikTarget: envOrDefault("TRAEFIK_GRPC_TARGET", "traefik:50051"),
	}, nil
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

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
