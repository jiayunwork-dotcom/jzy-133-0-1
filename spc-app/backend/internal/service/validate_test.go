package service

import (
	"errors"
	"testing"
)

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var fe FieldError
	if errors.As(err, &fe) {
		return fe.Field
	}
	var ves ValidationErrors
	if errors.As(err, &ves) && len(ves) > 0 {
		return ves[0].Field
	}
	t.Errorf("期望字段错误，得到 %v", err)
	return ""
}

func TestCheckBatchSize(t *testing.T) {
	if err := CheckBatchSize(5, 1); err != nil {
		t.Fatalf("单值应允许: %v", err)
	}
	if err := CheckBatchSize(5, 10); err != nil {
		t.Fatalf("整数组应允许: %v", err)
	}
	if err := CheckBatchSize(5, 7); err == nil || fieldOf(t, err) != "values" {
		t.Fatalf("数量与子组容量不符应报 values 错误: %v", err)
	}
	if err := CheckBatchSize(5, 0); err == nil {
		t.Fatal("0 个值应报错")
	}
}

func TestCheckBaselineRange(t *testing.T) {
	if err := CheckBaselineRange(25, 1, 25); err != nil {
		t.Fatalf("25 组合法: %v", err)
	}
	if err := CheckBaselineRange(25, 1, 20); err != nil {
		t.Fatalf("恰好 20 组合法: %v", err)
	}
	// 少于 20 组。
	if err := CheckBaselineRange(19, 1, 19); err == nil {
		t.Fatal("19 组应被拒绝（少于 20）")
	} else {
		var ves ValidationErrors
		if !errors.As(err, &ves) || !HasField(err, "endGroup") {
			t.Fatalf("应给出 endGroup 字段提示: %v", err)
		}
	}
	// 范围越界。
	if err := CheckBaselineRange(25, 1, 30); err == nil || !HasField(err, "startGroup") {
		t.Fatalf("end 超出总组数应报 startGroup: %v", err)
	}
	if err := CheckBaselineRange(25, 10, 5); err == nil {
		t.Fatal("start>end 应被拒绝")
	}
}

func TestValidateTarget(t *testing.T) {
	usl, lsl := 10.5, 9.5
	// 正常。
	_, err := ValidateTarget(TargetInput{Name: "x", SubgroupSize: 5, USL: &usl, LSL: &lsl})
	if err != nil {
		t.Fatalf("合法输入应通过: %v", err)
	}
	// 容量越界。
	_, err = ValidateTarget(TargetInput{Name: "x", SubgroupSize: 11, USL: &usl, LSL: &lsl})
	if !HasField(err, "subgroupSize") {
		t.Fatalf("n=11 应报 subgroupSize: %v", err)
	}
	// 上限不大于下限。
	badUSL, badLSL := 9.0, 9.5
	_, err = ValidateTarget(TargetInput{Name: "x", SubgroupSize: 5, USL: &badUSL, LSL: &badLSL})
	if !HasField(err, "usl") {
		t.Fatalf("上限<=下限应报 usl: %v", err)
	}
	// 双侧都空。
	_, err = ValidateTarget(TargetInput{Name: "x", SubgroupSize: 5})
	if !HasField(err, "usl") {
		t.Fatalf("规格双侧为空应报 usl: %v", err)
	}
	// 单侧合法。
	_, err = ValidateTarget(TargetInput{Name: "x", SubgroupSize: 5, USL: &usl})
	if err != nil {
		t.Fatalf("单侧上限应通过: %v", err)
	}
}

func TestParseNumbers(t *testing.T) {
	v, err := ParseNumbers([]any{1.5, "2.5", 3.0})
	if err != nil || len(v) != 3 || v[1] != 2.5 {
		t.Fatalf("混合数值/字符串解析错误: %v %v", v, err)
	}
	_, err = ParseNumbers([]any{1.5, "abc"})
	if err == nil || fieldOf(t, err) != "values" {
		t.Fatalf("非数字应定位报 values: %v", err)
	}
	_, err = ParseNumbers([]any{})
	if err == nil {
		t.Fatal("空数组应报错")
	}
}

func TestEnabledDefaultsAndUnknownRule(t *testing.T) {
	enabled, err := ValidateTarget(TargetInput{
		Name: "x", SubgroupSize: 5, USL: fptrSvc(1.0),
		Rules: map[string]bool{"R1": false, "R9": true},
	})
	_ = enabled
	if !HasField(err, "rules") {
		t.Fatalf("未知规则编号应报 rules: %v", err)
	}
}

func fptrSvc(f float64) *float64 { return &f }
