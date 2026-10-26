package hoard

import (
	"context"
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

func TestAdminRoleAcceptsTokenAndAdminSession(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	cases := []struct {
		name   string
		config string
		user   *User
		bearer string
		want   int
	}{
		{"static token", "sekret", nil, "sekret", http.StatusOK},
		{"admin session", "sekret", &User{Sub: "u1", Role: "admin"}, "", http.StatusOK},
		{"plain user session", "sekret", &User{Sub: "u2", Role: "user"}, "", http.StatusForbidden},
		{"wrong token", "sekret", nil, "nope", http.StatusForbidden},
		{"no credential", "sekret", nil, "", http.StatusForbidden},
		// fail closed：忘配令牌不等于开放管理端点
		{"empty token config", "", nil, "", http.StatusForbidden},
		{"empty token config, admin session", "", &User{Sub: "u1", Role: "admin"}, "", http.StatusOK},
	}
	for _, tc := range cases {
		h := AdminRole(tc.config)(ok)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		if tc.user != nil {
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, tc.user))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, w.Code, tc.want)
		}
	}
}

// 管理端点在真实路由上：站点管理员用会话即可（不必手贴令牌），普通用户 403。
func TestAdminEndpointsAcceptAdminSession(t *testing.T) {
	ctx := context.Background()
	_, store, srv := newAuthTestRouter(t) // 注册时 adminToken = "test-admin-token"
	defer srv.Close()
	_ = store.UpsertUser(ctx, User{Sub: "root", Username: "Root", Role: "admin"})
	_ = store.UpsertUser(ctx, User{Sub: "joe", Username: "Joe", Role: "user"})
	_ = store.CreateSession(ctx, "sess-root", "root", time.Now().Add(time.Hour))
	_ = store.CreateSession(ctx, "sess-joe", "joe", time.Now().Add(time.Hour))

	call := func(cookie, bearer string) int {
		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/mods/does-not-exist", nil)
		if err != nil {
			t.Fatal(err)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	// 管理员会话 → 通过鉴权（项目不存在故 404，而不是 403）
	if code := call("sess-root", ""); code != http.StatusNotFound {
		t.Fatalf("admin session: %d", code)
	}
	// 静态令牌仍可用（救援通道）
	if code := call("", "test-admin-token"); code != http.StatusNotFound {
		t.Fatalf("static token: %d", code)
	}
	// 普通用户会话 → 403
	if code := call("sess-joe", ""); code != http.StatusForbidden {
		t.Fatalf("plain user: %d", code)
	}
	// 无凭证 → 403
	if code := call("", ""); code != http.StatusForbidden {
		t.Fatalf("anonymous: %d", code)
	}
}
