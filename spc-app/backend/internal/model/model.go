// Package model 定义持久化与业务层共享的数据结构。
package model

import "time"

// Target 监控对象档案：一台机床的一个尺寸。
type Target struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Machine      string    `json:"machine"`
	Dimension    string    `json:"dimension"`
	SubgroupSize int       `json:"subgroupSize"`
	USL          *float64  `json:"usl"`
	LSL          *float64  `json:"lsl"`
	Rules        RuleMap   `json:"rules"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Measurement 单个测量值。同一档案内 seq 从 1 连续递增，是切组的唯一依据。
type Measurement struct {
	ID        int64     `json:"id"`
	TargetID  int64     `json:"targetId"`
	Value     float64   `json:"value"`
	Seq       int64     `json:"seq"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"createdAt"`
}

// Baseline 一次冻结的基准期：中心线、控制限、能力指数及其生效区间。
type Baseline struct {
	ID            int64     `json:"id"`
	TargetID      int64     `json:"targetId"`
	Version       int       `json:"version"`
	StartGroup    int       `json:"startGroup"`
	EndGroup      *int      `json:"endGroup"` // nil 表示当前仍在生效
	BasisStart    int       `json:"basisStart"`
	BasisEnd      int       `json:"basisEnd"`
	XbarCL        float64   `json:"xbarCl"`
	XbarU         float64   `json:"xbarU"`
	XbarL         float64   `json:"xbarL"`
	RbarCL        float64   `json:"rbarCl"`
	RbarU         float64   `json:"rbarU"`
	RbarL         float64   `json:"rbarL"`
	Sigma         float64   `json:"sigma"`
	Cp            *float64  `json:"cp"`
	Cpk           *float64  `json:"cpk"`
	SubgroupCount int       `json:"subgroupCount"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"createdAt"`
	// Note 不持久化：当前基准有告警时由服务层附“过程不受控，指数仅供参考”。
	Note string `json:"note,omitempty"`
}

// Alarm 持久化的一条判异告警。
type Alarm struct {
	ID         int64     `json:"id"`
	BaselineID int64     `json:"baselineId"`
	TargetID   int64     `json:"targetId"`
	Version    int       `json:"version"`
	Rule       string    `json:"rule"`
	RuleName   string    `json:"ruleName"`
	FirstIndex int       `json:"firstIndex"`
	Index      int       `json:"index"`
	Points     IntSlice  `json:"points"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SubgroupView 图上的一个子组点。
type SubgroupView struct {
	Index           int       `json:"index"`
	Mean            float64   `json:"mean"`
	Range           float64   `json:"range"`
	Values          []float64 `json:"values"`
	CreatedAt       time.Time `json:"createdAt"`
	BaselineID      *int64    `json:"baselineId"`
	BaselineVersion *int      `json:"baselineVersion"`
}

// Chart 是前端主图所需的全部数据，数字一律来自后端。
type Chart struct {
	Target       *Target          `json:"target"`
	State        string           `json:"state"` // collecting=基准数据采集中；frozen=已有冻结限
	Subgroups    []SubgroupView   `json:"subgroups"`
	Incomplete   []float64        `json:"incompleteValues"`
	TotalMeas    int64            `json:"totalMeasurements"`
	Baselines    []Baseline       `json:"baselines"`
	Alarms       []Alarm          `json:"alarms"`
	AlarmIndices map[int][]string `json:"alarmIndices"` // 子组序号 -> 命中的规则（着色用）
}
