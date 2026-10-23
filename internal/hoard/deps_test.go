package hoard

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestVersionDependenciesAndMrpack(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	// 建项目（fabric 1.20.1，带两条依赖）
	proj, err := svc.Upload(ctx, UploadInput{
		Title: "Dep Mod", ProjectType: "mod",
		Loaders: []string{"fabric"}, GameVersions: []string{"1.20.1"},
		ClientSide: "required", ServerSide: "optional",
		VersionName: "v1", VersionNumber: "1.0.0",
		Filename: "depmod.jar", Data: bytes.NewReader([]byte("jar-1")),
		Deps: []NewDependency{
			{DependencyType: "required", ProjectID: "proj-b"},
			{DependencyType: "embedded", FileName: "lib.jar"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 非法依赖类型 → ErrInvalid
	if _, err := svc.Upload(ctx, UploadInput{
		Title: "Bad Dep", VersionNumber: "1.0", Filename: "x.jar",
		Data: bytes.NewReader([]byte("jar")),
		Deps: []NewDependency{{DependencyType: "bogus"}},
	}); err != ErrInvalid {
		t.Fatalf("bad dep type: %v", err)
	}

	// 项目依赖列表
	deps, err := svc.Store().ListProjectDependencies(ctx, proj.ID)
	if err != nil || len(deps) != 2 {
		t.Fatalf("deps: %+v err=%v", deps, err)
	}
	if deps[0].DependencyType != "required" || deps[0].ProjectID != "proj-b" {
		t.Fatalf("dep0: %+v", deps[0])
	}
	if deps[1].FileName != "lib.jar" {
		t.Fatalf("dep1: %+v", deps[1])
	}

	// 追加版本（带依赖）
	ver, err := svc.UserAddVersion(ctx, &User{Sub: "", Role: "admin"}, VersionInput{
		ProjectID: proj.ID, VersionNumber: "1.1.0", Filename: "depmod-1.1.jar",
		Data: bytes.NewReader([]byte("jar-2")),
		Deps: []NewDependency{{DependencyType: "optional", ProjectID: "proj-c"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	vdeps, _ := svc.Store().ListVersionDependencies(ctx, ver.ID)
	if len(vdeps) != 1 || vdeps[0].ProjectID != "proj-c" {
		t.Fatalf("version deps: %+v", vdeps)
	}
	deps, _ = svc.Store().ListProjectDependencies(ctx, proj.ID)
	if len(deps) != 3 {
		t.Fatalf("project deps after add: %d", len(deps))
	}

	// mrpack：minecraft 依赖版本 + env + overrides/
	r := chi.NewRouter()
	svc.Register(r, "")
	srv := httptest.NewServer(r)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/v2/project/" + proj.ID + "/version/" + ver.ID + "/mrpack")
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mrpack status: %d", res.StatusCode)
	}
	zr, err := zip.NewReader(bytes.NewReader(pack), int64(len(pack)))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	hasOverrides := false
	for _, f := range zr.File {
		if f.Name == "overrides/" || f.Name == "overrides" {
			hasOverrides = true
		}
		if f.Name != "modrinth.index.json" {
			continue
		}
		rc, _ := f.Open()
		if err := json.NewDecoder(rc).Decode(&index); err != nil {
			t.Fatal(err)
		}
		rc.Close()
	}
	if !hasOverrides {
		t.Fatal("overrides/ missing in mrpack")
	}
	depsMap, _ := index["dependencies"].(map[string]any)
	mc, _ := depsMap["minecraft"].(map[string]any)
	if mc == nil || mc["version"] != "1.20.1" {
		t.Fatalf("mrpack minecraft dep: %+v", depsMap)
	}
	fl, _ := depsMap["fabric-loader"].(map[string]any)
	if fl == nil {
		t.Fatalf("mrpack loader dep missing: %+v", depsMap)
	}
	env, _ := index["env"].(map[string]any)
	if env["client"] != "required" || env["server"] != "optional" {
		t.Fatalf("mrpack env: %+v", env)
	}

	// GET /v2/project/{id}/dependencies 端点
	res, err = http.Get(srv.URL + "/v2/project/" + proj.ID + "/dependencies")
	if err != nil {
		t.Fatal(err)
	}
	var httpDeps []Dependency
	_ = json.NewDecoder(res.Body).Decode(&httpDeps)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || len(httpDeps) != 3 {
		t.Fatalf("http deps: %d %d", res.StatusCode, len(httpDeps))
	}
}
