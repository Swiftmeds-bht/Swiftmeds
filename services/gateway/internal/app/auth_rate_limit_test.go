package app

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthRateLimiterLimitAndReset(t *testing.T) {
	limiter := newAuthRateLimiter(2, time.Minute)
	start := time.Unix(100, 0)
	for i := 0; i < 2; i++ {
		if ok, _ := limiter.allow("ip:one", start); !ok {
			t.Fatalf("attempt %d was limited", i+1)
		}
	}
	if ok, retry := limiter.allow("ip:one", start.Add(10*time.Second)); ok || retry != 50*time.Second {
		t.Fatalf("limited attempt = (%v, %v), want (false, 50s)", ok, retry)
	}
	if ok, _ := limiter.allow("ip:one", start.Add(time.Minute)); !ok {
		t.Fatal("limit did not reset after its window")
	}
	if ok, _ := limiter.allow("ip:two", start.Add(10*time.Second)); !ok {
		t.Fatal("one client affected another client's limit")
	}
}

func TestAuthRateLimiterWrites429AndRetryAfter(t *testing.T) {
	limiter := newAuthRateLimiter(1, time.Minute)
	limiter.allow("auth:ip:/api/v1/auth/register:192.0.2.1", time.Now())
	req := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	if limiter.check(rec, req, "") {
		t.Fatal("second request was allowed")
	}
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" || rec.Body.String() != "{\"error\":\"too many authentication attempts; try again later\"}\n" {
		t.Fatalf("response = %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestClientIPUsesLastForwardedAddress(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.4:80"
	req.Header.Set("X-Forwarded-For", "203.0.113.77, 198.51.100.8")
	if got := clientIP(req); got != "198.51.100.8" {
		t.Fatalf("clientIP = %q", got)
	}
}
