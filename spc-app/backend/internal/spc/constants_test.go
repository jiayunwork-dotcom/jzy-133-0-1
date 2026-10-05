package spc

import "testing"

// 端到端核对常数表 → 控制限 → σ → 能力的数值，确保没有抄错表值。
func TestConstantsTableValues(t *testing.T) {
	cases := map[int]struct{ a2, d3, d4, d2 float64 }{
		2:  {1.880, 0.0, 3.267, 1.128},
		5:  {0.577, 0.0, 2.114, 2.326},
		7:  {0.419, 0.076, 1.924, 2.704},
		10: {0.308, 0.223, 1.777, 3.078},
	}
	for n, want := range cases {
		got, err := Lookup(n)
		if err != nil {
			t.Fatal(err)
		}
		if got.A2 != want.a2 || got.D3 != want.d3 || got.D4 != want.d4 || got.d2 != want.d2 {
			t.Fatalf("n=%d 表值错误: got=%+v want=%+v", n, got, want)
		}
	}
}

// 用一个对称基准：每组成员围绕均值 ±0.5，n=5 无法完全对称，
// 这里直接给恒定 Rbar 验证 WithinSigma = Rbar/d2。
func TestWithinSigmaFromRbar(t *testing.T) {
	// n=5, Rbar=2.326 => sigma = 1.0（d2=2.326）
	sig, err := WithinSigma(2.326, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(sig, 1.0) {
		t.Fatalf("sigma 应为 1.0, got %v", sig)
	}
}

// 完整能力数值链：σ=0.5，USL=11.5 LSL=8.5（宽 3），居中 μ=10。
// Cp = 3/(6*0.5)=1.0；居中 Cpk=1.0；规格加宽到 6 => Cp=2.0。
func TestCapabilityNumericChain(t *testing.T) {
	sigma := 0.5
	c1, err := CalcCapability(10, sigma, Spec{USL: fptr(11.5), LSL: fptr(8.5)})
	if err != nil {
		t.Fatal(err)
	}
	if !approx(deref(c1.Cp), 1.0) || !approx(deref(c1.Cpk), 1.0) {
		t.Fatalf("居中 Cp/Cpk 应为 1.0: Cp=%v Cpk=%v", c1.Cp, c1.Cpk)
	}
	c2, _ := CalcCapability(10, sigma, Spec{USL: fptr(13), LSL: fptr(7)})
	if !approx(deref(c2.Cp), 2.0) {
		t.Fatalf("规格加宽一倍 Cp 应为 2.0: %v", c2.Cp)
	}
	// μ 移到 10.5：Cp 仍 1.0（用 c1 规格），Cpk = (11.5-10.5)/(3*0.5)=0.6667
	c3, _ := CalcCapability(10.5, sigma, Spec{USL: fptr(11.5), LSL: fptr(8.5)})
	if !approx(deref(c3.Cp), 1.0) {
		t.Fatalf("平移不改变 Cp: %v", c3.Cp)
	}
	if !approx(deref(c3.Cpk), 2.0/3.0) {
		t.Fatalf("平移后 Cpk 应为 2/3: %v", c3.Cpk)
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
