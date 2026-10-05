// Package config 从环境变量读取配置，提供 docker-compose 可用的默认值。
package config

import (
	"os"
)

// Config 后端运行配置。
type Config struct {
	Addr        string // HTTP 监听地址
	DatabaseURL string // PostgreSQL 连接串
}

// Load 读取环境变量，未设置时使用开发默认值。
func Load() Config {
	return Config{
		Addr:        getenv("SPC_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://spc:spc@localhost:5432/spc?sslmode=disable"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
