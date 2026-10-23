// Package hoard 提供 mod/整合包分发后端（Modrinth 兼容读 + .mrpack）。
package hoard

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound 表示资源不存在。
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists 表示资源已存在（slug 冲突等）。
	ErrAlreadyExists = errors.New("already exists")
	// ErrInvalid 表示输入非法。
	ErrInvalid = errors.New("invalid input")
	// ErrFileTooLarge 表示上传文件超出大小上限。
	ErrFileTooLarge = errors.New("file too large")
)

// User 是 OIDC 用户投影（sub = Mirror users.id，不建跨服务外键）。
type User struct {
	Sub       string `json:"sub"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"` // user | admin
}

// IsAdmin 返回是否管理员。
func (u *User) IsAdmin() bool { return u != nil && u.Role == "admin" }

// Project 对应 Modrinth 的 project。
type Project struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Body         string   `json:"body"`
	ProjectType  string   `json:"project_type"`
	ClientSide   string   `json:"client_side"`
	ServerSide   string   `json:"server_side"`
	Status       string   `json:"status"`
	Downloads    int64    `json:"downloads"`
	Follows      int64    `json:"follows"`
	Published    string   `json:"published"`
	Loaders      []string `json:"loaders,omitempty"`
	GameVersions []string `json:"game_versions,omitempty"`
	Categories   []string `json:"categories,omitempty"`
}

// Version 对应 Modrinth 的 version。
type Version struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	VersionType   string   `json:"version_type"`
	Changelog     string   `json:"changelog"`
	Loaders       []string `json:"loaders,omitempty"`
	GameVersions  []string `json:"game_versions,omitempty"`
	DatePublished string   `json:"date_published"`
	Files         []File   `json:"files"`
}

// File 对应 Modrinth 的 version file。
type File struct {
	ID         string            `json:"-"`
	StorageKey string            `json:"-"`
	Hashes     map[string]string `json:"hashes"`
	URL        string            `json:"url"`
	Filename   string            `json:"filename"`
	Primary    bool              `json:"primary"`
	Size       int64             `json:"size"`
}

// NewProject 是新建项目的输入。
type NewProject struct {
	Slug         string
	Title        string
	Description  string
	Body         string
	ProjectType  string
	ClientSide   string
	ServerSide   string
	Status       string
	Loaders      []string
	GameVersions []string
	Categories   []string
}

// NewVersion 是新建版本的输入。
type NewVersion struct {
	Name          string
	VersionNumber string
	VersionType   string
	Changelog     string
	Loaders       []string
	GameVersions  []string
}

// NewFile 是新建文件的输入。
type NewFile struct {
	Filename   string
	Size       int64
	SHA1       string
	SHA512     string
	StorageKey string
	Primary    bool
}

// Filter 是列表/搜索的过滤条件。
type Filter struct {
	Loaders      []string
	GameVersions []string
	ProjectTypes []string
	Categories   []string
	Status       string // 空 = 不过滤；"listed" 只返回公开项目
}

// Store 是项目/版本/文件元数据的持久化抽象。
type Store interface {
	CreateProject(ctx context.Context, p NewProject) (*Project, error)
	GetProject(ctx context.Context, idOrSlug string) (*Project, error)
	GetProjectsByIDs(ctx context.Context, ids []string) ([]Project, error)
	Search(ctx context.Context, q string, f Filter, limit, offset int) ([]Project, int64, error)
	UpdateProjectStatus(ctx context.Context, id, status string) error
	DeleteProject(ctx context.Context, id string) error
	IncrementDownloads(ctx context.Context, projectID string) error

	CreateVersion(ctx context.Context, projectID string, v NewVersion) (*Version, error)
	ListVersions(ctx context.Context, projectID string) ([]Version, error)
	GetVersion(ctx context.Context, id string) (*Version, error)

	CreateFile(ctx context.Context, versionID string, f NewFile) (*File, error)
	ListFiles(ctx context.Context, versionID string) ([]File, error)

	ListLoaders(ctx context.Context) ([]string, error)
	ListGameVersions(ctx context.Context) ([]string, error)
	ListCategories(ctx context.Context) ([]string, error)

	// 身份（OIDC 投影与会话）
	UpsertUser(ctx context.Context, u User) error
	UserBySub(ctx context.Context, sub string) (*User, error)
	SetUserRole(ctx context.Context, sub, role string) error
	CreateSession(ctx context.Context, id, userSub string, expires time.Time) error
	SessionUser(ctx context.Context, sessionID string) (*User, error)
	DeleteSession(ctx context.Context, id string) error
}
