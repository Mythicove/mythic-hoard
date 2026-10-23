package pgstore

import (
	"context"
	"os"
	"testing"

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
