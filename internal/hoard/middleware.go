package hoard

import (
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// Admin 校验管理员 token（空 token 表示禁用鉴权）。
func Admin(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminToken != "" && bearerToken(r) != adminToken {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	return strings.TrimPrefix(h, "Bearer ")
}

// RateLimit 返回基于 IP 的令牌桶限流中间件。
func RateLimit(limit rate.Limit, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := map[string]*rate.Limiter{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			mu.Lock()
			l, ok := limiters[ip]
			if !ok {
				l = rate.NewLimiter(limit, burst)
				limiters[ip] = l
			}
			mu.Unlock()
			if !l.Allow() {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
