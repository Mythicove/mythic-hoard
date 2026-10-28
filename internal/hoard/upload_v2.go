package hoard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// dependencyInput 是版本依赖的 JSON 输入（Modrinth 形）。
type dependencyInput struct {
	DependencyType string `json:"dependency_type"`
	ProjectID      string `json:"project_id"`
	VersionID      string `json:"version_id"`
	FileName       string `json:"file_name"`
}

func toNewDeps(in []dependencyInput) []NewDependency {
	out := make([]NewDependency, 0, len(in))
	for _, d := range in {
		out = append(out, NewDependency{
			DependencyType: d.DependencyType, ProjectID: d.ProjectID,
			VersionID: d.VersionID, FileName: d.FileName,
		})
	}
	return out
}

// projectData 是 POST /v2/project 的 multipart data 部分（Modrinth 形，数组字段为 JSON 数组）。
type projectData struct {
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Body          string            `json:"body"`
	ProjectType   string            `json:"project_type"`
	ClientSide    string            `json:"client_side"`
	ServerSide    string            `json:"server_side"`
	Loaders       []string          `json:"loaders"`
	GameVersions  []string          `json:"game_versions"`
	Categories    []string          `json:"categories"`
	VersionName   string            `json:"version_name"`
	VersionNumber string            `json:"version_number"`
	VersionType   string            `json:"version_type"`
	Changelog     string            `json:"changelog"`
	Dependencies  []dependencyInput `json:"dependencies"`
}

// versionData 是 POST /v2/version 的 multipart data 部分。
type versionData struct {
	ProjectID     string            `json:"project_id"`
	Name          string            `json:"name"`
	VersionNumber string            `json:"version_number"`
	VersionType   string            `json:"version_type"`
	Changelog     string            `json:"changelog"`
	Loaders       []string          `json:"loaders"`
	GameVersions  []string          `json:"game_versions"`
	Dependencies  []dependencyInput `json:"dependencies"`
}

// requireUploadScope 校验 PAT scopes（会话登录无 scopes 限制；PAT 声明了 scopes 则须含 UPLOAD）。
func requireUploadScope(r *http.Request) bool {
	sc := ScopesFromContext(r.Context())
	if sc == nil {
		return true
	}
	for _, s := range sc {
		if s == "UPLOAD" {
			return true
		}
	}
	return false
}

// handleUserCreateProject 处理用户级项目创建（multipart：data JSON + file）。
func (s *Service) handleUserCreateProject(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if !requireUploadScope(r) {
		http.Error(w, "token lacks UPLOAD scope", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	var data projectData
	if err := json.Unmarshal([]byte(r.FormValue("data")), &data); err != nil {
		http.Error(w, "invalid data part", http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	proj, err := s.UserUpload(r.Context(), u, UploadInput{
		Title:         data.Title,
		Description:   data.Description,
		Body:          data.Body,
		ProjectType:   data.ProjectType,
		ClientSide:    data.ClientSide,
		ServerSide:    data.ServerSide,
		Loaders:       data.Loaders,
		GameVersions:  data.GameVersions,
		Categories:    data.Categories,
		VersionName:   data.VersionName,
		VersionNumber: data.VersionNumber,
		VersionType:   data.VersionType,
		Changelog:     data.Changelog,
		Filename:      hdr.Filename,
		Data:          file,
		Deps:          toNewDeps(data.Dependencies),
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, proj)
}

// handleUserAddVersion 处理用户级发版（multipart：data JSON + file；须为项目团队成员）。
func (s *Service) handleUserAddVersion(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if !requireUploadScope(r) {
		http.Error(w, "token lacks UPLOAD scope", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	var data versionData
	if err := json.Unmarshal([]byte(r.FormValue("data")), &data); err != nil {
		http.Error(w, "invalid data part", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(data.ProjectID) == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ver, err := s.UserAddVersion(r.Context(), u, VersionInput{
		ProjectID:     data.ProjectID,
		Name:          data.Name,
		VersionNumber: data.VersionNumber,
		VersionType:   data.VersionType,
		Changelog:     data.Changelog,
		Loaders:       data.Loaders,
		GameVersions:  data.GameVersions,
		Filename:      hdr.Filename,
		Data:          file,
		Deps:          toNewDeps(data.Dependencies),
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ver)
}

// handleUserDeleteProject 删除项目（团队 owner 或站点 admin）。
func (s *Service) handleUserDeleteProject(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if !requireUploadScope(r) {
		http.Error(w, "token lacks UPLOAD scope", http.StatusForbidden)
		return
	}
	if err := s.UserDeleteProject(r.Context(), u, chi.URLParam(r, "id")); err != nil {
		writeUploadError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleProjectDependencies 返回项目全部版本的依赖（Modrinth 形）。
func (s *Service) handleProjectDependencies(w http.ResponseWriter, r *http.Request) {
	p, err := s.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	deps, err := s.store.ListProjectDependencies(r.Context(), p.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, deps)
}

// writeUploadError 统一映射用户级写端点错误。
func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrFileTooLarge):
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrAlreadyExists):
		http.Error(w, "already exists", http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
