// Package api 提供 Echo HTTP 处理器与路由装配。
package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"spcapp/internal/service"
)

// Handler 持有业务服务。
type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// NewEcho 构建 Echo 实例：CORS 放开给 Vite 开发服务器与同源部署。
func (h *Handler) NewEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))

	e.HTTPErrorHandler = errorHandler

	api := e.Group("/api")
	api.GET("/targets", h.listTargets)
	api.POST("/targets", h.createTarget)
	api.GET("/targets/:id", h.getTarget)
	api.PUT("/targets/:id", h.updateTarget)
	api.GET("/targets/:id/chart", h.getChart)
	api.POST("/targets/:id/measurements", h.appendValues)
	api.POST("/targets/:id/rebaseline", h.rebaseline)

	e.GET("/api/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	return e
}
