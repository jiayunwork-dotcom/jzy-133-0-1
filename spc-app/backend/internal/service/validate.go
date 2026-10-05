// Package service 编排校验、切组、控制限/能力计算、判异重放与持久化。
package service

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"spcapp/internal/rules"
	"spcapp/internal/spc"
)

// TargetInput 创建/更新档案的入参。
type TargetInput struct {
	Name         string          `json:"name"`
	Machine      string          `json:"machine"`
	Dimension    string          `json:"dimension"`
	SubgroupSize int             `json:"subgroupSize"`
	USL          *float64        `json:"usl"`
	LSL          *float64        `json:"lsl"`
	Rules        map[string]bool `json:"rules"`
}

// FieldError 标记具体字段的校验错误，前端可定位到表单项。
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// ValidationErrors 多个字段错误的集合。
type ValidationErrors []FieldError

func (v ValidationErrors) Error() string {
	msgs := make([]string, len(v))
	for i, e := range v {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "；")
}

// HasField 判断错误集合中是否含某字段（测试与接口层复用）。
func HasField(err error, field string) bool {
	var ves ValidationErrors
	if errors.As(err, &ves) {
		for _, e := range ves {
			if e.Field == field {
				return true
			}
		}
	}
	return false
}

// ValidateTarget 校验档案输入，返回标准化（补全开关系省值）后的规则集合。
func ValidateTarget(in TargetInput) (rules.EnabledSet, error) {
	var ves ValidationErrors
	if strings.TrimSpace(in.Name) == "" {
		ves = append(ves, FieldError{"name", "监控对象名称必填"})
	}
	if in.SubgroupSize < 2 || in.SubgroupSize > 10 {
		ves = append(ves, FieldError{"subgroupSize", "子组容量必须是 2 到 10 的整数"})
	}
	ves = append(ves, validateSpec(in.USL, in.LSL)...)
	enabled := rules.DefaultEnabled()
	for k, v := range in.Rules {
		if _, ok := validRuleIDs[rules.RuleID(k)]; !ok {
			ves = append(ves, FieldError{"rules", fmt.Sprintf("未知的判异规则: %s", k)})
			continue
		}
		enabled[rules.RuleID(k)] = v
	}
	if len(ves) > 0 {
		return nil, ves
	}
	return enabled, nil
}

var validRuleIDs = map[rules.RuleID]struct{}{
	rules.R1Beyond3Sigma:      {},
	rules.R2SameSide9:         {},
	rules.R3Trend6:            {},
	rules.R4TwoOfThreeBeyond2: {},
}

func validateSpec(usl, lsl *float64) ValidationErrors {
	var ves ValidationErrors
	if usl == nil && lsl == nil {
		ves = append(ves, FieldError{"usl", "规格上下限至少填写一个"})
		return ves
	}
	if usl != nil && math.IsNaN(*usl) {
		ves = append(ves, FieldError{"usl", "规格上限必须是数字"})
	}
	if lsl != nil && math.IsNaN(*lsl) {
		ves = append(ves, FieldError{"lsl", "规格下限必须是数字"})
	}
	if usl != nil && lsl != nil && *usl <= *lsl {
		ves = append(ves, FieldError{"usl", "规格上限必须大于规格下限"})
	}
	return ves
}

// CheckSpec 纯规格校验，供能力计算与测试复用。
func CheckSpec(usl, lsl *float64) error {
	s := spc.Spec{USL: usl, LSL: lsl}
	return spc.ValidateSpec(s)
}

// ParseNumeric 解析单个录入值：必须是有限实数，否则给出带位置的明确提示。
func ParseNumeric(s string, pos int) (float64, error) {
	s = strings.TrimSpace(s)
	var v float64
	if _, err := fmt.Sscan(s, &v); err != nil || s == "" {
		return 0, FieldError{"values", fmt.Sprintf("第 %d 个测量值不是数字: %q", pos, s)}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, FieldError{"values", fmt.Sprintf("第 %d 个测量值不是有限数: %q", pos, s)}
	}
	return v, nil
}

// CheckBatchSize 校验一次录入的数值数：允许单值或整组对齐。
func CheckBatchSize(subgroupSize, count int) error {
	if count <= 0 {
		return FieldError{"values", "没有可录入的测量值"}
	}
	if count != 1 && count%subgroupSize != 0 {
		return FieldError{"values",
			fmt.Sprintf("一次录入 %d 个值不是子组容量 %d 的整数倍；请逐个录入或粘贴整组数据",
				count, subgroupSize)}
	}
	return nil
}

// CheckBaselineRange 校验基准期范围：合法闭区间且至少 20 组。
func CheckBaselineRange(totalGroups, start, end int) error {
	var ves ValidationErrors
	if start < 1 || end < start || end > totalGroups {
		ves = append(ves, FieldError{"startGroup",
			"基准子组范围无效，当前共有完整子组 " + fmtInt(totalGroups) + " 个"})
	}
	if end-start+1 < 20 {
		ves = append(ves, FieldError{"endGroup", "基准期至少需要 20 个子组"})
	}
	if len(ves) > 0 {
		return ves
	}
	return nil
}

// fmtInt 不引 strconv 也行，但 strconv 更清晰；这里直接用标准库。
func fmtInt(n int) string {
	return strconv.Itoa(n)
}

// ParseNumbers 接受前端两种形态：纯数值数组或字符串数组。
// 只负责「逐个解析成数」；是否与子组容量对齐由 CheckBatchSize 统一判定。
// 第一个非法值即报错（标明位置），保证「测量值不是数」前后端都有明确提示。
func ParseNumbers(raw []any) ([]float64, error) {
	out := make([]float64, 0, len(raw))
	for i, item := range raw {
		pos := i + 1
		switch v := item.(type) {
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, FieldError{"values", fmt.Sprintf("第 %d 个测量值不是有限数", pos)}
			}
			out = append(out, v)
		case string:
			f, err := ParseNumeric(v, pos)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		default:
			return nil, FieldError{"values", fmt.Sprintf("第 %d 个测量值不是数字", pos)}
		}
	}
	if len(out) == 0 {
		return nil, FieldError{"values", "没有可录入的测量值"}
	}
	return out, nil
}
