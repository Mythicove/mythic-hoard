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
	// ErrForbidden 表示已登录但无权操作该资源。
	ErrForbidden = errors.New("forbidden")
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

// TeamMember 是团队成员（username 为投影联查结果）。
type TeamMember struct {
	TeamID    string `json:"team_id"`
	UserSub   string `json:"user_sub"`
	Username  string `json:"username"`
	Role      string `json:"role"` // owner | member
	CreatedAt string `json:"created_at"`
}

// UserTeam 是用户视角的团队摘要。
type UserTeam struct {
	TeamID    string `json:"team_id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// TeamInvite 是团队管理者视角的待处理邀请（username 为投影联查结果）。
type TeamInvite struct {
	TeamID    string `json:"team_id"`
	UserSub   string `json:"user_sub"`
	Username  string `json:"username"`
	Role      string `json:"role"` // owner | member（接受后写入 team_members）
	InvitedBy string `json:"invited_by"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

// UserInvite 是被邀请人视角的待处理邀请（带团队名，供"我的邀请"列表）。
type UserInvite struct {
	TeamID    string `json:"team_id"`
	TeamName  string `json:"team_name"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

// PAT 是个人访问令牌元数据（永不含明文）。
type PAT struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	LastUsed  string   `json:"last_used,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// PATSession 是 PAT 校验结果（持有人 + scopes）。
type PATSession struct {
	User   User
	PATID  string
	Scopes []string
}

// NewDependency 是一条待存的版本依赖。
type NewDependency struct {
	DependencyType string // required | optional | incompatible | embedded
	ProjectID      string // 目标项目（可空）
	VersionID      string // 目标版本（可空）
	FileName       string // embedded 的展示文件名（可空）
}

// Dependency 是版本依赖记录（Modrinth GET /v2/project/{id}/dependencies 形）。
type Dependency struct {
	ID             string `json:"id"`
	SourceVersion  string `json:"-"`
	VersionID      string `json:"version_id,omitempty"` // 目标版本
	ProjectID      string `json:"project_id,omitempty"` // 目标项目
	DependencyType string `json:"dependency_type"`
	FileName       string `json:"file_name,omitempty"`
}

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
	TeamID       string   `json:"team"`
	License      string   `json:"license,omitempty"`
	IconURL      string   `json:"icon_url,omitempty"`
	IconKey      string   `json:"-"`
	MetaFlags    []string `json:"metadata_flags,omitempty"`
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
	License      string
	MetaFlags    []string
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

	// 版本依赖
	AddVersionDependencies(ctx context.Context, versionID string, deps []NewDependency) error
	ListVersionDependencies(ctx context.Context, versionID string) ([]Dependency, error)
	ListProjectDependencies(ctx context.Context, projectID string) ([]Dependency, error)

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

	// 团队（owner 管成员/删项目，member 传版本）
	CreateTeam(ctx context.Context, name, ownerSub string) (teamID string, err error)
	AddTeamMember(ctx context.Context, teamID, userSub, role string) error
	RemoveTeamMember(ctx context.Context, teamID, userSub string) error
	GetTeamMember(ctx context.Context, teamID, userSub string) (*TeamMember, error)
	ListTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error)
	ListUserTeams(ctx context.Context, userSub string) ([]UserTeam, error)
	// 团队邀请：加入需被邀请人同意。行存在即 pending，接受/拒绝/撤销都删除该行；
	// 过期时间由调用方给出（TTL 只在 Service 定义一处），过期的邀请在所有读取路径上视同不存在。
	UpsertTeamInvite(ctx context.Context, teamID, userSub, role, invitedBy string, expiresAt time.Time) error
	DeleteTeamInvite(ctx context.Context, teamID, userSub string) error
	GetTeamInvite(ctx context.Context, teamID, userSub string) (*TeamInvite, error)
	ListTeamInvites(ctx context.Context, teamID string) ([]TeamInvite, error)
	ListInvitesForUser(ctx context.Context, userSub string) ([]UserInvite, error)
	SetProjectTeam(ctx context.Context, projectID, teamID string) error
	ProjectTeam(ctx context.Context, projectID string) (string, error)
	SetProjectIcon(ctx context.Context, projectID, iconKey string) error

	// 个人访问令牌（tokenHash 为 sha256 hex；明文只在创建时返回一次）
	CreatePAT(ctx context.Context, userSub, name, tokenHash string, scopes []string, expires *time.Time) (id string, err error)
	ListPATs(ctx context.Context, userSub string) ([]PAT, error)
	ValidatePAT(ctx context.Context, tokenHash string) (*PATSession, error)
	DeletePAT(ctx context.Context, userSub, id string) error
}
