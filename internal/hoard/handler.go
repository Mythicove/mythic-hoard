package hoard

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Register 注册路由；adminToken 用于保护写端点（空则开放）。
// 注意：会话注入中间件（SessionAuth）须在调用方注册路由**之前**挂载（chi 约束），见 cmd/server/main.go。
func (s *Service) Register(r chi.Router, adminToken string) {
	// OIDC 登录流（配置了 RP 才有意义，未配置时端点返回 404）
	r.Get("/auth/oidc/login", s.HandleOIDCLogin)
	r.Get("/auth/oidc/callback", s.HandleOIDCCallback)
	r.Post("/auth/logout", s.HandleLogout)
	r.Get("/auth/me", s.HandleMe)

	// 用户资源（需登录：会话 cookie 或 PAT）
	r.Group(func(r chi.Router) {
		r.Use(RequireUser())
		r.Post("/api/team", s.handleCreateTeam)
		r.Get("/api/user/teams", s.handleListMyTeams)
		r.Get("/api/user/pats", s.handleListPATs)
		r.Post("/api/user/pats", s.handleCreatePAT)
		r.Delete("/api/user/pats/{id}", s.handleDeletePAT)
		r.Post("/api/team/{id}/members", s.handleAddTeamMember)
		r.Delete("/api/team/{id}/members/{sub}", s.handleRemoveTeamMember)
		// 团队邀请（加入需被邀请人同意）
		r.Post("/api/team/{id}/invites", s.handleInviteMember)
		r.Get("/api/team/{id}/invites", s.handleListTeamInvites)
		r.Delete("/api/team/{id}/invites/{sub}", s.handleRevokeTeamInvite)
		r.Get("/api/user/invites", s.handleListMyInvites)
		r.Post("/api/user/invites/{teamId}/accept", s.handleAcceptInvite)
		r.Delete("/api/user/invites/{teamId}", s.handleRejectInvite)
		// 用户级上传（Modrinth 形 multipart；管理员 /api/mods 队列保留）
		r.Post("/v2/project", s.handleUserCreateProject)
		r.Post("/v2/version", s.handleUserAddVersion)
		r.Delete("/v2/project/{id}", s.handleUserDeleteProject)
	})

	// 团队成员列表公开（与 Modrinth 一致）
	r.Get("/api/team/{id}/members", s.handleListTeamMembers)

	r.Group(func(r chi.Router) {
		r.Use(AdminRole(adminToken))
		r.Post("/api/mods", s.handleUpload)
		r.Patch("/api/mods/{id}/status", s.handleSetStatus)
		r.Delete("/api/mods/{id}", s.handleDelete)
	})

	r.Get("/api/mods", s.handleSearch)
	r.Get("/api/mods/facets", s.handleFacets)
	r.Get("/api/mods/{id}", s.handleGetProject)
	r.Get("/api/mods/{id}/icon", s.handleIcon)
	r.Get("/api/mods/{id}/download", s.handleDownload)
	r.Get("/api/mods/{id}/version/{vid}/file/{fid}", s.handleFile)

	r.Get("/v2/search", s.handleSearch)
	r.Get("/v2/project/{id}", s.handleGetProject)
	r.Get("/v2/project/{id}/icon", s.handleIcon)
	r.Get("/v2/project/{id}/dependencies", s.handleProjectDependencies)
	r.Get("/v2/project/{id}/version", s.handleProjectVersions)
	r.Get("/v2/version/{id}", s.handleGetVersion)
	r.Get("/v2/project/{id}/version/{vid}/mrpack", s.handleMrpack)
	r.Get("/v2/tag/loader", s.handleTagLoaders)
	r.Get("/v2/tag/game_version", s.handleTagGameVersions)
	r.Get("/v2/tag/category", s.handleTagCategories)
}

func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer f.Close()

	project, err := s.Upload(r.Context(), UploadInput{
		Title:         r.FormValue("title"),
		Description:   r.FormValue("description"),
		Body:          r.FormValue("body"),
		ProjectType:   r.FormValue("project_type"),
		ClientSide:    r.FormValue("client_side"),
		ServerSide:    r.FormValue("server_side"),
		Loaders:       splitCSV(r.FormValue("loaders")),
		GameVersions:  splitCSV(r.FormValue("game_versions")),
		Categories:    splitCSV(r.FormValue("categories")),
		VersionName:   r.FormValue("name"),
		VersionNumber: r.FormValue("version_number"),
		VersionType:   r.FormValue("version_type"),
		Changelog:     r.FormValue("changelog"),
		Filename:      hdr.Filename,
		Data:          f,
	})
	if err != nil {
		switch {
		case err == ErrInvalid:
			http.Error(w, "title and file are required", http.StatusBadRequest)
		case err == ErrFileTooLarge:
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	f := filterFromQuery(r)
	limit := intQuery(r, "limit", 20)
	offset := intQuery(r, "offset", 0)
	f.Status = "listed"

	res, err := s.Search(r.Context(), q, f, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleFacets 返回可选过滤维度（loaders/game_versions/categories），供前端渲染过滤 chips。
func (s *Service) handleFacets(w http.ResponseWriter, r *http.Request) {
	loaders, err1 := s.store.ListLoaders(r.Context())
	versions, err2 := s.store.ListGameVersions(r.Context())
	categories, err3 := s.store.ListCategories(r.Context())
	if err1 != nil || err2 != nil || err3 != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	empty := []string{}
	if loaders == nil {
		loaders = empty
	}
	if versions == nil {
		versions = empty
	}
	if categories == nil {
		categories = empty
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"loaders":       loaders,
		"game_versions": versions,
		"categories":    categories,
	})
}

func (s *Service) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleIcon 直出项目 icon（jar 内解析所得；无 icon 时 404）。
func (s *Service) handleIcon(w http.ResponseWriter, r *http.Request) {
	p, err := s.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.IconKey == "" {
		http.NotFound(w, r)
		return
	}
	rc, err := s.files.Get(r.Context(), p.IconKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

func (s *Service) handleDownload(w http.ResponseWriter, r *http.Request) {
	rc, file, err := s.Download(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", "attachment; filename="+file.Filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	_, _ = io.Copy(w, rc)
}

func (s *Service) handleFile(w http.ResponseWriter, r *http.Request) {
	rc, file, err := s.FileStream(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "vid"), chi.URLParam(r, "fid"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", "attachment; filename="+file.Filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	_, _ = io.Copy(w, rc)
}

func (s *Service) handleProjectVersions(w http.ResponseWriter, r *http.Request) {
	p, err := s.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	versions, err := s.Versions(r.Context(), p.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *Service) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.Version(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Service) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.SetStatus(r.Context(), chi.URLParam(r, "id"), req.Status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.DeleteProject(r.Context(), chi.URLParam(r, "id")); err != nil {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleMrpack(w http.ResponseWriter, r *http.Request) {
	v, err := s.Version(r.Context(), chi.URLParam(r, "vid"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.GetProject(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	isMod := p.ProjectType == "mod"
	files := make([]map[string]any, 0, len(v.Files))
	for _, f := range v.Files {
		path := f.Filename
		if isMod {
			path = "mods/" + f.Filename
		}
		files = append(files, map[string]any{
			"path":      path,
			"hashes":    f.Hashes,
			"downloads": []string{f.URL},
			"fileSize":  f.Size,
		})
	}
	deps := mrpackDependencies(v.Loaders, p.Loaders)
	// minecraft 版本：版本级 game_versions 优先，项目级兜底，均无则 "*"
	mcVersion := "*"
	gv := v.GameVersions
	if len(gv) == 0 {
		gv = p.GameVersions
	}
	if len(gv) > 0 {
		mcVersion = gv[0]
	}
	deps["minecraft"] = map[string]string{"version": mcVersion}
	index := map[string]any{
		"formatVersion": 1,
		"game":          "minecraft",
		"versionId":     v.VersionNumber,
		"name":          p.Title,
		"files":         files,
		"dependencies":  deps,
		"env": map[string]string{
			"client": mrpackEnv(p.ClientSide),
			"server": mrpackEnv(p.ServerSide),
		},
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+p.Slug+".mrpack")
	zw := zip.NewWriter(w)
	entry, _ := zw.Create("modrinth.index.json")
	data, _ := json.Marshal(index)
	_, _ = entry.Write(data)
	// overrides/ 目录（生成场景无覆盖文件，保留空目录符合 mrpack 惯例）
	_, _ = zw.Create("overrides/")
	_ = zw.Close()
}

// mrpackEnv 把项目 client_side/server_side 映射为 mrpack env 值。
func mrpackEnv(v string) string {
	switch v {
	case "required", "optional", "unsupported":
		return v
	default:
		return "required"
	}
}

// mrpackLoaderIDs 把项目 loader 名映射为 mrpack dependencies 的键。
var mrpackLoaderIDs = map[string]string{
	"fabric":     "fabric-loader",
	"quilt":      "quilt-loader",
	"forge":      "forge",
	"neoforge":   "neoforge",
	"liteloader": "liteloader",
	"modloader":  "modloader",
}

// mrpackDependencies 从版本（优先）或项目的 loaders 生成 mrpack 必需的 dependencies 对象。
func mrpackDependencies(versionLoaders, projectLoaders []string) map[string]any {
	loaders := versionLoaders
	if len(loaders) == 0 {
		loaders = projectLoaders
	}
	deps := map[string]any{}
	for _, l := range loaders {
		if id, ok := mrpackLoaderIDs[strings.ToLower(l)]; ok {
			deps[id] = map[string]string{"version": "*"}
		}
	}
	return deps
}

func (s *Service) handleTagLoaders(w http.ResponseWriter, r *http.Request) {
	list, err := s.Loaders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Service) handleTagGameVersions(w http.ResponseWriter, r *http.Request) {
	list, err := s.GameVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Service) handleTagCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.Categories(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// filterFromQuery 从查询参数构造 Filter，兼容 Modrinth facets 与简单参数。
func filterFromQuery(r *http.Request) Filter {
	f := Filter{}
	if facets := r.URL.Query().Get("facets"); facets != "" {
		parseFacets(facets, &f)
	}
	if v := r.URL.Query().Get("loaders"); v != "" {
		f.Loaders = splitCSV(v)
	}
	if v := r.URL.Query().Get("game_versions"); v != "" {
		f.GameVersions = splitCSV(v)
	}
	if v := r.URL.Query().Get("project_type"); v != "" {
		f.ProjectTypes = splitCSV(v)
	}
	if v := r.URL.Query().Get("categories"); v != "" {
		f.Categories = splitCSV(v)
	}
	return f
}

// parseFacets 解析 Modrinth facets（外层 AND，内层 OR）：[["versions:1.20.1"],["project_type:mod"]]。
func parseFacets(raw string, f *Filter) {
	var groups [][]string
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return
	}
	for _, group := range groups {
		for _, entry := range group {
			key, val, ok := strings.Cut(entry, ":")
			if !ok {
				continue
			}
			switch key {
			case "loaders":
				f.Loaders = append(f.Loaders, val)
			case "versions":
				f.GameVersions = append(f.GameVersions, val)
			case "categories":
				f.Categories = append(f.Categories, val)
			case "project_type":
				f.ProjectTypes = append(f.ProjectTypes, val)
			}
		}
	}
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

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
