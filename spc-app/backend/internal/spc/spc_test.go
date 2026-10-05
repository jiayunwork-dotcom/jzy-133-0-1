package spc

import (
	"math"
	"testing"
)

func fptr(f float64) *float64 { return &f }

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// 均值恰在规格中心时 Cpk == Cp。
func TestCapability_Centered_CpkEqualsCp(t *testing.T) {
	spec := Spec{USL: fptr(10.6), LSL: fptr(9.4)} // 中心 10，半宽 0.6
	c, err := CalcCapability(10.0, 0.2, spec)
	if err != nil {
		t.Fatal(err)
	}
	if c.Cp == nil || c.Cpk == nil {
		t.Fatal("双侧规格应有 Cp/Cpk")
	}
	if !approx(*c.Cp, *c.Cpk) {
		t.Fatalf("居中时 Cpk 应等于 Cp, got Cp=%v Cpk=%v", *c.Cp, *c.Cpk)
	}
	// 数值核对：Cp = 1.2 / (6*0.2) = 1.0
	if !approx(*c.Cp, 1.0) {
		t.Fatalf("Cp 应为 1.0, got %v", *c.Cp)
	}
}

// 规格宽度加倍而数据不变，Cp 加倍。
func TestCapability_DoubleSpecWidth_DoubleCp(t *testing.T) {
	sigma := 0.1
	c1, _ := CalcCapability(5.0, sigma, Spec{USL: fptr(5.2), LSL: fptr(4.8)})
	c2, _ := CalcCapability(5.0, sigma, Spec{USL: fptr(5.4), LSL: fptr(4.6)})
	if !approx(*c2.Cp, 2**c1.Cp) {
		t.Fatalf("规格宽度加倍 Cp 应加倍: %v vs %v", *c1.Cp, *c2.Cp)
	}
}

// 均值往一侧移：Cp 不变、Cpk 下降。
func TestCapability_ShiftMean_CpStableCpkDrops(t *testing.T) {
	spec := Spec{USL: fptr(10.6), LSL: fptr(9.4)}
	center, _ := CalcCapability(10.0, 0.2, spec)
	shifted, _ := CalcCapability(10.3, 0.2, spec)
	if !approx(*center.Cp, *shifted.Cp) {
		t.Fatalf("平移均值 Cp 应不变: %v vs %v", *center.Cp, *shifted.Cp)
	}
	if !(*shifted.Cpk < *center.Cpk) {
		t.Fatalf("平移均值 Cpk 应下降: center=%v shifted=%v", *center.Cpk, *shifted.Cpk)
	}
	// 居中时 Cpk=1.0；偏移 0.3/0.6 后 Cpk = (0.6-0.3)/0.6 = 0.5。
	if !approx(*shifted.Cpk, 0.5) {
		t.Fatalf("偏移后 Cpk 应为 0.5, got %v", *shifted.Cpk)
	}
}

// 单侧规格：只有 Cpk，没有 Cp。
func TestCapability_OneSided(t *testing.T) {
	c, err := CalcCapability(10.0, 0.1, Spec{USL: fptr(10.3)})
	if err != nil {
		t.Fatal(err)
	}
	if c.Cp != nil {
		t.Fatalf("单侧上限不应有 Cp")
	}
	if c.Cpk == nil || !approx(*c.Cpk, 1.0) {
		t.Fatalf("单侧 Cpk 应为 1.0, got %v", c.Cpk)
	}
}

func TestValidateSpec(t *testing.T) {
	if err := ValidateSpec(Spec{}); err == nil {
		t.Fatal("双侧都空应报错")
	}
	if err := ValidateSpec(Spec{USL: fptr(1), LSL: fptr(1)}); err == nil {
		t.Fatal("上限不大于下限应报错")
	}
	if err := ValidateSpec(Spec{USL: fptr(1), LSL: fptr(0.9)}); err != nil {
		t.Fatal("上限大于下限应通过")
	}
}

func TestSummarizeAndLimits(t *testing.T) {
	// n=5，两组全等值 => 极差 0，控制限退化。
	vals := []float64{1, 1, 1, 1, 1, 2, 2, 2, 2, 2}
	groups, incomplete, err := Summarize(vals, 5)
	if err != nil || len(groups) != 2 || len(incomplete) != 0 {
		t.Fatalf("切组错误: groups=%d incomplete=%d err=%v", len(groups), len(incomplete), err)
	}
	if !approx(groups[0].Mean, 1) || groups[0].Range != 0 {
		t.Fatalf("第一组统计错误: %+v", groups[0])
	}
	lim, err := CalcLimits(groups, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(lim.XbarCL, 1.5) || !approx(lim.RbarCL, 0) {
		t.Fatalf("中心线错误: %+v", lim)
	}
	// Rbar=0 => Xbar UCL=LCL=CL，R 限全 0（n=5 时 D3=0）。
	if !approx(lim.XbarU, 1.5) || !approx(lim.XbarL, 1.5) {
		t.Fatalf("零极差控制限应退化: %+v", lim)
	}
}

func TestSummarize_IncompleteTail(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7} // n=5，剩 2 个
	groups, inc, err := Summarize(vals, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(inc) != 2 {
		t.Fatalf("残段处理错误 groups=%d inc=%d", len(groups), len(inc))
	}
}

func TestBadSubgroupSize(t *testing.T) {
	if _, err := Lookup(1); err == nil {
		t.Fatal("n=1 应报错")
	}
	if _, err := Lookup(11); err == nil {
		t.Fatal("n=11 应报错")
	}
	if c, err := Lookup(5); err != nil || c.A2 != 0.577 {
		t.Fatalf("n=5 常数错误: %+v err=%v", c, err)
	}
}
