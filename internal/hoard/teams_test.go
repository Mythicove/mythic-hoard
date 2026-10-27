package hoard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestTeamsStoreFlow(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.UpsertUser(ctx, User{Sub: "owner1", Username: "Owner", Role: "user"})
	_ = store.UpsertUser(ctx, User{Sub: "dev1", Username: "Dev", Role: "user"})

	teamID, err := store.CreateTeam(ctx, "Core Team", "owner1")
	if err != nil {
		t.Fatal(err)
	}
	// 创建者即 owner
	m, err := store.GetTeamMember(ctx, teamID, "owner1")
	if err != nil || m.Role != "owner" {
		t.Fatalf("owner member: %+v err=%v", m, err)
	}
	// 添加/列出/移除成员
	if err := store.AddTeamMember(ctx, teamID, "dev1", "member"); err != nil {
		t.Fatal(err)
	}
	members, err := store.ListTeamMembers(ctx, teamID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %+v err=%v", members, err)
	}
	teams, _ := store.ListUserTeams(ctx, "dev1")
	if len(teams) != 1 || teams[0].Name != "Core Team" || teams[0].Role != "member" {
		t.Fatalf("user teams: %+v", teams)
	}
	if err := store.RemoveTeamMember(ctx, teamID, "dev1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTeamMember(ctx, teamID, "dev1"); err != ErrNotFound {
		t.Fatalf("removed member: %v", err)
	}
	if err := store.RemoveTeamMember(ctx, teamID, "nobody"); err != ErrNotFound {
		t.Fatalf("remove missing: %v", err)
	}
	// 项目归属
	if _, err := store.CreateTeam(ctx, "x", "nobody"); err != ErrNotFound {
		t.Fatalf("team for missing user: %v", err)
	}
}

func TestPATStoreFlow(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.UpsertUser(ctx, User{Sub: "u1", Username: "U1", Role: "user"})

	_, hash := func() (string, string) { tok, h := generatePAT(); return tok, h }()
	exp := time.Now().Add(time.Hour)
	id, err := store.CreatePAT(ctx, "u1", "ci", hash, []string{"UPLOAD"}, &exp)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.ValidatePAT(ctx, hash)
	if err != nil || sess.User.Sub != "u1" || sess.PATID != id || len(sess.Scopes) != 1 {
		t.Fatalf("validate: %+v err=%v", sess, err)
	}
	pats, _ := store.ListPATs(ctx, "u1")
	if len(pats) != 1 || pats[0].Name != "ci" || pats[0].LastUsed == "" {
		t.Fatalf("list: %+v", pats)
	}
	// 过期
	expPast := time.Now().Add(-time.Hour)
	_, hash2 := generatePAT()
	if _, err := store.CreatePAT(ctx, "u1", "old", hash2, nil, &expPast); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidatePAT(ctx, hash2); err != ErrNotFound {
		t.Fatalf("expired pat: %v", err)
	}
	// 撤销
	if err := store.DeletePAT(ctx, "u1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidatePAT(ctx, hash); err != ErrNotFound {
		t.Fatalf("deleted pat: %v", err)
	}
	if err := store.DeletePAT(ctx, "u1", id); err != ErrNotFound {
		t.Fatalf("double delete: %v", err)
	}
	if err := store.DeletePAT(ctx, "other", "x"); err != ErrNotFound {
		t.Fatalf("delete other: %v", err)
	}
}

func newAuthTestRouter(t *testing.T) (*Service, *MemoryStore, *httptest.Server) {
	t.Helper()
	store := NewMemoryStore()
	svc := NewService(store, newTestStorage(t), "http://localhost:8080")
	r := chi.NewRouter()
	r.Use(svc.SessionAuth())
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	svc.Register(r, "test-admin-token")
	return svc, store, httptest.NewServer(r)
}

func TestTeamsAndPATHTTP(t *testing.T) {
	ctx := context.Background()
	_, store, srv := newAuthTestRouter(t)
	defer srv.Close()
	_ = store.UpsertUser(ctx, User{Sub: "http-u1", Username: "HttpU1", Role: "user"})
	sid := "http-sess-1"
	if err := store.CreateSession(ctx, sid, "http-u1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: SessionCookie, Value: sid}

	// 经真实 server 走一遍
	do := func(method, path, body string, withCookie bool, bearer string) *http.Response {
		var rdr *strings.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		} else {
			rdr = strings.NewReader("")
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		if withCookie {
			req.AddCookie(cookie)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := do(http.MethodPost, "/api/team", `{"name":"T"}`, false, "")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth create team: %d", res.StatusCode)
	}

	res = do(http.MethodPost, "/api/team", `{"name":"Core"}`, true, "")
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("create team: %d %+v", res.StatusCode, created)
	}

	res = do(http.MethodGet, "/api/user/teams", "", true, "")
	var teams []UserTeam
	_ = json.NewDecoder(res.Body).Decode(&teams)
	res.Body.Close()
	if len(teams) != 1 || teams[0].Role != "owner" {
		t.Fatalf("my teams: %+v", teams)
	}

	// 成员列表公开（无凭证也可读）
	res = do(http.MethodGet, "/api/team/"+created.ID+"/members", "", false, "")
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("public members: %d", res.StatusCode)
	}

	// 创建 PAT → 明文仅一次
	res = do(http.MethodPost, "/api/user/pats", `{"name":"ci","scopes":["UPLOAD"],"expires_in_days":30}`, true, "")
	var pat struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	_ = json.NewDecoder(res.Body).Decode(&pat)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || !strings.HasPrefix(pat.Token, PATPrefix) {
		t.Fatalf("create pat: %d %+v", res.StatusCode, pat)
	}
	// PAT 可作 Bearer 认证
	res = do(http.MethodGet, "/auth/me", "", false, pat.Token)
	var me User
	_ = json.NewDecoder(res.Body).Decode(&me)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || me.Sub != "http-u1" {
		t.Fatalf("pat me: %d %+v", res.StatusCode, me)
	}
	// 列出 PAT 不含明文
	res = do(http.MethodGet, "/api/user/pats", "", true, "")
	body, _ := readAll(res)
	if strings.Contains(body, pat.Token) {
		t.Fatal("plaintext token leaked in list")
	}
	// 撤销后 PAT 失效
	res = do(http.MethodDelete, "/api/user/pats/"+pat.ID, "", true, "")
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete pat: %d", res.StatusCode)
	}
	res = do(http.MethodGet, "/auth/me", "", false, pat.Token)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked pat still works: %d", res.StatusCode)
	}
	// 非成员管理他人团队 → 403
	_ = store.UpsertUser(ctx, User{Sub: "http-u2", Username: "HttpU2", Role: "user"})
	sid2 := "http-sess-2"
	_ = store.CreateSession(ctx, sid2, "http-u2", time.Now().Add(time.Hour))
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/team/"+created.ID+"/members", strings.NewReader(`{"sub":"http-u2"}`))
	req2.AddCookie(&http.Cookie{Name: SessionCookie, Value: sid2})
	res2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner add member: %d", res2.StatusCode)
	}
	// 直接加人已被邀请流取代：非成员走 members 端点 → 409
	res = do(http.MethodPost, "/api/team/"+created.ID+"/members", `{"sub":"http-u2","role":"member"}`, true, "")
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("direct add should require an invite: %d", res.StatusCode)
	}
	// owner 邀请 → 204
	res = do(http.MethodPost, "/api/team/"+created.ID+"/invites", `{"sub":"http-u2","role":"member"}`, true, "")
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("owner invite: %d", res.StatusCode)
	}
	// 受邀人看到邀请并接受
	asU2 := func(method, path string) *http.Response {
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sid2})
		out, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	res3 := asU2(http.MethodGet, "/api/user/invites")
	var invites []UserInvite
	_ = json.NewDecoder(res3.Body).Decode(&invites)
	res3.Body.Close()
	if len(invites) != 1 || invites[0].TeamID != created.ID || invites[0].Role != "member" {
		t.Fatalf("my invites: %+v", invites)
	}
	res4 := asU2(http.MethodPost, "/api/user/invites/"+created.ID+"/accept")
	res4.Body.Close()
	if res4.StatusCode != http.StatusNoContent {
		t.Fatalf("accept invite: %d", res4.StatusCode)
	}
	// 接受后邀请被消费，成员列表含 http-u2
	res5 := asU2(http.MethodGet, "/api/user/invites")
	var after []UserInvite
	_ = json.NewDecoder(res5.Body).Decode(&after)
	res5.Body.Close()
	if len(after) != 0 {
		t.Fatalf("invite not consumed: %+v", after)
	}
	res = do(http.MethodGet, "/api/team/"+created.ID+"/members", "", true, "")
	var members []TeamMember
	_ = json.NewDecoder(res.Body).Decode(&members)
	res.Body.Close()
	found := false
	for _, m := range members {
		if m.UserSub == "http-u2" && m.Role == "member" {
			found = true
		}
	}
	if !found {
		t.Fatalf("accepted member missing: %+v", members)
	}
}

func readAll(res *http.Response) (string, error) {
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

// 末位 owner 保护：唯一所有者既不能退出也不能被降级，否则团队无人可管理。
func TestTeamLastOwnerGuard(t *testing.T) {
	ctx := context.Background()
	_, store, srv := newAuthTestRouter(t)
	defer srv.Close()
	_ = store.UpsertUser(ctx, User{Sub: "lo-owner", Username: "LoOwner", Role: "user"})
	_ = store.UpsertUser(ctx, User{Sub: "lo-dev", Username: "LoDev", Role: "user"})
	if err := store.CreateSession(ctx, "lo-sess", "lo-owner", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	do := func(method, path, body string) *http.Response {
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "lo-sess"})
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := do(http.MethodPost, "/api/team", `{"name":"Solo"}`)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("create team: %d %+v", res.StatusCode, created)
	}
	members := "/api/team/" + created.ID + "/members"

	// 唯一 owner 自己退出 → 409
	res = do(http.MethodDelete, members+"/lo-owner", "")
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("last owner leave: %d", res.StatusCode)
	}
	// 唯一 owner 被降级为 member → 409
	res = do(http.MethodPost, members, `{"sub":"lo-owner","role":"member"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("last owner demote: %d", res.StatusCode)
	}
	// 移交所有权：邀请 lo-dev 为 owner，由其接受后原 owner 才可退出
	res = do(http.MethodPost, "/api/team/"+created.ID+"/invites", `{"sub":"lo-dev","role":"owner"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("invite co-owner: %d", res.StatusCode)
	}
	if err := store.CreateSession(ctx, "lo-sess-dev", "lo-dev", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	reqAccept, err := http.NewRequest(http.MethodPost, srv.URL+"/api/user/invites/"+created.ID+"/accept", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	reqAccept.AddCookie(&http.Cookie{Name: SessionCookie, Value: "lo-sess-dev"})
	resAccept, err := srv.Client().Do(reqAccept)
	if err != nil {
		t.Fatal(err)
	}
	resAccept.Body.Close()
	if resAccept.StatusCode != http.StatusNoContent {
		t.Fatalf("accept co-owner invite: %d", resAccept.StatusCode)
	}
	res = do(http.MethodDelete, members+"/lo-owner", "")
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("leave after transfer: %d", res.StatusCode)
	}
}

// 邀请过期：过期的邀请在所有读取路径上视同不存在；重新邀请会刷新有效期。
func TestTeamInviteExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.UpsertUser(ctx, User{Sub: "exp-own", Username: "ExpOwn", Role: "user"})
	_ = store.UpsertUser(ctx, User{Sub: "exp-inv", Username: "ExpInv", Role: "user"})
	teamID, err := store.CreateTeam(ctx, "Expiry", "exp-own")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.UpsertTeamInvite(ctx, teamID, "exp-inv", "member", "exp-own", time.Now().UTC().Add(InviteTTL)); err != nil {
		t.Fatal(err)
	}
	inv, err := store.GetTeamInvite(ctx, teamID, "exp-inv")
	if err != nil || inv.ExpiresAt == "" {
		t.Fatalf("get invite: %+v err=%v", inv, err)
	}
	if mine, err := store.ListInvitesForUser(ctx, "exp-inv"); err != nil || len(mine) != 1 || mine[0].ExpiresAt == "" {
		t.Fatalf("my invites: %+v err=%v", mine, err)
	}
	if got, err := store.ListTeamInvites(ctx, teamID); err != nil || len(got) != 1 {
		t.Fatalf("team invites: %+v err=%v", got, err)
	}

	// 把过期时间挪到过去（等价于时间流逝）
	store.mu.Lock()
	store.teams[teamID].invites["exp-inv"].expires = time.Now().UTC().Add(-time.Minute)
	store.mu.Unlock()

	if _, err := store.GetTeamInvite(ctx, teamID, "exp-inv"); err != ErrNotFound {
		t.Fatalf("expired invite should read as not found: %v", err)
	}
	if got, _ := store.ListInvitesForUser(ctx, "exp-inv"); len(got) != 0 {
		t.Fatalf("expired invite still listed for user: %+v", got)
	}
	if got, _ := store.ListTeamInvites(ctx, teamID); len(got) != 0 {
		t.Fatalf("expired invite still listed for team: %+v", got)
	}

	// 重新邀请刷新有效期 → 又可用
	if err := store.UpsertTeamInvite(ctx, teamID, "exp-inv", "member", "exp-own", time.Now().UTC().Add(InviteTTL)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetTeamInvite(ctx, teamID, "exp-inv"); err != nil {
		t.Fatalf("re-invite should be visible again: %v", err)
	}
}
