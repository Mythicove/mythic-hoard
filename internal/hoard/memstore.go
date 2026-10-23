package hoard

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore 是进程内实现（开发/测试）。
type MemoryStore struct {
	mu            sync.RWMutex
	seq           int
	projects      map[string]*Project
	versions      map[string]*Version
	files         map[string][]*File
	loadersSet    map[string]struct{}
	gameVersSet   map[string]struct{}
	categoriesSet map[string]struct{}
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore 构造内存 Store。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		projects:      map[string]*Project{},
		versions:      map[string]*Version{},
		files:         map[string][]*File{},
		loadersSet:    map[string]struct{}{},
		gameVersSet:   map[string]struct{}{},
		categoriesSet: map[string]struct{}{},
	}
}

func (s *MemoryStore) nextID(prefix string) string {
	s.seq++
	return prefix + itoa(s.seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *MemoryStore) CreateProject(_ context.Context, p NewProject) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.projects {
		if x.Slug == p.Slug {
			return nil, ErrAlreadyExists
		}
	}
	proj := &Project{
		ID:           s.nextID("p"),
		Slug:         p.Slug,
		Title:        p.Title,
		Description:  p.Description,
		Body:         p.Body,
		ProjectType:  p.ProjectType,
		ClientSide:   p.ClientSide,
		ServerSide:   p.ServerSide,
		Status:       p.Status,
		Loaders:      p.Loaders,
		GameVersions: p.GameVersions,
		Categories:   p.Categories,
		Published:    time.Now().UTC().Format(time.RFC3339),
	}
	s.projects[proj.ID] = proj
	for _, l := range p.Loaders {
		s.loadersSet[l] = struct{}{}
	}
	for _, g := range p.GameVersions {
		s.gameVersSet[g] = struct{}{}
	}
	for _, c := range p.Categories {
		s.categoriesSet[c] = struct{}{}
	}
	return proj, nil
}

func (s *MemoryStore) GetProject(_ context.Context, idOrSlug string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.projects[idOrSlug]; ok {
		return p, nil
	}
	for _, p := range s.projects {
		if p.Slug == idOrSlug {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

func (s *MemoryStore) GetProjectsByIDs(_ context.Context, ids []string) ([]Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, 0, len(ids))
	for _, id := range ids {
		if p, ok := s.projects[id]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (s *MemoryStore) Search(_ context.Context, q string, f Filter, limit, offset int) ([]Project, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Title), strings.ToLower(q)) && !strings.Contains(strings.ToLower(p.Description), strings.ToLower(q)) {
			continue
		}
		if !containsAny(p.Loaders, f.Loaders) || !containsAny(p.GameVersions, f.GameVersions) ||
			!containsAny(p.Categories, f.Categories) || !containsAny([]string{p.ProjectType}, f.ProjectTypes) {
			continue
		}
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Downloads > all[j].Downloads })
	total := int64(len(all))
	if offset >= len(all) {
		return []Project{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	out := make([]Project, 0, end-offset)
	for _, p := range all[offset:end] {
		out = append(out, *p)
	}
	return out, total, nil
}

func containsAny(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, h := range have {
		set[h] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}

func (s *MemoryStore) UpdateProjectStatus(_ context.Context, id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.Status = status
	return nil
}

func (s *MemoryStore) DeleteProject(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return ErrNotFound
	}
	delete(s.projects, id)
	return nil
}

func (s *MemoryStore) IncrementDownloads(_ context.Context, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.projects[projectID]; ok {
		p.Downloads++
	}
	return nil
}

func (s *MemoryStore) CreateVersion(_ context.Context, projectID string, v NewVersion) (*Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ver := &Version{
		ID:            s.nextID("v"),
		ProjectID:     projectID,
		Name:          v.Name,
		VersionNumber: v.VersionNumber,
		VersionType:   v.VersionType,
		Changelog:     v.Changelog,
		Loaders:       v.Loaders,
		GameVersions:  v.GameVersions,
		DatePublished: time.Now().UTC().Format(time.RFC3339),
	}
	s.versions[ver.ID] = ver
	for _, l := range v.Loaders {
		s.loadersSet[l] = struct{}{}
	}
	for _, g := range v.GameVersions {
		s.gameVersSet[g] = struct{}{}
	}
	return ver, nil
}

func (s *MemoryStore) ListVersions(_ context.Context, projectID string) ([]Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Version{}
	for _, v := range s.versions {
		if v.ProjectID == projectID {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (s *MemoryStore) GetVersion(_ context.Context, id string) (*Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.versions[id]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (s *MemoryStore) CreateFile(_ context.Context, versionID string, f NewFile) (*File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file := &File{
		ID:         s.nextID("f"),
		StorageKey: f.StorageKey,
		Hashes:     map[string]string{"sha1": f.SHA1, "sha512": f.SHA512},
		Filename:   f.Filename,
		Primary:    f.Primary,
		Size:       f.Size,
	}
	s.files[versionID] = append(s.files[versionID], file)
	return file, nil
}

func (s *MemoryStore) ListFiles(_ context.Context, versionID string) ([]File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []File{}
	for _, f := range s.files[versionID] {
		out = append(out, *f)
	}
	return out, nil
}

func (s *MemoryStore) ListLoaders(_ context.Context) ([]string, error) {
	return sortedKeys(s.loadersSet), nil
}
func (s *MemoryStore) ListGameVersions(_ context.Context) ([]string, error) {
	return sortedKeys(s.gameVersSet), nil
}
func (s *MemoryStore) ListCategories(_ context.Context) ([]string, error) {
	return sortedKeys(s.categoriesSet), nil
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
