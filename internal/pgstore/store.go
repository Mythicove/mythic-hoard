// Package pgstore 提供 Postgres 实现的 hoard.Store（sqlc 生成的类型安全查询）。
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/Mythicove/mythic-hoard/internal/hoard"
	db "github.com/Mythicove/mythic-hoard/internal/pgstore/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 是 Postgres 实现的 hoard.Store。
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

var _ hoard.Store = (*Store)(nil)

// New 连接并 ping 数据库。
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

// Close 关闭连接池。
func (s *Store) Close() { s.pool.Close() }

func (s *Store) CreateProject(ctx context.Context, p hoard.NewProject) (*hoard.Project, error) {
	if err := s.ensureLookups(ctx, p.Loaders, p.GameVersions, p.Categories); err != nil {
		return nil, err
	}
	row, err := s.q.CreateProject(ctx, db.CreateProjectParams{
		Slug:         p.Slug,
		Title:        p.Title,
		Description:  p.Description,
		Body:         p.Body,
		ProjectType:  p.ProjectType,
		ClientSide:   p.ClientSide,
		ServerSide:   p.ServerSide,
		Status:       p.Status,
		Loaders:      nonNil(p.Loaders),
		GameVersions: nonNil(p.GameVersions),
		Categories:   nonNil(p.Categories),
		License:      p.License,
		MetaFlags:    nonNil(p.MetaFlags),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, hoard.ErrAlreadyExists
		}
		return nil, err
	}
	return projectFromRow(row), nil
}

func (s *Store) GetProject(ctx context.Context, idOrSlug string) (*hoard.Project, error) {
	row, err := s.q.GetProjectByIDOrSlug(ctx, idOrSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return projectFromGetRow(row), nil
}

func (s *Store) GetProjectsByIDs(ctx context.Context, ids []string) ([]hoard.Project, error) {
	rows, err := s.q.GetProjectsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, *projectFromIDsRow(r))
	}
	return out, nil
}

func (s *Store) Search(ctx context.Context, q string, f hoard.Filter, limit, offset int) ([]hoard.Project, int64, error) {
	params := db.SearchProjectsParams{
		Column1: q,
		Column2: nonNil(f.Loaders),
		Column3: nonNil(f.GameVersions),
		Column4: nonNil(f.ProjectTypes),
		Column5: nonNil(f.Categories),
		Column6: f.Status,
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	count, err := s.q.CountSearchProjects(ctx, db.CountSearchProjectsParams{
		Column1: q, Column2: nonNil(f.Loaders), Column3: nonNil(f.GameVersions),
		Column4: nonNil(f.ProjectTypes), Column5: nonNil(f.Categories), Column6: f.Status,
	})
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.SearchProjects(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	out := make([]hoard.Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, *projectFromSearchRow(r))
	}
	return out, count, nil
}

func (s *Store) UpdateProjectStatus(ctx context.Context, id, status string) error {
	return s.q.UpdateProjectStatus(ctx, db.UpdateProjectStatusParams{ID: id, Status: status})
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	n, err := s.q.DeleteProject(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) IncrementDownloads(ctx context.Context, projectID string) error {
	return s.q.IncrementDownloads(ctx, projectID)
}

func (s *Store) CreateVersion(ctx context.Context, projectID string, v hoard.NewVersion) (*hoard.Version, error) {
	if err := s.ensureLookups(ctx, v.Loaders, v.GameVersions, nil); err != nil {
		return nil, err
	}
	row, err := s.q.CreateVersion(ctx, db.CreateVersionParams{
		ProjectID:     projectID,
		Name:          v.Name,
		VersionNumber: v.VersionNumber,
		VersionType:   v.VersionType,
		Changelog:     v.Changelog,
		Loaders:       nonNil(v.Loaders),
		GameVersions:  nonNil(v.GameVersions),
	})
	if err != nil {
		return nil, err
	}
	return versionFromRow(row), nil
}

func (s *Store) ListVersions(ctx context.Context, projectID string) ([]hoard.Version, error) {
	rows, err := s.q.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.Version, 0, len(rows))
	for _, r := range rows {
		out = append(out, *versionFromListRow(r))
	}
	return out, nil
}

func (s *Store) GetVersion(ctx context.Context, id string) (*hoard.Version, error) {
	row, err := s.q.GetVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return versionFromGetRow(row), nil
}

func (s *Store) CreateFile(ctx context.Context, versionID string, f hoard.NewFile) (*hoard.File, error) {
	row, err := s.q.CreateFile(ctx, db.CreateFileParams{
		VersionID:  versionID,
		Filename:   f.Filename,
		Size:       f.Size,
		Sha1:       f.SHA1,
		Sha512:     f.SHA512,
		StorageKey: f.StorageKey,
		IsPrimary:  f.Primary,
	})
	if err != nil {
		return nil, err
	}
	return &hoard.File{
		ID:         row.ID,
		StorageKey: row.StorageKey,
		Hashes:     map[string]string{"sha1": row.Sha1, "sha512": row.Sha512},
		Filename:   row.Filename,
		Primary:    row.IsPrimary,
		Size:       row.Size,
	}, nil
}

func (s *Store) ListFiles(ctx context.Context, versionID string) ([]hoard.File, error) {
	rows, err := s.q.ListFiles(ctx, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.File, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.File{
			ID:         r.ID,
			StorageKey: r.StorageKey,
			Hashes:     map[string]string{"sha1": r.Sha1, "sha512": r.Sha512},
			Filename:   r.Filename,
			Primary:    r.IsPrimary,
			Size:       r.Size,
		})
	}
	return out, nil
}

func (s *Store) ListLoaders(ctx context.Context) ([]string, error) {
	return s.q.ListLoaders(ctx)
}

func (s *Store) ListGameVersions(ctx context.Context) ([]string, error) {
	return s.q.ListGameVersions(ctx)
}

func (s *Store) ListCategories(ctx context.Context) ([]string, error) {
	return s.q.ListCategories(ctx)
}

func (s *Store) ensureLookups(ctx context.Context, loaders, gameVersions, categories []string) error {
	for _, l := range loaders {
		if err := s.q.UpsertLoader(ctx, l); err != nil {
			return err
		}
	}
	for _, g := range gameVersions {
		if err := s.q.UpsertGameVersion(ctx, g); err != nil {
			return err
		}
	}
	for _, c := range categories {
		if err := s.q.UpsertCategory(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ts(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

func toProject(id, slug, title, description, body, projectType, clientSide, serverSide, status, teamID, license, iconKey string, metaFlags []string, downloads, follows int64, loaders, gameVersions, categories []string, publishedAt pgtype.Timestamptz) *hoard.Project {
	return &hoard.Project{
		ID:           id,
		Slug:         slug,
		Title:        title,
		Description:  description,
		Body:         body,
		ProjectType:  projectType,
		ClientSide:   clientSide,
		ServerSide:   serverSide,
		Status:       status,
		TeamID:       teamID,
		License:      license,
		IconKey:      iconKey,
		MetaFlags:    metaFlags,
		Downloads:    downloads,
		Follows:      follows,
		Published:    ts(publishedAt),
		Loaders:      loaders,
		GameVersions: gameVersions,
		Categories:   categories,
	}
}

func projectFromRow(r db.CreateProjectRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.TeamID.String, r.License, r.IconKey, r.MetaFlags, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromGetRow(r db.GetProjectByIDOrSlugRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.TeamID.String, r.License, r.IconKey, r.MetaFlags, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromIDsRow(r db.GetProjectsByIDsRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.TeamID.String, r.License, r.IconKey, r.MetaFlags, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromSearchRow(r db.SearchProjectsRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.TeamID.String, r.License, r.IconKey, r.MetaFlags, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func versionFromRow(r db.CreateVersionRow) *hoard.Version {
	return &hoard.Version{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		VersionNumber: r.VersionNumber,
		VersionType:   r.VersionType,
		Changelog:     r.Changelog,
		Loaders:       r.Loaders,
		GameVersions:  r.GameVersions,
		DatePublished: ts(r.DatePublished),
	}
}

func versionFromListRow(r db.ListVersionsRow) *hoard.Version {
	return &hoard.Version{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		VersionNumber: r.VersionNumber,
		VersionType:   r.VersionType,
		Changelog:     r.Changelog,
		Loaders:       r.Loaders,
		GameVersions:  r.GameVersions,
		DatePublished: ts(r.DatePublished),
	}
}

func versionFromGetRow(r db.GetVersionRow) *hoard.Version {
	return &hoard.Version{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		VersionNumber: r.VersionNumber,
		VersionType:   r.VersionType,
		Changelog:     r.Changelog,
		Loaders:       r.Loaders,
		GameVersions:  r.GameVersions,
		DatePublished: ts(r.DatePublished),
	}
}

// 身份投影与会话。

func (s *Store) UpsertUser(ctx context.Context, u hoard.User) error {
	return s.q.UpsertUser(ctx, db.UpsertUserParams{
		Sub: u.Sub, Username: u.Username, Email: u.Email, AvatarUrl: u.AvatarURL, Role: u.Role,
	})
}

func (s *Store) UserBySub(ctx context.Context, sub string) (*hoard.User, error) {
	row, err := s.q.GetUserBySub(ctx, sub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &hoard.User{Sub: row.Sub, Username: row.Username, Email: row.Email, AvatarURL: row.AvatarUrl, Role: row.Role}, nil
}

func (s *Store) SetUserRole(ctx context.Context, sub, role string) error {
	n, err := s.q.SetUserRole(ctx, db.SetUserRoleParams{Sub: sub, Role: role})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, id, userSub string, expires time.Time) error {
	return s.q.CreateSession(ctx, db.CreateSessionParams{
		ID: id, UserSub: userSub, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
}

func (s *Store) SessionUser(ctx context.Context, sessionID string) (*hoard.User, error) {
	row, err := s.q.GetSession(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &hoard.User{Sub: row.UserSub, Username: row.Username, Email: row.Email, AvatarURL: row.AvatarUrl, Role: row.Role}, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.q.DeleteSession(ctx, id)
	return err
}

// 团队与个人访问令牌。

func (s *Store) CreateTeam(ctx context.Context, name, ownerSub string) (string, error) {
	id, err := s.q.CreateTeam(ctx, name)
	if err != nil {
		return "", err
	}
	if _, err := s.q.AddTeamMember(ctx, db.AddTeamMemberParams{TeamID: id, UserSub: ownerSub, Role: "owner"}); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) AddTeamMember(ctx context.Context, teamID, userSub, role string) error {
	_, err := s.q.AddTeamMember(ctx, db.AddTeamMemberParams{TeamID: teamID, UserSub: userSub, Role: role})
	return err
}
func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userSub string) error {
	n, err := s.q.RemoveTeamMember(ctx, db.RemoveTeamMemberParams{TeamID: teamID, UserSub: userSub})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) GetTeamMember(ctx context.Context, teamID, userSub string) (*hoard.TeamMember, error) {
	row, err := s.q.GetTeamMember(ctx, db.GetTeamMemberParams{TeamID: teamID, UserSub: userSub})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &hoard.TeamMember{
		TeamID: row.TeamID, UserSub: row.UserSub, Username: row.Username,
		Role: row.Role, CreatedAt: ts(row.CreatedAt),
	}, nil
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID string) ([]hoard.TeamMember, error) {
	rows, err := s.q.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.TeamMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.TeamMember{
			TeamID: r.TeamID, UserSub: r.UserSub, Username: r.Username,
			Role: r.Role, CreatedAt: ts(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *Store) ListUserTeams(ctx context.Context, userSub string) ([]hoard.UserTeam, error) {
	rows, err := s.q.ListUserTeams(ctx, userSub)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.UserTeam, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.UserTeam{TeamID: r.TeamID, Name: r.Name, Role: r.Role, CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (s *Store) UpsertTeamInvite(ctx context.Context, teamID, userSub, role, invitedBy string) error {
	_, err := s.q.UpsertTeamInvite(ctx, db.UpsertTeamInviteParams{
		TeamID: teamID, UserSub: userSub, Role: role, InvitedBy: invitedBy,
	})
	if err != nil {
		// 目标用户不存在（外键）→ 404，而不是把 DB 细节漏给调用方
		return err
	}
	return nil
}

func (s *Store) DeleteTeamInvite(ctx context.Context, teamID, userSub string) error {
	n, err := s.q.DeleteTeamInvite(ctx, db.DeleteTeamInviteParams{TeamID: teamID, UserSub: userSub})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) GetTeamInvite(ctx context.Context, teamID, userSub string) (*hoard.TeamInvite, error) {
	row, err := s.q.GetTeamInvite(ctx, db.GetTeamInviteParams{TeamID: teamID, UserSub: userSub})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &hoard.TeamInvite{
		TeamID: row.TeamID, UserSub: row.UserSub, Username: row.Username,
		Role: row.Role, InvitedBy: row.InvitedBy, CreatedAt: ts(row.CreatedAt),
	}, nil
}

func (s *Store) ListTeamInvites(ctx context.Context, teamID string) ([]hoard.TeamInvite, error) {
	rows, err := s.q.ListTeamInvites(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.TeamInvite, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.TeamInvite{
			TeamID: r.TeamID, UserSub: r.UserSub, Username: r.Username,
			Role: r.Role, InvitedBy: r.InvitedBy, CreatedAt: ts(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *Store) ListInvitesForUser(ctx context.Context, userSub string) ([]hoard.UserInvite, error) {
	rows, err := s.q.ListInvitesForUser(ctx, userSub)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.UserInvite, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.UserInvite{
			TeamID: r.TeamID, TeamName: r.TeamName,
			Role: r.Role, InvitedBy: r.InvitedBy, CreatedAt: ts(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *Store) SetProjectTeam(ctx context.Context, projectID, teamID string) error {
	n, err := s.q.SetProjectTeam(ctx, db.SetProjectTeamParams{ID: projectID, TeamID: pgtype.Text{String: teamID, Valid: teamID != ""}})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) ProjectTeam(ctx context.Context, projectID string) (string, error) {
	teamID, err := s.q.GetProjectTeam(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", hoard.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return teamID, nil
}

func (s *Store) SetProjectIcon(ctx context.Context, projectID, iconKey string) error {
	n, err := s.q.SetProjectIcon(ctx, db.SetProjectIconParams{ID: projectID, IconKey: iconKey})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

func (s *Store) CreatePAT(ctx context.Context, userSub, name, tokenHash string, scopes []string, expires *time.Time) (string, error) {
	var exp pgtype.Timestamptz
	if expires != nil {
		exp = pgtype.Timestamptz{Time: *expires, Valid: true}
	}
	return s.q.CreatePAT(ctx, db.CreatePATParams{
		UserSub: userSub, Name: name, TokenHash: tokenHash, Scopes: nonNil(scopes), ExpiresAt: exp,
	})
}

func (s *Store) ListPATs(ctx context.Context, userSub string) ([]hoard.PAT, error) {
	rows, err := s.q.ListPATs(ctx, userSub)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.PAT, 0, len(rows))
	for _, r := range rows {
		out = append(out, hoard.PAT{
			ID: r.ID, Name: r.Name, Scopes: r.Scopes,
			ExpiresAt: ts(r.ExpiresAt), LastUsed: ts(r.LastUsed), CreatedAt: ts(r.CreatedAt),
		})
	}
	return out, nil
}

func (s *Store) ValidatePAT(ctx context.Context, tokenHash string) (*hoard.PATSession, error) {
	row, err := s.q.GetPATByHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, hoard.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = s.q.TouchPAT(ctx, row.ID)
	return &hoard.PATSession{
		User:   hoard.User{Sub: row.UserSub, Username: row.Username, Role: row.Role},
		PATID:  row.ID,
		Scopes: row.Scopes,
	}, nil
}

func (s *Store) DeletePAT(ctx context.Context, userSub, id string) error {
	n, err := s.q.DeletePAT(ctx, db.DeletePATParams{ID: id, UserSub: userSub})
	if err != nil {
		return err
	}
	if n == 0 {
		return hoard.ErrNotFound
	}
	return nil
}

// 版本依赖。

func (s *Store) AddVersionDependencies(ctx context.Context, versionID string, deps []hoard.NewDependency) error {
	for _, d := range deps {
		if err := s.q.AddVersionDependency(ctx, db.AddVersionDependencyParams{
			VersionID: versionID, DependencyType: d.DependencyType,
			ProjectID: d.ProjectID, DepVersionID: d.VersionID, FileName: d.FileName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func depFromRow(id, versionID, depType, projectID, depVersionID, fileName string) hoard.Dependency {
	return hoard.Dependency{
		ID: id, SourceVersion: versionID, DependencyType: depType,
		ProjectID: projectID, VersionID: depVersionID, FileName: fileName,
	}
}

func (s *Store) ListVersionDependencies(ctx context.Context, versionID string) ([]hoard.Dependency, error) {
	rows, err := s.q.ListVersionDependencies(ctx, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.Dependency, 0, len(rows))
	for _, r := range rows {
		out = append(out, depFromRow(r.ID, r.VersionID, r.DependencyType, r.ProjectID, r.DepVersionID, r.FileName))
	}
	return out, nil
}

func (s *Store) ListProjectDependencies(ctx context.Context, projectID string) ([]hoard.Dependency, error) {
	rows, err := s.q.ListProjectDependencies(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]hoard.Dependency, 0, len(rows))
	for _, r := range rows {
		out = append(out, depFromRow(r.ID, r.VersionID, r.DependencyType, r.ProjectID, r.DepVersionID, r.FileName))
	}
	return out, nil
}
