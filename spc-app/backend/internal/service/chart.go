package service

import (
	"context"

	"spcapp/internal/model"
)

// GetChart 装配前端主图所需的全部数据。
//
// 每个子组点带上其「当时生效」的基准 id/版本；各基准的限由前端按段绘制，
// 从而重新基准后旧限与旧点仍按旧限展示，新点按新限展示。
func (s *Service) GetChart(ctx context.Context, targetID int64) (*model.Chart, error) {
	t, err := s.repo.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	measurements, err := s.repo.ListValues(ctx, targetID)
	if err != nil {
		return nil, err
	}
	baselines, err := s.repo.ListBaselines(ctx, targetID)
	if err != nil {
		return nil, err
	}
	alarms, err := s.repo.ListAlarms(ctx, targetID)
	if err != nil {
		return nil, err
	}

	n := t.SubgroupSize
	subs := make([]model.SubgroupView, 0, len(measurements)/n)
	for i := 0; i+n <= len(measurements); i += n {
		seg := measurements[i : i+n]
		vals := make([]float64, n)
		sum := 0.0
		minv, maxv := seg[0].Value, seg[0].Value
		last := seg[0].CreatedAt
		for j, m := range seg {
			vals[j] = m.Value
			sum += m.Value
			if m.Value < minv {
				minv = m.Value
			}
			if m.Value > maxv {
				maxv = m.Value
			}
			if m.CreatedAt.After(last) {
				last = m.CreatedAt
			}
		}
		idx := i/n + 1
		view := model.SubgroupView{
			Index:     idx,
			Mean:      sum / float64(n),
			Range:     maxv - minv,
			Values:    vals,
			CreatedAt: last,
		}
		if bid, ver, ok := baselineFor(baselines, idx); ok {
			view.BaselineID = &bid
			v := ver
			view.BaselineVersion = &v
		}
		subs = append(subs, view)
	}

	// 残段（不足一子组的待补测量值）原样回传，录入区提示“还缺几个”。
	// 用非 nil 切片，保证空时 JSON 输出 [] 而非 null（前端直接 .length）。
	incomplete := make([]float64, 0)
	if rem := len(measurements) % n; rem != 0 {
		incomplete = make([]float64, rem)
		start := len(measurements) - rem
		for i, m := range measurements[start:] {
			incomplete[i] = m.Value
		}
	}

	// 子组序号 -> 命中的规则（着色用）。触发点与被牵连的点都标。
	alarmIdx := map[int][]string{}
	for _, a := range alarms {
		for _, p := range a.Points {
			alarmIdx[p] = append(alarmIdx[p], a.Rule)
		}
	}

	state := "collecting"
	outBaselines := make([]model.Baseline, 0, len(baselines))
	for i := range baselines {
		b := baselines[i]
		if b.Active {
			state = "frozen"
			b.Note = capabilityNote(alarmsForVersion(alarms, b.Version))
		}
		outBaselines = append(outBaselines, b)
	}

	chart := &model.Chart{
		Target:       t,
		State:        state,
		Subgroups:    subs,
		Incomplete:   incomplete,
		TotalMeas:    int64(len(measurements)),
		Baselines:    outBaselines,
		Alarms:       alarms,
		AlarmIndices: alarmIdx,
	}
	return chart, nil
}

// baselineFor 判定某子组在冻结时受哪条基准约束：
// 点序号 >= 该基准起点，且 < 下一基准起点（或当前基准 active）。
func baselineFor(bs []model.Baseline, groupIdx int) (int64, int, bool) {
	var chosen *model.Baseline
	for i := range bs {
		b := &bs[i]
		if groupIdx < b.StartGroup {
			continue
		}
		if b.EndGroup != nil && groupIdx > *b.EndGroup {
			continue
		}
		if chosen == nil || b.StartGroup >= chosen.StartGroup {
			chosen = b
		}
	}
	if chosen == nil {
		return 0, 0, false
	}
	return chosen.ID, chosen.Version, true
}

func alarmsForVersion(all []model.Alarm, version int) []model.Alarm {
	out := make([]model.Alarm, 0)
	for _, a := range all {
		if a.Version == version {
			out = append(out, a)
		}
	}
	return out
}

// capabilityNote 有告警点时能力指数附言；无告警为空。
func capabilityNote(versionAlarms []model.Alarm) string {
	if len(versionAlarms) > 0 {
		return "过程不受控，指数仅供参考"
	}
	return ""
}
