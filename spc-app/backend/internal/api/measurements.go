package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"spcapp/internal/service"
)

// appendRequest 录入入参。
//
//	values: 支持三种粘贴形态
//	        - 数字数组 [10.1, 10.2]
//	        - 字符串数组 ["10.1", "abc"]（非数字会被逐个定位报错）
//	        - 一整列文本 "10.1\n10.2 10.3"（空白/换行/逗号分隔，按子组容量自动分组）
//	operator: 检验员标识（可选）。
type appendRequest struct {
	Values   json.RawMessage `json:"values"`
	Text     string          `json:"text"`
	Operator string          `json:"operator"`
}

func (h *Handler) appendValues(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req appendRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求体不是合法 JSON")
	}

	values, err := decodeValues(req)
	if err != nil {
		return err
	}
	parsed, err := service.ParseNumbers(values)
	if err != nil {
		return err
	}
	res, err := h.svc.AppendValues(c.Request().Context(), id, parsed, req.Operator)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, res)
}

// decodeValues 把三种入参形态统一成 []any（数字或字符串）。
func decodeValues(req appendRequest) ([]any, error) {
	if len(req.Values) > 0 && string(req.Values) != "null" {
		var arr []any
		if err := json.Unmarshal(req.Values, &arr); err != nil {
			// 也许传了一个标量。
			var one any
			if e2 := json.Unmarshal(req.Values, &one); e2 != nil {
				return nil, echo.NewHTTPError(http.StatusBadRequest,
					"values 必须是数组或一整列数字文本")
			}
			arr = []any{one}
		}
		return arr, nil
	}
	if req.Text != "" {
		fields := splitColumn(req.Text)
		out := make([]any, len(fields))
		for i, f := range fields {
			out[i] = f
		}
		return out, nil
	}
	return nil, echo.NewHTTPError(http.StatusBadRequest, "请提供要录入的测量值（values 或 text）")
}

// splitColumn 按换行、逗号、空白（含制表/全角空格）切出一列 token。
func splitColumn(text string) []string {
	fields := make([]string, 0)
	cur := make([]rune, 0, 16)
	flush := func() {
		if len(cur) > 0 {
			fields = append(fields, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range text {
		switch r {
		case '\n', '\r', ',', ' ', '\t', ';', '，', '　':
			flush()
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return fields
}
