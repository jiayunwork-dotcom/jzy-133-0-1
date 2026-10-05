// Package spc 实现均值–极差图（Xbar-R）所需的标准常数与统计算法。
//
// 所有控制限、中心线、过程能力指数都只在本包（Go 后端）计算，前端只做展示。
package spc

import (
	"errors"
	"math"
)

// Constants 按子组容量 n（2..10）取标准表值：
//
//	A2: Xbar 图控制限系数，UCL/LCL = Xbarbar ± A2 * Rbar
//	D3: R 图下控制限系数，LCL = D3 * Rbar
//	D4: R 图上控制限系数，UCL = D4 * Rbar
//	d2: 相对极差常数，组内标准差 sigma = Rbar / d2（用于 Cp/Cpk）
//
// 数值来自统计过程控制常用的标准常数表。
type Constants struct {
	A2, D3, D4, d2 float64
}

var table = map[int]Constants{
	2:  {A2: 1.880, D3: 0.0, D4: 3.267, d2: 1.128},
	3:  {A2: 1.023, D3: 0.0, D4: 2.574, d2: 1.693},
	4:  {A2: 0.729, D3: 0.0, D4: 2.282, d2: 2.059},
	5:  {A2: 0.577, D3: 0.0, D4: 2.114, d2: 2.326},
	6:  {A2: 0.483, D3: 0.0, D4: 2.004, d2: 2.534},
	7:  {A2: 0.419, D3: 0.076, D4: 1.924, d2: 2.704},
	8:  {A2: 0.373, D3: 0.136, D4: 1.864, d2: 2.847},
	9:  {A2: 0.337, D3: 0.184, D4: 1.816, d2: 2.970},
	10: {A2: 0.308, D3: 0.223, D4: 1.777, d2: 3.078},
}

// ErrBadSubgroupSize 子组容量不在 2..10 的标准表范围内。
var ErrBadSubgroupSize = errors.New("子组容量必须在 2 到 10 之间")

// Lookup 返回子组容量对应的标准常数。
func Lookup(n int) (Constants, error) {
	c, ok := table[n]
	if !ok {
		return Constants{}, ErrBadSubgroupSize
	}
	return c, nil
}

// Subgroup 是一个已经切好的子组：序号从 1 开始（与用户核对时口径一致）。
type Subgroup struct {
	Index int
	// Mean 子组均值，Range 子组极差（max-min）。
	Mean, Range float64
}

// Summarize 把一段原始测量值按容量 n 顺序切成子组并计算均值/极差。
// 末尾不足 n 个的残段返回为 incomplete，不计入统计。
func Summarize(values []float64, n int) (groups []Subgroup, incomplete []float64, err error) {
	if _, err := Lookup(n); err != nil {
		return nil, nil, err
	}
	full := len(values) / n
	groups = make([]Subgroup, 0, full)
	for i := 0; i < full; i++ {
		seg := values[i*n : (i+1)*n]
		sum, min, max := 0.0, seg[0], seg[0]
		for _, v := range seg {
			sum += v
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		groups = append(groups, Subgroup{Index: i + 1, Mean: sum / float64(n), Range: max - min})
	}
	if rem := len(values) % n; rem != 0 {
		incomplete = values[full*n:]
	}
	return groups, incomplete, nil
}

// Limits 是冻结下来的中心线与控制限（Xbar 图与 R 图）。
type Limits struct {
	XbarCL float64 // 均值图中心线 Xbarbar
	XbarU  float64 // 均值图 UCL
	XbarL  float64 // 均值图 LCL
	RbarCL float64 // 极差图中心线 Rbar
	RbarU  float64 // 极差图 UCL
	RbarL  float64 // 极差图 LCL
}

// CalcLimits 用基准期子组计算中心线与控制限。
func CalcLimits(groups []Subgroup, n int) (Limits, error) {
	c, err := Lookup(n)
	if err != nil {
		return Limits{}, err
	}
	if len(groups) == 0 {
		return Limits{}, errors.New("基准期没有可用子组")
	}
	var sumM, sumR float64
	for _, g := range groups {
		sumM += g.Mean
		sumR += g.Range
	}
	m := sumM / float64(len(groups))
	r := sumR / float64(len(groups))
	return Limits{
		XbarCL: m,
		XbarU:  m + c.A2*r,
		XbarL:  m - c.A2*r,
		RbarCL: r,
		RbarU:  c.D4 * r,
		RbarL:  c.D3 * r,
	}, nil
}

// WithinSigma 由基准期平均极差估计组内标准差：sigma = Rbar / d2。
// Rbar 为 0 时返回 0；能力指数计算方据此拒绝出数。
func WithinSigma(rbar float64, n int) (float64, error) {
	c, err := Lookup(n)
	if err != nil {
		return 0, err
	}
	return rbar / c.d2, nil
}

// Capability 为过程能力结果。单侧规格时缺失的一侧对应字段为 nil。
type Capability struct {
	Cp  *float64
	Cpk *float64
	// Sigma 组内标准差估计；为 0 时 Cp/Cpk 均为 nil。
	Sigma float64
}

// Spec 表示规格限，允许单侧（USL 或 LSL 之一为 nil）。
type Spec struct {
	USL *float64
	LSL *float64
}

// ValidateSpec 校验规格限：至少有一侧；双侧时 USL 必须大于 LSL。
func ValidateSpec(s Spec) error {
	if s.USL == nil && s.LSL == nil {
		return errors.New("规格上下限至少要填写一个")
	}
	if s.USL != nil && s.LSL != nil && *s.USL <= *s.LSL {
		return errors.New("规格上限必须大于规格下限")
	}
	return nil
}

// CalcCapability 按基准期（均值 Xbarbar、组内 sigma）计算 Cp/Cpk。
//
//	Cp  = (USL-LSL)/(6σ)                 仅双侧规格时有值
//	Cpk = min((USL-μ)/3σ, (μ-LSL)/3σ)    有哪侧算哪侧
func CalcCapability(mean, sigma float64, spec Spec) (Capability, error) {
	if err := ValidateSpec(spec); err != nil {
		return Capability{}, err
	}
	capv := Capability{Sigma: sigma}
	if sigma <= 0 {
		// 零波动时比值无定义，交给上层以 null 展示并说明原因。
		return capv, nil
	}
	if spec.USL != nil && spec.LSL != nil {
		cp := (*spec.USL - *spec.LSL) / (6 * sigma)
		capv.Cp = &cp
	}
	var cpk float64 = math.Inf(1)
	if spec.USL != nil {
		cpk = math.Min(cpk, (*spec.USL-mean)/(3*sigma))
	}
	if spec.LSL != nil {
		cpk = math.Min(cpk, (mean-*spec.LSL)/(3*sigma))
	}
	capv.Cpk = &cpk
	return capv, nil
}
