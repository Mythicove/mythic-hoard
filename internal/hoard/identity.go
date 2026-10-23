package hoard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Mythicove/mythic-hoard/internal/oidcrp"
)

// 会话与 OIDC 中转 cookie 名。
const (
	SessionCookie = "hoard_session"
	stateCookie   = "hoard_oidc_state"
	nonceCookie   = "hoard_oidc_nonce"
)

// 会话有效期（7 天）与 OIDC 中转 state/nonce 有效期（10 分钟）。
const (
	sessionTTL = 7 * 24 * time.Hour
	flowTTL    = 10 * time.Minute
)

// SetOIDC 注入 OIDC RP 与管理员 subject 白名单（白名单内用户登录即映射为 admin，幂等）。
func (s *Service) SetOIDC(rp *oidcrp.RP, adminSubjects []string) {
	s.oidc = rp
	s.adminSubjects = adminSubjects
}

// userKey 是会话用户的 context 键。
type userKey struct{}

// UserFromContext 返回请求的登录用户（未登录返回 nil）。
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userKey{}).(*User)
	return u
}

// SessionAuth 读取会话 cookie 并把用户注入 context（不强制；由端点决定是否要求登录）。
func (s *Service) SessionAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
				if u, err := s.store.SessionUser(r.Context(), c.Value); err == nil && u != nil {
					r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser 拒绝未登录请求。
func RequireUser() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if UserFromContext(r.Context()) == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// HandleOIDCLogin 生成 state/nonce（短期 httpOnly cookie 保管）并跳转授权端点。
func (s *Service) HandleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, "oidc not configured", http.StatusNotFound)
		return
	}
	state, nonce := randomHex(16), randomHex(16)
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	for _, c := range []*http.Cookie{
		{Name: stateCookie, Value: state, Path: "/auth/oidc", MaxAge: int(flowTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure},
		{Name: nonceCookie, Value: nonce, Path: "/auth/oidc", MaxAge: int(flowTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure},
	} {
		http.SetCookie(w, c)
	}
	http.Redirect(w, r, s.oidc.AuthCodeURL(state, nonce), http.StatusFound)
}

// HandleOIDCCallback 校验 state → 换 token → upsert 投影（白名单映射 admin）→ 建会话。
func (s *Service) HandleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.Error(w, "oidc not configured", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		http.Error(w, "authorization denied: "+q.Get("error"), http.StatusForbidden)
		return
	}
	stateC, err1 := r.Cookie(stateCookie)
	nonceC, err2 := r.Cookie(nonceCookie)
	if err1 != nil || err2 != nil || q.Get("state") == "" || q.Get("state") != stateC.Value {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	claims, err := s.oidc.Exchange(r.Context(), q.Get("code"), nonceC.Value)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	// 中转 cookie 用完即清
	for _, name := range []string{stateCookie, nonceCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/auth/oidc", MaxAge: -1})
	}

	ctx := r.Context()
	role := "user"
	for _, sub := range s.adminSubjects {
		if sub == claims.Sub {
			role = "admin"
		}
	}
	if err := s.store.UpsertUser(ctx, User{
		Sub: claims.Sub, Username: claims.DisplayName(), Email: claims.Email, Role: role,
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if role == "admin" {
		_ = s.store.SetUserRole(ctx, claims.Sub, "admin") // 白名单幂等提升
	}

	sid := randomHex(32)
	if err := s.store.CreateSession(ctx, sid, claims.Sub, time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: sid, Path: "/",
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout 删除服务端会话并清 cookie。
func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		_ = s.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// HandleMe 返回当前登录用户（未登录 401）。
func (s *Service) HandleMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(u)
}
