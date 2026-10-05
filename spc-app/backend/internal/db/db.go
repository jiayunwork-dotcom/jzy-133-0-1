// Package db 负责连接池与启动时建表（嵌入式迁移）。
package db

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect 建立连接池，等待 PostgreSQL 就绪并执行迁移。
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("解析数据库连接串失败: %w", err)
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("建立连接池失败: %w", err)
	}
	if err := waitForReady(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// waitForReady 轮询 Ping，容忍 compose 中后端早于数据库就绪。
func waitForReady(ctx context.Context, pool *pgxpool.Pool) error {
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		if err := pool.Ping(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("数据库 30 秒内未就绪: %w", lastErr)
}

// Migrate 按文件名顺序执行 migrations 目录下的 SQL（全部幂等，IF NOT EXISTS）。
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("执行迁移 %s 失败: %w", e.Name(), err)
		}
	}
	return nil
}
