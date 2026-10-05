package rules

import (
	"testing"

	"spcapp/internal/spc"
)

// 构造一个方便的限值：中心线 10，σ=0.25（二进制精确，避免边界用例的浮点噪声），
// 故 3σ=0.75（UCL=10.75 / LCL=9.25），2σ=0.5（边界 10.5 / 9.5）。
func testLimits() spc.Limits {
	return spc.Limits{XbarCL: 10, XbarU: 10.75, XbarL: 9.25,
		RbarCL: 0.25, RbarU: 0.5285, RbarL: 0}
}

func pts(means ...float64) []Point {
	out := make([]Point, len(means))
	for i, m := range means {
		out[i] = Point{Index: i + 1, Mean: m}
	}
	return out
}

func hasRule(alarms []Alarm, r RuleID, triggerIndex int) bool {
	for _, a := range alarms {
		if a.Rule == r && a.Index == triggerIndex {
			return true
		}
	}
	return false
}

// ---- R1：一点超出 3σ ------------------------------------------------------

func TestR1_TriggersBeyond3Sigma(t *testing.T) {
	lim := testLimits()
	// 恰好在 UCL（10.75）上不算超出；10.751 超出 => 触发。
	a := Evaluate(pts(10, 10, 10.751), lim, DefaultEnabled())
	if !hasRule(a, R1Beyond3Sigma, 3) {
		t.Fatalf("超出 UCL 应触发 R1: %+v", a)
	}
}

func TestR1_BoundaryDoesNotTrigger(t *testing.T) {
	lim := testLimits()
	// 恰好等于 UCL=10.75（3σ 线上）不触发。
	a := Evaluate(pts(10, 10, 10.75), lim, DefaultEnabled())
	if hasRule(a, R1Beyond3Sigma, 3) {
		t.Fatalf("恰在控制限上不应触发 R1: %+v", a)
	}
	// 下侧对称。
	a = Evaluate(pts(10, 10, 9.25), lim, DefaultEnabled())
	if hasRule(a, R1Beyond3Sigma, 3) {
		t.Fatalf("恰在 LCL 上不应触发 R1: %+v", a)
	}
}

// ---- R2：连续 9 点同侧 -----------------------------------------------------

func TestR2_NineSameSideTriggers(t *testing.T) {
	lim := testLimits()
	means := []float64{10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if !hasRule(a, R2SameSide9, 9) {
		t.Fatalf("连续 9 点同侧应在第 9 点触发 R2: %+v", a)
	}
	if len(a[0].Points) != 9 {
		t.Fatalf("R2 应记录涉及的 9 个子组")
	}
}

func TestR2_EightSameSideDoesNotTrigger(t *testing.T) {
	lim := testLimits()
	means := []float64{10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R2SameSide9, 8) {
		t.Fatalf("只有 8 点同侧不应触发 R2: %+v", a)
	}
}

func TestR2_OnCenterlineBreaksRun(t *testing.T) {
	lim := testLimits()
	// 8 点在上，第 9 点在中心线上，不触发。
	means := []float64{10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10.01, 10}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R2SameSide9, 9) {
		t.Fatalf("中心线点打断连续，不应触发: %+v", a)
	}
}

// ---- R3：连续 6 点递增/递减 ------------------------------------------------

func TestR3_SixIncreasingTriggers(t *testing.T) {
	lim := testLimits()
	means := []float64{9.95, 9.96, 9.97, 9.98, 9.99, 10.0} // 严格 6 点递增
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if !hasRule(a, R3Trend6, 6) {
		t.Fatalf("连续 6 点递增应触发 R3: %+v", a)
	}
}

func TestR3_SixDecreasingTriggers(t *testing.T) {
	lim := testLimits()
	means := []float64{10.0, 9.99, 9.98, 9.97, 9.96, 9.95}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if !hasRule(a, R3Trend6, 6) {
		t.Fatalf("连续 6 点递减应触发 R3: %+v", a)
	}
}

func TestR3_FiveStepsDoesNotTrigger(t *testing.T) {
	lim := testLimits()
	means := []float64{9.96, 9.97, 9.98, 9.99, 10.0} // 只有 5 点
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R3Trend6, 5) {
		t.Fatalf("仅 5 点递增不应触发 R3: %+v", a)
	}
}

func TestR3_EqualValueBreaksTrend(t *testing.T) {
	lim := testLimits()
	// 6 点中出现相等（非严格递增），不触发。
	means := []float64{9.95, 9.96, 9.97, 9.98, 9.99, 9.99}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R3Trend6, 6) {
		t.Fatalf("相等值打断趋势，不应触发: %+v", a)
	}
}

// ---- R4：3 点中 2 点落在同侧 2σ~3σ ----------------------------------------

func TestR4_TwoOfThreeBeyond2SigmaTriggers(t *testing.T) {
	lim := testLimits()
	// 10.5 恰在 2σ 线上（计入区域），10.6 在区内，第 3 点 10.0 普通。
	means := []float64{10.5, 10.6, 10.0}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if !hasRule(a, R4TwoOfThreeBeyond2, 3) {
		t.Fatalf("3 点中 2 点在上侧 2σ~3σ 应触发 R4: %+v", a)
	}
}

func TestR4_OnlyOneBeyond2DoesNotTrigger(t *testing.T) {
	lim := testLimits()
	// 三点中仅 1 点达到 2σ，差一点不触发。
	means := []float64{10.5, 10.1, 10.0}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R4TwoOfThreeBeyond2, 3) {
		t.Fatalf("仅 1 点在 2σ 区不应触发 R4: %+v", a)
	}
}

func TestR4_OppositeSidesDoNotCombine(t *testing.T) {
	lim := testLimits()
	// 上侧一点、下侧一点 + 普通点，不触发（必须同侧）。
	means := []float64{10.6, 9.4, 10.0}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if hasRule(a, R4TwoOfThreeBeyond2, 3) {
		t.Fatalf("异侧的 2σ 点不能合并计数: %+v", a)
	}
}

func TestR4_Beyond3SigmaAlsoCounts(t *testing.T) {
	lim := testLimits()
	// 一个点超 3σ（也算远离中心）+ 一个 2σ 点，同样满足“两点远离”。
	means := []float64{10.8, 10.55, 10.0}
	a := Evaluate(pts(means...), lim, DefaultEnabled())
	if !hasRule(a, R4TwoOfThreeBeyond2, 3) {
		t.Fatalf("超 3σ 点应同样计入 2σ 远离计数: %+v", a)
	}
}

// ---- 开关 -----------------------------------------------------------------

func TestDisabledRuleDoesNotEvaluate(t *testing.T) {
	lim := testLimits()
	enabled := EnabledSet{R1Beyond3Sigma: false, R2SameSide9: false,
		R3Trend6: false, R4TwoOfThreeBeyond2: false}
	a := Evaluate(pts(11.0), lim, enabled)
	if len(a) != 0 {
		t.Fatalf("全部规则关闭时不应有告警: %+v", a)
	}
}

// ---- 一致性：在线逐点追加 == 整段重放 -------------------------------------

// 这是用户明确要核对的性质：告警按窗口末点登记，旧告警永不消失。
func TestOnlineAppendEqualsFullReplay(t *testing.T) {
	lim := testLimits()
	all := []float64{
		10.01, 10.02, 10.0, 10.01, 10.02, // 杂点（第 3 点恰在中心线，打断 R2）
		10.55, 10.56, 10.05, // 6、7 在 2σ~3σ 区，第 8 点在普通区 => R4 在第 8 点
		10.05, 10.05, // 9、10
		10.05, 10.05, 10.05, 10.05, 10.05, 10.05, 10.05, // 11..17
		10.8, // 第 18 点超 UCL => R1
	}
	// R4 窗口 6-8（触发点 8）；同侧串 8..16 九点 => R2 触发点 16，8..17 => 触发点 17；
	// 第 18 点 => R1。共 4 条，且出现顺序与整段重放的 (触发点, 规则) 顺序一致。

	// 整段重放。
	full := Evaluate(pts(all...), lim, DefaultEnabled())

	// 逐点“在线追加”：每加一点评估一次，累积所有出现过的告警。
	seen := map[string]bool{}
	var online []Alarm
	for k := 1; k <= len(all); k++ {
		cur := Evaluate(pts(all[:k]...), lim, DefaultEnabled())
		for _, a := range cur {
			key := string(a.Rule) + "@" + itoa(a.FirstIndex) + "-" + itoa(a.Index)
			if !seen[key] {
				seen[key] = true
				online = append(online, a)
			}
		}
	}

	if len(online) != len(full) {
		t.Fatalf("在线追加(%d) 与整段重放(%d) 告警数不一致", len(online), len(full))
	}
	for i := range full {
		if online[i].Rule != full[i].Rule ||
			online[i].FirstIndex != full[i].FirstIndex ||
			online[i].Index != full[i].Index {
			t.Fatalf("第 %d 条告警不一致: online=%+v full=%+v", i, online[i], full[i])
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
