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

	"github.com/go-chi/chi/v5"
)

func TestUploadListDownloadMrpack(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	store.Register(r)
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
	var projects []*Project
	_ = json.NewDecoder(resp.Body).Decode(&projects)
	resp.Body.Close()
	if len(projects) != 1 || projects[0].Title != "Fabric Mod" {
		t.Fatalf("list: %+v", projects)
	}

	// Modrinth 兼容读
	resp, _ = http.Get(srv.URL + "/v2/project/" + projects[0].ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("modrinth project status %d", resp.StatusCode)
	}

	// 下载
	resp, _ = http.Get(srv.URL + "/api/mods/" + projects[0].ID + "/download")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "fake-jar-bytes" {
		t.Fatalf("download: %q", data)
	}

	// 取版本 id
	resp, _ = http.Get(srv.URL + "/v2/project/" + projects[0].ID + "/version")
	var versions []*Version
	_ = json.NewDecoder(resp.Body).Decode(&versions)
	resp.Body.Close()
	if len(versions) != 1 {
		t.Fatalf("versions: %+v", versions)
	}

	// mrpack
	resp, _ = http.Get(srv.URL + "/v2/project/" + projects[0].ID + "/version/" + versions[0].ID + "/mrpack")
	zdata, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(zdata), int64(len(zdata)))
	if err != nil {
		t.Fatal(err)
	}
	hasIndex := false
	for _, f := range zr.File {
		if f.Name == "modrinth.index.json" {
			hasIndex = true
		}
	}
	if !hasIndex {
		t.Fatal("mrpack missing modrinth.index.json")
	}
}
