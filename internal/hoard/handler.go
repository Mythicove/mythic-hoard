package hoard

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Register 注册路由；adminToken 用于保护上传端点（空则开放）。
func (s *Store) Register(r chi.Router, adminToken string) {
	r.Group(func(r chi.Router) {
		r.Use(Admin(adminToken))
		r.Post("/api/mods", s.handleUpload)
	})
	r.Get("/api/mods", s.handleList)
	r.Get("/api/mods/{id}", s.handleGetProject)
	r.Get("/api/mods/{id}/download", s.handleDownload)
	r.Get("/v2/search", s.handleSearch)
	r.Get("/v2/project/{id}", s.handleGetProjectModrinth)
	r.Get("/v2/project/{id}/version", s.handleProjectVersions)
	r.Get("/v2/version/{id}", s.handleGetVersion)
	r.Get("/v2/project/{id}/version/{vid}/mrpack", s.handleMrpack)
}

func (s *Store) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	description := r.FormValue("description")
	projectType := r.FormValue("project_type")
	if projectType == "" {
		projectType = "mod"
	}
	versionNumber := r.FormValue("version_number")
	loaders := splitCSV(r.FormValue("loaders"))
	gameVersions := splitCSV(r.FormValue("game_versions"))
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer f.Close()

	project, err := s.Upload(title, description, projectType, versionNumber, loaders, gameVersions, hdr.Filename, f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (s *Store) handleList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.List())
}

func (s *Store) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.FindProject(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Store) handleDownload(w http.ResponseWriter, r *http.Request) {
	p, ok := s.FindProject(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	versions := s.VersionsOf(p.ID)
	if len(versions) == 0 {
		http.NotFound(w, r)
		return
	}
	rc, file, err := s.OpenFile(versions[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", "attachment; filename="+file.Filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, rc)
}

func (s *Store) handleSearch(w http.ResponseWriter, _ *http.Request) {
	projects := s.List()
	hits := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		hits = append(hits, map[string]any{
			"project_id": p.ID, "project_type": p.ProjectType, "slug": p.Slug,
			"title": p.Title, "description": p.Description, "downloads": p.Downloads,
			"author": "mythicove", "date_modified": p.Published,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hits": hits, "offset": 0, "limit": len(hits), "total_hits": len(hits),
	})
}

func (s *Store) handleGetProjectModrinth(w http.ResponseWriter, r *http.Request) {
	p, ok := s.FindProject(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Store) handleProjectVersions(w http.ResponseWriter, r *http.Request) {
	p, ok := s.FindProject(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, s.VersionsOf(p.ID))
}

func (s *Store) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	v, ok := s.GetVersion(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Store) handleMrpack(w http.ResponseWriter, r *http.Request) {
	v, ok := s.GetVersion(chi.URLParam(r, "vid"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, _ := s.GetProject(v.ProjectID)

	files := make([]map[string]any, 0, len(v.Files))
	for _, f := range v.Files {
		files = append(files, map[string]any{
			"path":      f.Filename,
			"hashes":    f.Hashes,
			"downloads": []string{"/api/mods/" + p.ID + "/download"},
			"fileSize":  f.Size,
		})
	}
	index := map[string]any{
		"formatVersion": 1,
		"game":          "minecraft",
		"versionId":     v.VersionNumber,
		"name":          p.Title,
		"files":         files,
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+p.Slug+".mrpack")
	zw := zip.NewWriter(w)
	entry, _ := zw.Create("modrinth.index.json")
	data, _ := json.Marshal(index)
	_, _ = entry.Write(data)
	_ = zw.Close()
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
