package model

import (
	"database/sql/driver"
	"encoding/json"
)

// RuleMap 是 targets.rules(JSONB) 的 Go 类型：map[规则编号]是否启用。
// 实现 sql.Scanner / driver.Valuer，使 pgx 能在 JSONB 与 map 间可靠转换。
type RuleMap map[string]bool

// Scan 实现 sql.Scanner。
func (m *RuleMap) Scan(src any) error {
	if src == nil {
		*m = RuleMap{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	}
	out := RuleMap{}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return err
		}
	}
	*m = out
	return nil
}

// Value 实现 driver.Valuer。
func (m RuleMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// IntSlice 是 alarms.points(JSONB) 的 Go 类型。
type IntSlice []int

// Scan 实现 sql.Scanner。
func (s *IntSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	}
	out := IntSlice{}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return err
		}
	}
	*s = out
	return nil
}

// Value 实现 driver.Valuer。
func (s IntSlice) Value() (driver.Value, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}
