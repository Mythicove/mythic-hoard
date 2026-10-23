package hoard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mythicove/shared/ratelimit"
)

func TestClientIPStripsPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:5678"
	if got := clientIP(r); got != "1.2.3.4" {
		t.Fatalf("clientIP = %q", got)
	}
}

func TestRateLimitSameIPDifferentPort(t *testing.T) {
	h := RateLimit(ratelimit.NewMemory(), "test", 2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	status := make([]int, 0, 3)
	for _, port := range []string{"40001", "40002", "40003"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.9:" + port
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		status = append(status, w.Code)
	}
	if status[2] != http.StatusTooManyRequests {
		t.Fatalf("third (same IP) should be 429, got %v", status)
	}
}
