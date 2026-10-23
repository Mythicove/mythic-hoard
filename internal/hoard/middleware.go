package hoard

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Mythicove/shared/ratelimit"
)

// clientIP 提取客户端 IP（去掉端口），作为限流键。
// 反代场景 clientip.Middleware（可信代理感知）已把 RemoteAddr 还原为真实客户端 IP。
func clientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

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

// RateLimit 返回基于 IP 的固定窗口限流中间件（实现见 shared/ratelimit）。
// name 隔离不同挂载点的计数，避免共享 Redis 时相互干扰。
func RateLimit(l ratelimit.Limiter, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, err := l.Allow(r.Context(), name+":"+clientIP(r), limit, window)
			if err == nil && !ok {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders 设置基础安全响应头（API 只返回 JSON/文件流）。
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
