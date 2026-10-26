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
	users         map[string]*User
	sessions      map[string]memSession
	teams         map[string]*memTeam
	pats          map[string]*memPAT
	deps          map[string][]Dependency // version_id -> dependencies
}

type memTeam struct {
	name      string
	createdAt time.Time
	members   map[string]*TeamMember // user_sub -> member
	invites   map[string]*TeamInvite // user_sub -> 待处理邀请
}

type memPAT struct {
	id       string
	userSub  string
	name     string
	hash     string
	scopes   []string
	expires  *time.Time
	lastUsed time.Time
	created  time.Time
}

type memSession struct {
	userSub string
	expires time.Time
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
		users:         map[string]*User{},
		sessions:      map[string]memSession{},
		teams:         map[string]*memTeam{},
		pats:          map[string]*memPAT{},
		deps:          map[string][]Dependency{},
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
		License:      p.License,
		MetaFlags:    p.MetaFlags,
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

// 身份投影与会话（内存实现）。

func (s *MemoryStore) UpsertUser(_ context.Context, u User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.users[u.Sub]; ok {
		existing.Username = u.Username
		existing.Email = u.Email
		existing.AvatarURL = u.AvatarURL
		return nil // role 保留现值
	}
	cp := u
	s.users[u.Sub] = &cp
	return nil
}

func (s *MemoryStore) UserBySub(_ context.Context, sub string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[sub]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (s *MemoryStore) SetUserRole(_ context.Context, sub, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[sub]
	if !ok {
		return ErrNotFound
	}
	u.Role = role
	return nil
}

func (s *MemoryStore) CreateSession(_ context.Context, id, userSub string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userSub]; !ok {
		return ErrNotFound
	}
	s.sessions[id] = memSession{userSub: userSub, expires: expires}
	return nil
}

func (s *MemoryStore) SessionUser(_ context.Context, sessionID string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok || time.Now().After(sess.expires) {
		return nil, ErrNotFound
	}
	u, ok := s.users[sess.userSub]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// 团队与 PAT（内存实现）。

func (s *MemoryStore) CreateTeam(_ context.Context, name, ownerSub string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[ownerSub]; !ok {
		return "", ErrNotFound
	}
	id := s.nextID("team")
	now := time.Now().UTC()
	s.teams[id] = &memTeam{
		name: name, createdAt: now,
		members: map[string]*TeamMember{ownerSub: {
			TeamID: id, UserSub: ownerSub, Username: s.users[ownerSub].Username,
			Role: "owner", CreatedAt: now.Format(time.RFC3339),
		}},
		invites: map[string]*TeamInvite{},
	}
	return id, nil
}

// 团队邀请（内存实现）：行存在即 pending，接受/拒绝/撤销都删除。

func (s *MemoryStore) UpsertTeamInvite(_ context.Context, teamID, userSub, role, invitedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return ErrNotFound
	}
	u, ok := s.users[userSub]
	if !ok {
		return ErrNotFound
	}
	if team.invites == nil {
		team.invites = map[string]*TeamInvite{}
	}
	team.invites[userSub] = &TeamInvite{
		TeamID: teamID, UserSub: userSub, Username: u.Username,
		Role: role, InvitedBy: invitedBy, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return nil
}

func (s *MemoryStore) DeleteTeamInvite(_ context.Context, teamID, userSub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := team.invites[userSub]; !ok {
		return ErrNotFound
	}
	delete(team.invites, userSub)
	return nil
}

func (s *MemoryStore) GetTeamInvite(_ context.Context, teamID, userSub string) (*TeamInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[teamID]
	if !ok {
		return nil, ErrNotFound
	}
	inv, ok := team.invites[userSub]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *inv
	return &cp, nil
}

func (s *MemoryStore) ListTeamInvites(_ context.Context, teamID string) ([]TeamInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[teamID]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]TeamInvite, 0, len(team.invites))
	for _, inv := range team.invites {
		out = append(out, *inv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (s *MemoryStore) ListInvitesForUser(_ context.Context, userSub string) ([]UserInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []UserInvite
	for id, team := range s.teams {
		if inv, ok := team.invites[userSub]; ok {
			out = append(out, UserInvite{
				TeamID: id, TeamName: team.name,
				Role: inv.Role, InvitedBy: inv.InvitedBy, CreatedAt: inv.CreatedAt,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (s *MemoryStore) AddTeamMember(_ context.Context, teamID, userSub, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return ErrNotFound
	}
	u, ok := s.users[userSub]
	if !ok {
		return ErrNotFound
	}
	team.members[userSub] = &TeamMember{
		TeamID: teamID, UserSub: userSub, Username: u.Username,
		Role: role, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return nil
}

func (s *MemoryStore) RemoveTeamMember(_ context.Context, teamID, userSub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	team, ok := s.teams[teamID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := team.members[userSub]; !ok {
		return ErrNotFound
	}
	delete(team.members, userSub)
	return nil
}

func (s *MemoryStore) GetTeamMember(_ context.Context, teamID, userSub string) (*TeamMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[teamID]
	if !ok {
		return nil, ErrNotFound
	}
	m, ok := team.members[userSub]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (s *MemoryStore) ListTeamMembers(_ context.Context, teamID string) ([]TeamMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	team, ok := s.teams[teamID]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]TeamMember, 0, len(team.members))
	for _, m := range team.members {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (s *MemoryStore) ListUserTeams(_ context.Context, userSub string) ([]UserTeam, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []UserTeam{}
	for id, team := range s.teams {
		if m, ok := team.members[userSub]; ok {
			out = append(out, UserTeam{TeamID: id, Name: team.name, Role: m.Role, CreatedAt: m.CreatedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (s *MemoryStore) SetProjectTeam(_ context.Context, projectID, teamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok {
		return ErrNotFound
	}
	p.TeamID = teamID
	return nil
}

func (s *MemoryStore) ProjectTeam(_ context.Context, projectID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[projectID]
	if !ok {
		return "", ErrNotFound
	}
	return p.TeamID, nil
}

func (s *MemoryStore) SetProjectIcon(_ context.Context, projectID, iconKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok {
		return ErrNotFound
	}
	p.IconKey = iconKey
	return nil
}

func (s *MemoryStore) CreatePAT(_ context.Context, userSub, name, tokenHash string, scopes []string, expires *time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userSub]; !ok {
		return "", ErrNotFound
	}
	id := s.nextID("pat")
	s.pats[id] = &memPAT{
		id: id, userSub: userSub, name: name, hash: tokenHash,
		scopes: scopes, expires: expires, created: time.Now().UTC(),
	}
	return id, nil
}

func (s *MemoryStore) ListPATs(_ context.Context, userSub string) ([]PAT, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []PAT{}
	for _, p := range s.pats {
		if p.userSub != userSub {
			continue
		}
		item := PAT{ID: p.id, Name: p.name, Scopes: p.scopes, CreatedAt: p.created.Format(time.RFC3339)}
		if p.expires != nil {
			item.ExpiresAt = p.expires.Format(time.RFC3339)
		}
		if !p.lastUsed.IsZero() {
			item.LastUsed = p.lastUsed.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *MemoryStore) ValidatePAT(_ context.Context, tokenHash string) (*PATSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pats {
		if p.hash != tokenHash {
			continue
		}
		if p.expires != nil && time.Now().After(*p.expires) {
			return nil, ErrNotFound
		}
		u, ok := s.users[p.userSub]
		if !ok {
			return nil, ErrNotFound
		}
		p.lastUsed = time.Now().UTC()
		return &PATSession{User: *u, PATID: p.id, Scopes: p.scopes}, nil
	}
	return nil, ErrNotFound
}

func (s *MemoryStore) DeletePAT(_ context.Context, userSub, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pats[id]
	if !ok || p.userSub != userSub {
		return ErrNotFound
	}
	delete(s.pats, id)
	return nil
}

// 版本依赖（内存实现）。

func (s *MemoryStore) AddVersionDependencies(_ context.Context, versionID string, deps []NewDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.versions[versionID]; !ok {
		return ErrNotFound
	}
	for _, d := range deps {
		s.deps[versionID] = append(s.deps[versionID], Dependency{
			ID: s.nextID("dep"), VersionID: d.VersionID, ProjectID: d.ProjectID,
			DependencyType: d.DependencyType, FileName: d.FileName,
		})
	}
	return nil
}

func (s *MemoryStore) ListVersionDependencies(_ context.Context, versionID string) ([]Dependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Dependency, 0, len(s.deps[versionID]))
	for _, d := range s.deps[versionID] {
		d.SourceVersion = versionID
		out = append(out, d)
	}
	return out, nil
}

func (s *MemoryStore) ListProjectDependencies(_ context.Context, projectID string) ([]Dependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Dependency{}
	for vid, deps := range s.deps {
		if v, ok := s.versions[vid]; ok && v.ProjectID == projectID {
			for _, d := range deps {
				d.SourceVersion = vid
				out = append(out, d)
			}
		}
	}
	return out, nil
}
