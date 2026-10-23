package hoard

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func buildTestJar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUploadModMetadata(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	jar := buildTestJar(t, map[string][]byte{
		"fabric.mod.json": []byte(`{
			"id": "coolmod", "version": "2.5.0", "name": "Cool Mod",
			"description": "Does cool things", "license": "MIT",
			"icon": "assets/coolmod/icon.png",
			"depends": {"minecraft": ">=1.20.1"}
		}`),
		"assets/coolmod/icon.png": []byte("fake-icon-bytes"),
		"com/example/Main.class":  []byte("cafebabe"),
	})

	// 1. 表单留空 → 全部由 jar 元数据填充
	proj, err := svc.Upload(ctx, UploadInput{Filename: "coolmod.jar", Data: bytes.NewReader(jar)})
	if err != nil {
		t.Fatal(err)
	}
	if proj.Title != "Cool Mod" || proj.Description != "Does cool things" || proj.License != "MIT" {
		t.Fatalf("fill: %+v", proj)
	}
	if len(proj.Loaders) != 1 || proj.Loaders[0] != "fabric" {
		t.Fatalf("loaders: %+v", proj.Loaders)
	}
	if len(proj.GameVersions) != 1 || proj.GameVersions[0] != "1.20.1" {
		t.Fatalf("game versions: %+v", proj.GameVersions)
	}
	if len(proj.MetaFlags) != 0 {
		t.Fatalf("unexpected flags: %+v", proj.MetaFlags)
	}
	// 版本名同样兜底
	vers, err := svc.Versions(ctx, proj.ID)
	if err != nil || len(vers) != 1 || vers[0].VersionNumber != "2.5.0" {
		t.Fatalf("version fill: %+v err=%v", vers, err)
	}
	// icon URL 与直出
	if proj.IconURL == "" {
		t.Fatal("icon url empty")
	}
	r := chi.NewRouter()
	svc.Register(r, "")
	srv := httptest.NewServer(r)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/mods/" + proj.ID + "/icon")
	if err != nil {
		t.Fatal(err)
	}
	icon := new(bytes.Buffer)
	_, _ = icon.ReadFrom(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || icon.String() != "fake-icon-bytes" {
		t.Fatalf("icon serve: %d %q", res.StatusCode, icon.String())
	}

	// 2. 表单冲突 → 表单优先 + 标旗
	conflicted, err := svc.Upload(ctx, UploadInput{
		Title:        "Different Title",
		Description:  "Other desc",
		Loaders:      []string{"quilt"},
		GameVersions: []string{"1.19"},
		Filename:     "coolmod2.jar",
		Data:         bytes.NewReader(jar),
	})
	if err != nil {
		t.Fatal(err)
	}
	if conflicted.Title != "Different Title" || conflicted.Loaders[0] != "quilt" {
		t.Fatalf("form should win: %+v", conflicted)
	}
	want := map[string]bool{
		"title_mismatch":         false,
		"description_mismatch":   false,
		"loader_mismatch":        false,
		"game_versions_mismatch": false,
	}
	for _, f := range conflicted.MetaFlags {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for f, seen := range want {
		if !seen {
			t.Fatalf("flag %q missing in %+v", f, conflicted.MetaFlags)
		}
	}

	// 3. 非 jar 文件（无元数据）仍可上传，表单必填
	if _, err := svc.Upload(ctx, UploadInput{Filename: "plain.jar", Data: bytes.NewReader([]byte("not a zip"))}); err != ErrInvalid {
		t.Fatalf("no-meta no-title upload: %v", err)
	}
	plain, err := svc.Upload(ctx, UploadInput{Title: "Plain", Filename: "plain.jar", Data: bytes.NewReader([]byte("not a zip"))})
	if err != nil || plain.Title != "Plain" || plain.IconURL != "" {
		t.Fatalf("plain upload: %+v err=%v", plain, err)
	}
}
