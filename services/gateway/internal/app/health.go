package app

import (
	"context"
	"fmt"
	"net/http"
)

// CheckHealth calls GET url and returns nil only for a 200 response. The
// gateway's "healthcheck" sub-command uses it for the Docker HEALTHCHECK,
// because the distroless image has no shell or curl.
func CheckHealth(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check %s: status %d", url, resp.StatusCode)
	}
	return nil
}
