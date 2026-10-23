package hoard

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mythicove/mythic-hoard/internal/storage"
	"github.com/go-chi/chi/v5"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	files, err := storage.NewLocal(t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	return NewService(NewMemoryStore(), files, "http://localhost:8080")
}

func TestUploadListDownloadMrpack(t *testing.T) {
	svc := newTestService(t)
	r := chi.NewRouter()
	svc.Register(r, "")
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 上传
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("title", "Fabric Mod")
	_ = mw.WriteField("description", "test mod")
	_ = mw.WriteField("project_type", "mod")
	_ = mw.WriteField("version_number", "1.0.0")
	_ = mw.WriteField("loaders", "fabric")
	_ = mw.WriteField("game_versions", "1.20.1")
	fw, _ := mw.CreateFormFile("file", "mod.jar")
	_, _ = fw.Write([]byte("fake-jar-bytes"))
	_ = mw.Close()

	resp, err := http.Post(srv.URL+"/api/mods", mw.FormDataContentType(), body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 列表
	resp, _ = http.Get(srv.URL + "/api/mods")
	var result SearchResult
	_ = json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if len(result.Hits) != 1 || result.Hits[0].Title != "Fabric Mod" {
		t.Fatalf("list: %+v", result)
	}
	projID := result.Hits[0].ID

	// Modrinth 兼容读
	resp, _ = http.Get(srv.URL + "/v2/project/" + projID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("modrinth project status %d", resp.StatusCode)
	}

	// 下载
	resp, _ = http.Get(srv.URL + "/api/mods/" + projID + "/download")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "fake-jar-bytes" {
		t.Fatalf("download: %q", data)
	}

	// 取版本 id
	resp, _ = http.Get(srv.URL + "/v2/project/" + projID + "/version")
	var versions []Version
	_ = json.NewDecoder(resp.Body).Decode(&versions)
	resp.Body.Close()
	if len(versions) != 1 {
		t.Fatalf("versions: %+v", versions)
	}

	// mrpack
	resp, _ = http.Get(srv.URL + "/v2/project/" + projID + "/version/" + versions[0].ID + "/mrpack")
	zdata, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(zdata), int64(len(zdata)))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	for _, f := range zr.File {
		if f.Name == "modrinth.index.json" {
			rc, _ := f.Open()
			_ = json.NewDecoder(rc).Decode(&index)
			rc.Close()
		}
	}
	if index == nil {
		t.Fatal("mrpack missing modrinth.index.json")
	}
	deps, _ := index["dependencies"].(map[string]any)
	if _, ok := deps["fabric-loader"]; !ok {
		t.Fatalf("mrpack dependencies missing fabric-loader: %v", index["dependencies"])
	}
	mods, _ := index["files"].([]any)
	if len(mods) != 1 {
		t.Fatalf("mrpack files: %v", index["files"])
	}
	entry, _ := mods[0].(map[string]any)
	if entry["path"] != "mods/mod.jar" {
		t.Fatalf("mrpack file path = %v, want mods/mod.jar", entry["path"])
	}
}
