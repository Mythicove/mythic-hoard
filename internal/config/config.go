// Package config 从环境变量加载配置（caarlos0/env）。
package config

import "github.com/caarlos0/env/v11"

// Config 是 Hoard 服务配置。
type Config struct {
	ListenAddr  string `env:"HOARD_LISTEN_ADDR" envDefault:":8081"`
	BaseURL     string `env:"HOARD_BASE_URL" envDefault:"http://localhost:8080"`
	AdminToken  string `env:"HOARD_ADMIN_TOKEN"`
	DataDir     string `env:"HOARD_DATA_DIR" envDefault:"data/files"`
	DatabaseURL string `env:"HOARD_DATABASE_URL"`

	// MaxFileBytes 是单个 mod 文件大小上限（默认 512 MiB）。
	MaxFileBytes int64 `env:"HOARD_MAX_FILE_BYTES" envDefault:"536870912"`

	// 存储：local / s3
	StorageDriver string `env:"HOARD_STORAGE_DRIVER" envDefault:"local"`
	S3Endpoint    string `env:"HOARD_S3_ENDPOINT"`
	S3Region      string `env:"HOARD_S3_REGION" envDefault:"auto"`
	S3Bucket      string `env:"HOARD_S3_BUCKET"`
	S3AccessKey   string `env:"HOARD_S3_ACCESS_KEY"`
	S3SecretKey   string `env:"HOARD_S3_SECRET_KEY"`

	// 搜索：pgtrgm / meili
	SearchDriver string `env:"HOARD_SEARCH_DRIVER" envDefault:"pgtrgm"`
	MeiliURL     string `env:"HOARD_MEILI_URL"`
	MeiliAPIKey  string `env:"HOARD_MEILI_API_KEY"`

	// 队列：inline / asynq
	QueueDriver string `env:"HOARD_QUEUE_DRIVER" envDefault:"inline"`
	RedisAddr   string `env:"HOARD_REDIS_ADDR"`

	RateLimitRPS   float64 `env:"HOARD_RATE_LIMIT_RPS" envDefault:"100"`
	RateLimitBurst int     `env:"HOARD_RATE_LIMIT_BURST" envDefault:"200"`

	// CORS 允许来源（逗号分隔）；留空回退 "*"（仅建议开发环境）。
	CORSOrigins []string `env:"HOARD_CORS_ORIGINS" envSeparator:","`

	// 客户端 IP 解析：转发头顺序与可信代理网段（见 shared/clientip）。
	// 留空 = 默认头序（XFF→X-Real-IP→CF-Connecting-IP→True-Client-IP）+ 默认信任私网/环回代理。
	ClientIPHeaders   []string `env:"HOARD_CLIENT_IP_HEADERS" envSeparator:","`
	TrustedProxies    []string `env:"HOARD_TRUSTED_PROXIES" envSeparator:","`
	ClientIPTrustNone bool     `env:"HOARD_CLIENT_IP_TRUST_NONE"`

	// OIDC RP（Mirror 为 IdP）；Issuer 留空则禁用 SSO 登录。
	OIDCIssuer       string `env:"HOARD_OIDC_ISSUER"`
	OIDCClientID     string `env:"HOARD_OIDC_CLIENT_ID"`
	OIDCClientSecret string `env:"HOARD_OIDC_CLIENT_SECRET"`
	// 回调地址；留空则取 BaseURL + /auth/oidc/callback。
	OIDCRedirectURL string `env:"HOARD_OIDC_REDIRECT_URL"`
	// 管理员 subject 白名单（逗号分隔的 Mirror 用户 ID）；命中者登录即映射 role=admin。
	AdminSubjects []string `env:"HOARD_ADMIN_SUBJECTS" envSeparator:","`
}

// Load 从环境变量加载配置。
func Load() *Config {
	cfg := &Config{}
	_ = env.Parse(cfg)
	return cfg
}
