// Command server 启动 SPC 后端：连库、迁移、挂 Echo。
package main

import (
	"context"
	"log"
	"time"

	"spcapp/internal/api"
	"spcapp/internal/config"
	"spcapp/internal/db"
	"spcapp/internal/repo"
	"spcapp/internal/service"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer pool.Close()

	r := repo.New(pool)
	svc := service.New(r)
	h := api.NewHandler(svc)

	e := h.NewEcho()
	log.Printf("SPC 后端监听 %s", cfg.Addr)
	if err := e.Start(cfg.Addr); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}
