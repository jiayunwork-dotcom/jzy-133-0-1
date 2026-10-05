package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"spcapp/internal/repo"
	"spcapp/internal/service"
)

func parseID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "监控对象 id 无效")
	}
	return id, nil
}

// errorHandler 把服务层的字段错误/未找到翻译成结构化响应，前端可直接展示。
func errorHandler(err error, c echo.Context) {
	var ves service.ValidationErrors
	switch {
	case errors.As(err, &ves):
		_ = c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "校验失败",
			"fields":  []service.FieldError(ves),
			"message": ves.Error(),
		})
		return
	case errors.Is(err, repo.ErrNotFound):
		_ = c.JSON(http.StatusNotFound, map[string]any{"error": "监控对象不存在"})
		return
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		_ = c.JSON(he.Code, map[string]any{"error": he.Message})
		return
	}
	_ = c.JSON(http.StatusInternalServerError, map[string]any{"error": "服务器内部错误"})
}

func (h *Handler) listTargets(c echo.Context) error {
	ts, err := h.svc.ListTargets(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"targets": ts})
}

func (h *Handler) createTarget(c echo.Context) error {
	var in service.TargetInput
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求体不是合法 JSON")
	}
	t, err := h.svc.CreateTarget(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, t)
}

func (h *Handler) getTarget(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.svc.GetTarget(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, t)
}

func (h *Handler) updateTarget(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var in service.TargetInput
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求体不是合法 JSON")
	}
	t, err := h.svc.UpdateTarget(c.Request().Context(), id, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, t)
}

func (h *Handler) getChart(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	chart, err := h.svc.GetChart(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, chart)
}

func (h *Handler) rebaseline(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var in service.RebaselineInput
	// 允许空 body：默认取当前全部子组。
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&in); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "请求体不是合法 JSON")
		}
	}
	b, err := h.svc.Rebaseline(c.Request().Context(), id, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, b)
}
