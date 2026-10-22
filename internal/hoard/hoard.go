// Package hoard 提供 mod/整合包分发后端（Modrinth 兼容读 + .mrpack）。
package hoard

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Project 对应 Modrinth 的 project。
type Project struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ProjectType  string   `json:"project_type"`
	Loaders      []string `json:"loaders,omitempty"`
	GameVersions []string `json:"game_versions,omitempty"`
	Downloads    int64    `json:"downloads"`
	Published    string   `json:"published"`
}

// Version 对应 Modrinth 的 version。
type Version struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	VersionType   string   `json:"version_type"`
	Loaders       []string `json:"loaders,omitempty"`
	GameVersions  []string `json:"game_versions,omitempty"`
	DatePublished string   `json:"date_published"`
	Files         []File   `json:"files"`
}

// File 对应 Modrinth 的 version file。
type File struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	path     string            `json:"-"`
}

// Store 是内存元数据 + 本地文件存储（MVP；M2 换 Postgres + S3）。
type Store struct {
	mu       sync.RWMutex
	dir      string
	projects map[string]*Project
	versions map[string]*Version
	order    []string // project 插入顺序
	seq      int
}

// New 构造 Store。
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dir:      dir,
		projects: map[string]*Project{},
		versions: map[string]*Version{},
	}, nil
}

// Upload 存储文件并创建 project + version。
func (s *Store) Upload(title, description, projectType, versionNumber string, loaders, gameVersions []string, filename string, r io.Reader) (*Project, error) {
	fileID := fmt.Sprintf("f%d", s.next())
	dst := filepath.Join(s.dir, fileID)
	f, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	h1 := sha1.New()
	h512 := sha512.New()
	n, err := io.Copy(io.MultiWriter(f, h1, h512), r)
	f.Close()
	if err != nil {
		return nil, err
	}

	file := &File{
		Hashes:   map[string]string{"sha1": hex.EncodeToString(h1.Sum(nil)), "sha512": hex.EncodeToString(h512.Sum(nil))},
		Filename: filename,
		Primary:  true,
		Size:     n,
		path:     dst,
	}
	projectID := fmt.Sprintf("p%d", s.next())
	versionID := fmt.Sprintf("v%d", s.next())
	now := time.Now().UTC().Format(time.RFC3339)

	project := &Project{
		ID: projectID, Slug: slugify(title) + "-" + projectID, Title: title, Description: description,
		ProjectType: projectType, Loaders: loaders, GameVersions: gameVersions, Published: now,
	}
	version := &Version{
		ID: versionID, ProjectID: projectID, VersionNumber: versionNumber, VersionType: "release",
		Loaders: loaders, GameVersions: gameVersions, DatePublished: now, Files: []File{*file},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[projectID] = project
	s.versions[versionID] = version
	s.order = append(s.order, projectID)
	return project, nil
}

// List 按插入顺序返回项目。
func (s *Store) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Project, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.projects[id])
	}
	return out
}

// GetProject 按 id 返回项目。
func (s *Store) GetProject(id string) (*Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	return p, ok
}

// GetVersion 按 id 返回版本。
func (s *Store) GetVersion(id string) (*Version, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.versions[id]
	return v, ok
}

// VersionsOf 返回某项目的全部版本。
func (s *Store) VersionsOf(projectID string) []*Version {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Version{}
	for _, v := range s.versions {
		if v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	return out
}

// FindProject 支持按 slug/id 查找。
func (s *Store) FindProject(idOrSlug string) (*Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.projects[idOrSlug]; ok {
		return p, true
	}
	for _, p := range s.projects {
		if p.Slug == idOrSlug {
			return p, true
		}
	}
	return nil, false
}

// OpenFile 打开版本的首个文件，并累加下载次数。
func (s *Store) OpenFile(version *Version) (io.ReadCloser, *File, error) {
	if len(version.Files) == 0 {
		return nil, nil, os.ErrNotExist
	}
	f := &version.Files[0]
	rc, err := os.Open(f.path)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	if p, ok := s.projects[version.ProjectID]; ok {
		p.Downloads++
	}
	s.mu.Unlock()
	return rc, f, nil
}

func (s *Store) next() int {
	s.seq++
	return s.seq
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
