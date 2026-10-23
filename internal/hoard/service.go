package hoard

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Mythicove/shared/queue"
	"github.com/Mythicove/shared/search"
	"github.com/Mythicove/shared/storage"
)

// Service 串接 Store、对象存储、搜索与队列。
type Service struct {
	store         Store
	files         storage.Storage
	search        search.Search
	queue         queue.Queue
	baseURL       string
	requireReview bool
	maxFileBytes  int64
}

// DefaultMaxFileBytes 是单个 mod 文件的默认大小上限（512 MiB）。
const DefaultMaxFileBytes = 512 << 20

// NewService 构造服务。
func NewService(store Store, files storage.Storage, baseURL string) *Service {
	return &Service{store: store, files: files, baseURL: strings.TrimRight(baseURL, "/"), maxFileBytes: DefaultMaxFileBytes}
}

// SetMaxFileBytes 设置单文件大小上限（<=0 恢复默认）。
func (s *Service) SetMaxFileBytes(n int64) {
	if n > 0 {
		s.maxFileBytes = n
	} else {
		s.maxFileBytes = DefaultMaxFileBytes
	}
}

// SetSearch 设置搜索后端（可选，nil 表示使用 Store 内置 pg_trgm 查询）。
func (s *Service) SetSearch(sr search.Search) { s.search = sr }

// SetQueue 设置任务队列（可选；用于异步索引）。
func (s *Service) SetQueue(q queue.Queue) {
	s.queue = q
	if q != nil {
		q.Register("search.index", s.handleIndexTask)
	}
}

// SetRequireReview 开启上传审核（新项目状态为 under_review）。
func (s *Service) SetRequireReview(b bool) { s.requireReview = b }

// UploadInput 是一次上传的输入。
type UploadInput struct {
	Title         string
	Description   string
	Body          string
	ProjectType   string
	ClientSide    string
	ServerSide    string
	Loaders       []string
	GameVersions  []string
	Categories    []string
	VersionName   string
	VersionNumber string
	VersionType   string
	Changelog     string
	Filename      string
	Data          io.Reader
}

// Upload 存储文件并创建 project + version + file 元数据。
func (s *Service) Upload(ctx context.Context, in UploadInput) (*Project, error) {
	if strings.TrimSpace(in.Title) == "" || in.Filename == "" {
		return nil, ErrInvalid
	}
	if in.ProjectType == "" {
		in.ProjectType = "mod"
	}
	if in.VersionType == "" {
		in.VersionType = "release"
	}
	status := "listed"
	if s.requireReview {
		status = "under_review"
	}

	storageKey, sha1sum, sha512sum, size, err := s.storeFile(ctx, in.Data)
	if err != nil {
		return nil, err
	}

	proj, err := s.store.CreateProject(ctx, NewProject{
		Slug:         slugify(in.Title) + "-" + randomSuffix(),
		Title:        in.Title,
		Description:  in.Description,
		Body:         in.Body,
		ProjectType:  in.ProjectType,
		ClientSide:   in.ClientSide,
		ServerSide:   in.ServerSide,
		Status:       status,
		Loaders:      in.Loaders,
		GameVersions: in.GameVersions,
		Categories:   in.Categories,
	})
	if err != nil {
		return nil, err
	}

	ver, err := s.store.CreateVersion(ctx, proj.ID, NewVersion{
		Name:          in.VersionName,
		VersionNumber: in.VersionNumber,
		VersionType:   in.VersionType,
		Changelog:     in.Changelog,
		Loaders:       in.Loaders,
		GameVersions:  in.GameVersions,
	})
	if err != nil {
		return nil, err
	}

	if _, err := s.store.CreateFile(ctx, ver.ID, NewFile{
		Filename:   in.Filename,
		Size:       size,
		SHA1:       sha1sum,
		SHA512:     sha512sum,
		StorageKey: storageKey,
		Primary:    true,
	}); err != nil {
		return nil, err
	}

	s.indexProject(ctx, proj)
	return proj, nil
}

func (s *Service) storeFile(ctx context.Context, r io.Reader) (key, sha1sum, sha512sum string, size int64, err error) {
	tmp, err := os.CreateTemp("", "hoard-upload-*")
	if err != nil {
		return "", "", "", 0, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h1 := sha1.New()
	h512 := sha512.New()
	n, err := io.Copy(io.MultiWriter(tmp, h1, h512), io.LimitReader(r, s.maxFileBytes+1))
	if err != nil {
		return "", "", "", 0, err
	}
	if n > s.maxFileBytes {
		return "", "", "", 0, ErrFileTooLarge
	}
	size = n
	sha1sum = hex.EncodeToString(h1.Sum(nil))
	sha512sum = hex.EncodeToString(h512.Sum(nil))
	key = "files/" + sha512sum

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", "", "", 0, err
	}
	if _, err := s.files.Put(ctx, storage.Object{
		Key:         key,
		Size:        size,
		ContentType: "application/octet-stream",
		Body:        tmp,
	}); err != nil {
		return "", "", "", 0, err
	}
	return key, sha1sum, sha512sum, size, nil
}

// SearchResult 是列表/搜索结果。
type SearchResult struct {
	Hits      []Project `json:"hits"`
	Offset    int       `json:"offset"`
	Limit     int       `json:"limit"`
	TotalHits int64     `json:"total_hits"`
}

// Search 列表/搜索项目（仅返回公开 listed 项目）。
func (s *Service) Search(ctx context.Context, q string, f Filter, limit, offset int) (SearchResult, error) {
	if s.search != nil {
		ids, err := s.search.Query(ctx, "projects", q, map[string][]string{
			"loaders":       f.Loaders,
			"game_versions": f.GameVersions,
			"categories":    f.Categories,
			"project_type":  f.ProjectTypes,
		}, limit, offset)
		if err != nil {
			return SearchResult{}, err
		}
		projects, err := s.store.GetProjectsByIDs(ctx, ids)
		if err != nil {
			return SearchResult{}, err
		}
		return SearchResult{Hits: projects, Offset: offset, Limit: limit, TotalHits: int64(len(projects))}, nil
	}
	projects, total, err := s.store.Search(ctx, q, f, limit, offset)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Hits: projects, Offset: offset, Limit: limit, TotalHits: total}, nil
}

// GetProject 按 id/slug 获取项目（含版本与文件 URL）。
func (s *Service) GetProject(ctx context.Context, idOrSlug string) (*Project, error) {
	return s.store.GetProject(ctx, idOrSlug)
}

// Versions 返回某项目的版本（文件 URL 已补全）。
func (s *Service) Versions(ctx context.Context, projectID string) ([]Version, error) {
	versions, err := s.store.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range versions {
		s.enrichVersion(ctx, projectID, &versions[i])
	}
	return versions, nil
}

// Version 返回单个版本（文件 URL 已补全）。
func (s *Service) Version(ctx context.Context, id string) (*Version, error) {
	v, err := s.store.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	s.enrichVersion(ctx, v.ProjectID, v)
	return v, nil
}

func (s *Service) enrichVersion(ctx context.Context, projectID string, v *Version) {
	files, err := s.store.ListFiles(ctx, v.ID)
	if err != nil {
		return
	}
	v.Files = make([]File, 0, len(files))
	for _, f := range files {
		f.URL = s.fileURL(projectID, v.ID, f.ID)
		v.Files = append(v.Files, f)
	}
}

func (s *Service) fileURL(projectID, versionID, fileID string) string {
	return s.baseURL + "/api/mods/" + projectID + "/version/" + versionID + "/file/" + fileID
}

// Download 返回最新版本的首个文件流。
func (s *Service) Download(ctx context.Context, idOrSlug string) (io.ReadCloser, *File, error) {
	p, err := s.store.GetProject(ctx, idOrSlug)
	if err != nil {
		return nil, nil, err
	}
	versions, err := s.store.ListVersions(ctx, p.ID)
	if err != nil || len(versions) == 0 {
		return nil, nil, ErrNotFound
	}
	files, err := s.store.ListFiles(ctx, versions[0].ID)
	if err != nil || len(files) == 0 {
		return nil, nil, ErrNotFound
	}
	f := files[0]
	rc, err := s.files.Get(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	_ = s.store.IncrementDownloads(ctx, p.ID)
	return rc, &f, nil
}

// FileStream 按 (project, version, file) 返回文件流。
func (s *Service) FileStream(ctx context.Context, projectID, versionID, fileID string) (io.ReadCloser, *File, error) {
	ver, err := s.store.GetVersion(ctx, versionID)
	if err != nil || ver.ProjectID != projectID {
		return nil, nil, ErrNotFound
	}
	files, err := s.store.ListFiles(ctx, versionID)
	if err != nil {
		return nil, nil, err
	}
	for i := range files {
		if files[i].ID == fileID {
			rc, err := s.files.Get(ctx, files[i].StorageKey)
			if err != nil {
				return nil, nil, err
			}
			return rc, &files[i], nil
		}
	}
	return nil, nil, ErrNotFound
}

// SetStatus 更新项目审核状态（listed/under_review/rejected/archived）。
func (s *Service) SetStatus(ctx context.Context, id, status string) error {
	switch status {
	case "listed", "under_review", "rejected", "archived":
	default:
		return ErrInvalid
	}
	return s.store.UpdateProjectStatus(ctx, id, status)
}

// DeleteProject 删除项目及其文件。
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	if err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}
	if s.search != nil {
		_ = s.search.Delete(ctx, "projects", id)
	}
	return nil
}

// Loaders / GameVersions / Categories 返回规范化元数据列表。
func (s *Service) Loaders(ctx context.Context) ([]string, error) { return s.store.ListLoaders(ctx) }
func (s *Service) GameVersions(ctx context.Context) ([]string, error) {
	return s.store.ListGameVersions(ctx)
}
func (s *Service) Categories(ctx context.Context) ([]string, error) {
	return s.store.ListCategories(ctx)
}

func (s *Service) indexProject(ctx context.Context, p *Project) {
	if s.search == nil || s.queue == nil {
		return
	}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = s.queue.Enqueue(ctx, queue.Task{Type: "search.index", Payload: b})
}

func (s *Service) handleIndexTask(ctx context.Context, t queue.Task) error {
	if s.search == nil {
		return nil
	}
	var p Project
	if err := json.Unmarshal(t.Payload, &p); err != nil {
		return err
	}
	return s.search.Index(ctx, "projects", []search.Document{{
		ID:    p.ID,
		Index: "projects",
		Fields: map[string]any{
			"slug":         p.Slug,
			"title":        p.Title,
			"description":  p.Description,
			"project_type": p.ProjectType,
			"downloads":    p.Downloads,
			"published":    p.Published,
		},
		Facets: map[string][]string{
			"loaders":       p.Loaders,
			"game_versions": p.GameVersions,
			"categories":    p.Categories,
		},
	}})
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
