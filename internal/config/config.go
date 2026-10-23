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
}

// Load 从环境变量加载配置。
func Load() *Config {
	cfg := &Config{}
	_ = env.Parse(cfg)
	return cfg
}
