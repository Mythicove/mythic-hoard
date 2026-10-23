package pgstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Mythicove/mythic-hoard/internal/hoard"
	"github.com/Mythicove/mythic-hoard/internal/migrate"
)

// 需要 HOARD_TEST_DATABASE_URL（指向空库/测试库）才会运行；迁移由测试自动执行。
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("HOARD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HOARD_TEST_DATABASE_URL not set")
	}
	if err := migrate.Run(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return dsn
}

func TestStoreIntegration(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	st, err := New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	proj, err := st.CreateProject(ctx, hoard.NewProject{
		Slug:         "test-mod",
		Title:        "Test Mod",
		Description:  "integration test project",
		ProjectType:  "mod",
		ClientSide:   "required",
		ServerSide:   "optional",
		Status:       "listed",
		Loaders:      []string{"fabric"},
		GameVersions: []string{"1.21"},
		Categories:   []string{"utility"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.DeleteProject(ctx, proj.ID) }()

	if _, err := st.CreateProject(ctx, hoard.NewProject{Slug: "test-mod", Title: "Dup", Status: "listed"}); err != hoard.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	got, err := st.GetProject(ctx, "test-mod")
	if err != nil || got.ID != proj.ID {
		t.Fatalf("get by slug: %+v err=%v", got, err)
	}

	ver, err := st.CreateVersion(ctx, proj.ID, hoard.NewVersion{
		Name:          "1.0.0 for 1.21",
		VersionNumber: "1.0.0",
		VersionType:   "release",
		Changelog:     "initial",
		Loaders:       []string{"fabric"},
		GameVersions:  []string{"1.21"},
	})
	if err != nil {
		t.Fatal(err)
	}

	f, err := st.CreateFile(ctx, ver.ID, hoard.NewFile{
		Filename:   "test-mod-1.0.0.jar",
		Size:       42,
		SHA1:       "aa",
		SHA512:     "bb",
		StorageKey: "files/bb",
		Primary:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	versions, err := st.ListVersions(ctx, proj.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v err=%v", versions, err)
	}
	files, err := st.ListFiles(ctx, ver.ID)
	if err != nil || len(files) != 1 || files[0].ID != f.ID {
		t.Fatalf("files: %v err=%v", files, err)
	}

	hits, total, err := st.Search(ctx, "test mod", hoard.Filter{Status: "listed"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 || len(hits) < 1 {
		t.Fatalf("search hits=%d total=%d", len(hits), total)
	}

	if err := st.IncrementDownloads(ctx, proj.ID); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetProject(ctx, proj.ID)
	if err != nil || got.Downloads != 1 {
		t.Fatalf("downloads: %+v err=%v", got, err)
	}

	if err := st.UpdateProjectStatus(ctx, proj.ID, "archived"); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetProject(ctx, proj.ID)
	if err != nil || got.Status != "archived" {
		t.Fatalf("status: %+v err=%v", got, err)
	}

	loaders, err := st.ListLoaders(ctx)
	if err != nil || len(loaders) == 0 {
		t.Fatalf("loaders: %v err=%v", loaders, err)
	}

	if err := st.DeleteProject(ctx, proj.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProject(ctx, proj.ID); err != hoard.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestIdentityPG(t *testing.T) {
	dsn := os.Getenv("HOARD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HOARD_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := migrate.Run(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sub := "it-sub-" + time.Now().Format("150405.000")
	_, _ = st.pool.Exec(ctx, `DELETE FROM users WHERE sub = $1`, sub)

	// upsert 插入 + 重登保留 role
	if err := st.UpsertUser(ctx, hoard.User{Sub: sub, Username: "u1", Email: "e1", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(ctx, hoard.User{Sub: sub, Username: "u1-new", Email: "e2", Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserBySub(ctx, sub)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "u1-new" || u.Email != "e2" || u.Role != "user" {
		t.Fatalf("upsert: %+v", u)
	}
	// 提权 + 会话
	if err := st.SetUserRole(ctx, sub, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(ctx, "it-sess-"+sub, sub, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	su, err := st.SessionUser(ctx, "it-sess-"+sub)
	if err != nil || su.Sub != sub || su.Role != "admin" {
		t.Fatalf("session: %+v err=%v", su, err)
	}
	// 过期会话不可用
	if err := st.CreateSession(ctx, "it-exp-"+sub, sub, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SessionUser(ctx, "it-exp-"+sub); err != hoard.ErrNotFound {
		t.Fatalf("expired: %v", err)
	}
	// 删除
	if err := st.DeleteSession(ctx, "it-sess-"+sub); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SessionUser(ctx, "it-sess-"+sub); err != hoard.ErrNotFound {
		t.Fatalf("deleted: %v", err)
	}
}

func TestTeamsAndPATPG(t *testing.T) {
	dsn := os.Getenv("HOARD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HOARD_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := migrate.Run(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	stamp := time.Now().Format("150405.000")
	owner, dev := "it-owner-"+stamp, "it-dev-"+stamp
	_, _ = st.pool.Exec(ctx, `DELETE FROM users WHERE sub IN ($1,$2)`, owner, dev)
	if err := st.UpsertUser(ctx, hoard.User{Sub: owner, Username: "Owner", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(ctx, hoard.User{Sub: dev, Username: "Dev", Role: "user"}); err != nil {
		t.Fatal(err)
	}

	teamID, err := st.CreateTeam(ctx, "PG Team", owner)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.GetTeamMember(ctx, teamID, owner)
	if err != nil || m.Role != "owner" || m.Username != "Owner" {
		t.Fatalf("owner: %+v err=%v", m, err)
	}
	if err := st.AddTeamMember(ctx, teamID, dev, "member"); err != nil {
		t.Fatal(err)
	}
	members, err := st.ListTeamMembers(ctx, teamID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %+v err=%v", members, err)
	}
	teams, _ := st.ListUserTeams(ctx, dev)
	if len(teams) != 1 || teams[0].Name != "PG Team" {
		t.Fatalf("user teams: %+v", teams)
	}
	if err := st.RemoveTeamMember(ctx, teamID, dev); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTeamMember(ctx, teamID, dev); err != hoard.ErrNotFound {
		t.Fatalf("removed: %v", err)
	}

	var pid string
	if err := st.pool.QueryRow(ctx, `SELECT id FROM projects LIMIT 1`).Scan(&pid); err == nil {
		if err := st.SetProjectTeam(ctx, pid, teamID); err != nil {
			t.Fatal(err)
		}
		got, err := st.ProjectTeam(ctx, pid)
		if err != nil || got != teamID {
			t.Fatalf("project team: %q err=%v", got, err)
		}
		p, err := st.GetProject(ctx, pid)
		if err != nil || p.TeamID != teamID {
			t.Fatalf("project.TeamID: %+v err=%v", p, err)
		}
	}
	if err := st.SetProjectTeam(ctx, "nope", teamID); err != hoard.ErrNotFound {
		t.Fatalf("set team missing project: %v", err)
	}

	hash := "hash-" + stamp
	exp := time.Now().Add(time.Hour)
	id, err := st.CreatePAT(ctx, owner, "ci", hash, []string{"UPLOAD"}, &exp)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.ValidatePAT(ctx, hash)
	if err != nil || sess.User.Sub != owner || sess.PATID != id || sess.Scopes[0] != "UPLOAD" {
		t.Fatalf("validate: %+v err=%v", sess, err)
	}
	pats, _ := st.ListPATs(ctx, owner)
	if len(pats) != 1 || pats[0].LastUsed == "" {
		t.Fatalf("list pats: %+v", pats)
	}
	if err := st.DeletePAT(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ValidatePAT(ctx, hash); err != hoard.ErrNotFound {
		t.Fatalf("deleted pat: %v", err)
	}
}
