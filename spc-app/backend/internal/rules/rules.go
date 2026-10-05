// Package rules 实现均值图（Xbar）上的四条 Nelson 判异规则。
//
// 规则（仅作用于均值图，中心线与控制限来自冻结的基准期）：
//
//	R1 一点落在 3σ（UCL/LCL）之外。
//	R2 连续 9 点落在中心线同一侧。
//	R3 连续 6 点递增或递减。
//	R4 连续 3 点中有 2 点落在同侧 2σ～3σ 区（含 2σ 边界、不含 3σ 边界）。
//
// 评估策略见 ARCHITECTURE.md：采用「整段序列从头重放」，而不是增量维护窗口。
// 规则按「窗口的最后一点」登记告警（firstIndex=窗口首点、index=窗口末点），
// 因此按时间顺序追加只会产生新告警，旧告警永不消失（重新基准除外），
// 从而保证「在线逐点追加」与「整段重放」结果完全一致。
package rules

import (
	"fmt"
	"spcapp/internal/spc"
)

// RuleID 规则编号。
type RuleID string

const (
	R1Beyond3Sigma      RuleID = "R1" // 一点超出 3σ
	R2SameSide9         RuleID = "R2" // 连续 9 点同侧
	R3Trend6            RuleID = "R3" // 连续 6 点递增/递减
	R4TwoOfThreeBeyond2 RuleID = "R4" // 3 点中 2 点落在同侧 2σ~3σ
)

// RuleName 规则中文名（供前端直接展示，数字仍以后端为准）。
func RuleName(r RuleID) string {
	switch r {
	case R1Beyond3Sigma:
		return "一点超出三倍标准差"
	case R2SameSide9:
		return "连续九点落在中心线同侧"
	case R3Trend6:
		return "连续六点递增或递减"
	case R4TwoOfThreeBeyond2:
		return "三点中有两点落在同侧两倍到三倍之间"
	}
	return string(r)
}

// EnabledSet 每条规则的开关。
type EnabledSet map[RuleID]bool

// DefaultEnabled 默认四条全开。
func DefaultEnabled() EnabledSet {
	return EnabledSet{R1Beyond3Sigma: true, R2SameSide9: true, R3Trend6: true, R4TwoOfThreeBeyond2: true}
}

// Validate 拒绝未知规则编号。
func (e EnabledSet) Validate() error {
	for r := range e {
		if RuleName(r) == string(r) {
			return errUnknownRule(string(r))
		}
	}
	return nil
}

// Alarm 一条判异告警。
type Alarm struct {
	Rule       RuleID `json:"rule"`
	RuleName   string `json:"ruleName"`
	Chart      string `json:"chart"` // 固定 "xbar"，当前只判均值图
	FirstIndex int    `json:"firstIndex"`
	Index      int    `json:"index"` // 触发点（窗口末点），图上该点着色
	Points     []int  `json:"points"`
	Message    string `json:"message"`
}

// Point 参与判异的一个均值点。
type Point struct {
	Index int
	Mean  float64
}

// zone 返回均值点相对中心线的分区：
//
//	+3 / -3 : 严格超出 3σ（含恰在限上不算——限上为在控；超出才算）
//	+2 / -2 : 达到 2σ 但未超 3σ（含 2σ 边界、含恰在 3σ 限上）
//	+1 / -1 : 中心线与 2σ 之间（恰在中心线上归 0）
//	0        : 恰在中心线上
//
// σ 由 Xbar 控制限推出：sigma = (UCL-CL)/3。
func zone(mean, cl, sigma float64) int {
	switch {
	case mean > cl+3*sigma:
		return 3
	case mean < cl-3*sigma:
		return -3
	case mean >= cl+2*sigma:
		return 2
	case mean <= cl-2*sigma:
		return -2
	case mean > cl:
		return 1
	case mean < cl:
		return -1
	default:
		return 0
	}
}

// Evaluate 对整条均值序列按启用的规则重放，返回按 (触发点序号, 规则) 排序的告警。
// 该函数是确定性的：同一段序列 + 同一组冻结限值 => 同一批告警。
func Evaluate(points []Point, lim spc.Limits, enabled EnabledSet) []Alarm {
	alarms := []Alarm{}
	sigma := (lim.XbarU - lim.XbarCL) / 3
	if sigma <= 0 {
		// 极差为 0 时控制限退化为中心线，任何抖动都超 3σ；只保留 R1 的退化判定，
		// 其余依赖 σ 分区的规则无意义。
		if enabled[R1Beyond3Sigma] {
			for _, p := range points {
				if p.Mean != lim.XbarCL {
					alarms = append(alarms, makeAlarm(R1Beyond3Sigma, p.Index, p.Index,
						[]int{p.Index}, "子组 %d 均值超出三倍标准差控制限", p.Index))
				}
			}
		}
		return alarms
	}

	zones := make([]int, len(points))
	for i, p := range points {
		zones[i] = zone(p.Mean, lim.XbarCL, sigma)
	}

	for i, p := range points {
		z := zones[i]

		if enabled[R1Beyond3Sigma] && (z == 3 || z == -3) {
			alarms = append(alarms, makeAlarm(R1Beyond3Sigma, p.Index, p.Index,
				[]int{p.Index}, "子组 %d 均值超出三倍标准差控制限", p.Index))
		}

		if enabled[R2SameSide9] && i >= 8 {
			same := true
			for k := i - 8; k < i; k++ {
				if zones[k] == 0 || sign(zones[k]) != sign(z) || z == 0 {
					same = false
					break
				}
			}
			if same {
				pts := indexRange(points[i-8].Index, p.Index)
				side := "上侧"
				if z < 0 {
					side = "下侧"
				}
				alarms = append(alarms, makeAlarm(R2SameSide9, pts[0], pts[len(pts)-1], pts,
					"子组 %d～%d 连续九点落在中心线%s", pts[0], pts[len(pts)-1], side))
			}
		}

		if enabled[R3Trend6] && i >= 5 {
			up, down := true, true
			for k := i - 5; k < i; k++ {
				if !(points[k+1].Mean > points[k].Mean) {
					up = false
				}
				if !(points[k+1].Mean < points[k].Mean) {
					down = false
				}
			}
			if up || down {
				pts := indexRange(points[i-5].Index, p.Index)
				dir := "递增"
				if down {
					dir = "递减"
				}
				alarms = append(alarms, makeAlarm(R3Trend6, pts[0], pts[len(pts)-1], pts,
					"子组 %d～%d 连续六点%s", pts[0], pts[len(pts)-1], dir))
			}
		}

		if enabled[R4TwoOfThreeBeyond2] && i >= 2 {
			var hi, lo []int
			for k := i - 2; k <= i; k++ {
				switch zones[k] {
				case 2, 3:
					hi = append(hi, points[k].Index)
				case -2, -3:
					lo = append(lo, points[k].Index)
				}
			}
			winStart, winEnd := points[i-2].Index, p.Index
			switch {
			case len(hi) >= 2:
				alarms = append(alarms, makeAlarm(R4TwoOfThreeBeyond2, winStart, winEnd,
					colorPoints(hi, winEnd),
					"子组 %d～%d 的三点中有两点落在上侧两倍到三倍标准差区", winStart, winEnd))
			case len(lo) >= 2:
				alarms = append(alarms, makeAlarm(R4TwoOfThreeBeyond2, winStart, winEnd,
					colorPoints(lo, winEnd),
					"子组 %d～%d 的三点中有两点落在下侧两倍到三倍标准差区", winStart, winEnd))
			}
		}
	}
	return alarms
}

func errUnknownRule(id string) error {
	return fmt.Errorf("未知的判异规则编号: %s", id)
}

func sign(z int) int {
	switch {
	case z > 0:
		return 1
	case z < 0:
		return -1
	}
	return 0
}

func indexRange(from, to int) []int {
	pts := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		pts = append(pts, i)
	}
	return pts
}

func makeAlarm(r RuleID, firstIndex, index int, pts []int, format string, args ...any) Alarm {
	return Alarm{
		Rule:       r,
		RuleName:   RuleName(r),
		Chart:      "xbar",
		FirstIndex: firstIndex,
		Index:      index,
		Points:     pts,
		Message:    fmt.Sprintf(format, args...),
	}
}

// colorPoints 告警着色的点：远离中心的点 + 触发点（窗口末点，可能本身不在 2σ 区）。
func colorPoints(far []int, windowEnd int) []int {
	pts := append([]int{}, far...)
	for _, p := range pts {
		if p == windowEnd {
			return pts
		}
	}
	pts = append(pts, windowEnd)
	return pts
}
