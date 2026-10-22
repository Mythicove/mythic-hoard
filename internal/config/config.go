// Package config 从环境变量加载配置。
package config

import "os"

// Config 是 Hoard 服务配置。
type Config struct {
	ListenAddr string
	AdminToken string
	DataDir    string
}

// Load 从环境变量读取配置。
func Load() *Config {
	return &Config{
		ListenAddr: getenv("HOARD_LISTEN_ADDR", ":8081"),
		AdminToken: getenv("HOARD_ADMIN_TOKEN", ""),
		DataDir:    getenv("HOARD_DATA_DIR", "data/files"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
