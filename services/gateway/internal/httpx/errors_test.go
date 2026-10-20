package httpx

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusCode(t *testing.T) {
	tests := []struct {
		code codes.Code
		want int
	}{
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.NotFound, http.StatusNotFound},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
	}
	for _, test := range tests {
		if got := StatusCode(status.Error(test.code, "test")); got != test.want {
			t.Errorf("StatusCode(%s) = %d, want %d", test.code, got, test.want)
		}
	}
}
