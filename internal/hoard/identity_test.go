package hoard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mythicove/mythic-hoard/internal/storage"
	sharedstorage "github.com/Mythicove/shared/storage"
)

func newTestStorage(t *testing.T) sharedstorage.Storage {
	t.Helper()
	files, err := storage.NewLocal(t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestIdentityStoreFlow(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 首次 upsert 插入（含初始 role）
	if err := store.UpsertUser(ctx, User{Sub: "s1", Username: "u1", Email: "e1", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	// 再次 upsert 更新资料但保留现有 role（登录不得覆盖管理员提升）
	if err := store.UpsertUser(ctx, User{Sub: "s1", Username: "u1-new", Email: "e2", Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	u, err := store.UserBySub(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "u1-new" || u.Email != "e2" {
		t.Fatalf("profile not updated: %+v", u)
	}
	if u.Role != "user" {
		t.Fatalf("role overwritten on re-login: %q", u.Role)
	}
	// 显式提权
	if err := store.SetUserRole(ctx, "s1", "admin"); err != nil {
		t.Fatal(err)
	}
	u, _ = store.UserBySub(ctx, "s1")
	if u.Role != "admin" || !u.IsAdmin() {
		t.Fatalf("role not elevated: %+v", u)
	}
	if err := store.SetUserRole(ctx, "nobody", "admin"); err != ErrNotFound {
		t.Fatalf("elevate missing: %v", err)
	}
	if _, err := store.UserBySub(ctx, "nobody"); err != ErrNotFound {
		t.Fatalf("missing user: %v", err)
	}

	// 会话：有效/过期/删除/未知用户
	if err := store.CreateSession(ctx, "sid", "s1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	su, err := store.SessionUser(ctx, "sid")
	if err != nil || su.Sub != "s1" || su.Role != "admin" {
		t.Fatalf("session user: %+v err=%v", su, err)
	}
	if err := store.CreateSession(ctx, "sid-exp", "s1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionUser(ctx, "sid-exp"); err != ErrNotFound {
		t.Fatalf("expired session: %v", err)
	}
	if err := store.DeleteSession(ctx, "sid"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionUser(ctx, "sid"); err != ErrNotFound {
		t.Fatalf("deleted session: %v", err)
	}
	if err := store.CreateSession(ctx, "sid-x", "nobody", time.Now().Add(time.Hour)); err != ErrNotFound {
		t.Fatalf("session for missing user: %v", err)
	}
}

func TestSessionAuthMiddleware(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	files := newTestStorage(t)
	svc := NewService(store, files, "http://localhost:8080")

	if err := store.UpsertUser(ctx, User{Sub: "s2", Username: "u2", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, "sess-ok", "s2", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	var seen *User
	h := svc.SessionAuth()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = UserFromContext(r.Context())
	}))

	// 有效会话 → 注入用户
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "sess-ok"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen == nil || seen.Sub != "s2" {
		t.Fatalf("user not injected: %+v", seen)
	}

	// 无 cookie → 不注入、不拒绝
	seen = nil
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	if seen != nil {
		t.Fatalf("unexpected user: %+v", seen)
	}

	// 无效会话 → 不注入
	seen = nil
	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "bogus"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != nil {
		t.Fatalf("unexpected user for bogus session: %+v", seen)
	}

	// RequireUser 拒绝未登录
	rec := httptest.NewRecorder()
	RequireUser()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("require user: %d", rec.Code)
	}
}
