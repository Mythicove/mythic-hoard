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

func toProject(id, slug, title, description, body, projectType, clientSide, serverSide, status string, downloads, follows int64, loaders, gameVersions, categories []string, publishedAt pgtype.Timestamptz) *hoard.Project {
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
		Downloads:    downloads,
		Follows:      follows,
		Published:    ts(publishedAt),
		Loaders:      loaders,
		GameVersions: gameVersions,
		Categories:   categories,
	}
}

func projectFromRow(r db.CreateProjectRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromGetRow(r db.GetProjectByIDOrSlugRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromIDsRow(r db.GetProjectsByIDsRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
}

func projectFromSearchRow(r db.SearchProjectsRow) *hoard.Project {
	return toProject(r.ID, r.Slug, r.Title, r.Description, r.Body, r.ProjectType, r.ClientSide, r.ServerSide, r.Status, r.Downloads, r.Follows, r.Loaders, r.GameVersions, r.Categories, r.PublishedAt)
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
