package app

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
)

// authRateLimiter applies fixed-window limits independently to login IPs,
// login emails, and registration IPs.
type authRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]rateBucket
}

type rateBucket struct {
	count int
	reset time.Time
}

func newAuthRateLimiter(limit int, window time.Duration) *authRateLimiter {
	return &authRateLimiter{limit: limit, window: window, buckets: make(map[string]rateBucket)}
}

func (l *authRateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if !now.Before(b.reset) {
			delete(l.buckets, k)
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		b = rateBucket{reset: now.Add(l.window)}
	}
	if b.count >= l.limit {
		return false, b.reset.Sub(now)
	}
	b.count++
	l.buckets[key] = b
	return true, 0
}

func (l *authRateLimiter) check(w http.ResponseWriter, r *http.Request, email string) bool {
	path := r.URL.Path
	ip := clientIP(r)
	keys := []string{"auth:ip:" + path + ":" + ip}
	if path == "/api/v1/auth/login" && email != "" {
		keys = append(keys, "auth:email:"+strings.ToLower(strings.TrimSpace(email)))
	}
	for _, key := range keys {
		if ok, retry := l.allow(key, time.Now()); !ok {
			seconds := int((retry + time.Second - 1) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many authentication attempts; try again later"})
			return false
		}
	}
	return true
}

// Traefik appends its observed client address to X-Forwarded-For. Using the
// last valid address avoids trusting a client-supplied prefix in that header.
func clientIP(r *http.Request) string {
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(forwarded) - 1; i >= 0; i-- {
		if ip := net.ParseIP(strings.TrimSpace(forwarded[i])); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return "unknown"
}
