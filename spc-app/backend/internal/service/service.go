package service

import (
	"context"

	"github.com/jackc/pgx/v5"
	"spcapp/internal/model"
	"spcapp/internal/repo"
	"spcapp/internal/rules"
	"spcapp/internal/spc"
)

// ErrNotFound 档案不存在（等于仓储的同名错误，接口层据此回 404）。
var ErrNotFound = repo.ErrNotFound

// Service 业务编排层。
type Service struct {
	repo *repo.Repository
}

func New(r *repo.Repository) *Service {
	return &Service{repo: r}
}

// ---- 档案 ------------------------------------------------------------------

// CreateTarget 校验并新建监控对象。
func (s *Service) CreateTarget(ctx context.Context, in TargetInput) (*model.Target, error) {
	enabled, err := ValidateTarget(in)
	if err != nil {
		return nil, err
	}
	t := &model.Target{
		Name:         in.Name,
		Machine:      in.Machine,
		Dimension:    in.Dimension,
		SubgroupSize: in.SubgroupSize,
		USL:          in.USL,
		LSL:          in.LSL,
		Rules:        enabledMap(enabled),
	}
	if err := s.repo.CreateTarget(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTarget 更新档案配置。子组容量建档后不可修改（历史切组会失效），
// 规格与判异开关可改；判异开关变化立即按当前序列重放告警。
func (s *Service) UpdateTarget(ctx context.Context, id int64, in TargetInput) (*model.Target, error) {
	old, err := s.repo.GetTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	enabled, err := ValidateTarget(in)
	if err != nil {
		return nil, err
	}
	if in.SubgroupSize != old.SubgroupSize {
		return nil, FieldError{"subgroupSize", "子组容量建档后不可修改；如需变更请新建监控对象"}
	}
	t := &model.Target{
		ID:           id,
		Name:         in.Name,
		Machine:      in.Machine,
		Dimension:    in.Dimension,
		SubgroupSize: in.SubgroupSize,
		USL:          in.USL,
		LSL:          in.LSL,
		Rules:        enabledMap(enabled),
	}
	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		got, err := s.repo.UpdateTarget(ctx, t)
		if err != nil {
			return err
		}
		t = got
		active, err := s.repo.ActiveBaselineTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if active != nil {
			values, err := s.repo.ListValuesTx(ctx, tx, id)
			if err != nil {
				return err
			}
			return s.reconcile(ctx, tx, t, active, measurementsToFloat(values))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListTargets 档案列表。
func (s *Service) ListTargets(ctx context.Context) ([]model.Target, error) {
	return s.repo.ListTargets(ctx)
}

// GetTarget 单个档案。
func (s *Service) GetTarget(ctx context.Context, id int64) (*model.Target, error) {
	return s.repo.GetTarget(ctx, id)
}

// ---- 录入 ------------------------------------------------------------------

// AppendResult 录入结果：完整图数据 + 本次新完成的子组数与残段提示。
type AppendResult struct {
	Chart              *model.Chart `json:"chart"`
	AppendedCount      int          `json:"appendedCount"`
	NewCompletedGroups int          `json:"newCompletedGroups"`
	IncompleteCount    int          `json:"incompleteCount"`
}

// AppendValues 录入一批测量值（逐个或粘贴一列）。
//
// 同一档案的并发录入在事务里对档案加 pg 咨询锁串行化，
// 一次请求的整列数连续占用一段 seq，故子组不丢不重、顺序为服务器收序。
func (s *Service) AppendValues(ctx context.Context, targetID int64,
	values []float64, operator string) (*AppendResult, error) {
	t, err := s.repo.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if err := CheckBatchSize(t.SubgroupSize, len(values)); err != nil {
		return nil, err
	}

	var before int64
	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockTargetTx(ctx, tx, targetID); err != nil {
			return err
		}
		before, err = s.repo.CountValuesTx(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if err := s.repo.InsertValuesTx(ctx, tx, targetID, values, before+1, operator); err != nil {
			return err
		}
		all, err := s.repo.ListValuesTx(ctx, tx, targetID)
		if err != nil {
			return err
		}
		active, err := s.repo.ActiveBaselineTx(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if active != nil {
			return s.reconcile(ctx, tx, t, active, measurementsToFloat(all))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	chart, err := s.GetChart(ctx, targetID)
	if err != nil {
		return nil, err
	}
	beforeGroups := int(before) / t.SubgroupSize
	res := &AppendResult{
		Chart:              chart,
		AppendedCount:      len(values),
		NewCompletedGroups: len(chart.Subgroups) - beforeGroups,
		IncompleteCount:    len(chart.Incomplete),
	}
	return res, nil
}

// reconcile 按「冻结限 + 档案当前启用规则」对基准序列整段重放，
// 并把结果与 alarms 表对齐：新告警插入、消失（如规则被关闭）的删除。
// 这是保证在线追加与整段重放一致的唯一入口。
func (s *Service) reconcile(ctx context.Context, tx pgx.Tx,
	t *model.Target, b *model.Baseline, allValues []float64) error {
	groups, _, err := spc.Summarize(allValues, t.SubgroupSize)
	if err != nil {
		return err
	}
	points := make([]rules.Point, 0, len(groups))
	for _, g := range groups {
		if g.Index < b.StartGroup {
			continue
		}
		points = append(points, rules.Point{Index: g.Index, Mean: g.Mean})
	}
	lim := spc.Limits{
		XbarCL: b.XbarCL, XbarU: b.XbarU, XbarL: b.XbarL,
		RbarCL: b.RbarCL, RbarU: b.RbarU, RbarL: b.RbarL,
	}
	desired := rules.Evaluate(points, lim, toEnabledSet(t.Rules))

	existing, err := s.repo.ListAlarmsForBaselineTx(ctx, tx, b.ID)
	if err != nil {
		return err
	}
	keep := make(map[alarmKey]int64, len(existing))
	for _, a := range existing {
		keep[alarmKey{a.Rule, a.FirstIndex, a.Index}] = a.ID
	}
	wanted := make(map[alarmKey]struct{}, len(desired))
	var staleIDs []int64
	for _, a := range desired {
		key := alarmKey{string(a.Rule), a.FirstIndex, a.Index}
		wanted[key] = struct{}{}
		if _, exists := keep[key]; !exists {
			alarm := model.Alarm{
				BaselineID: b.ID,
				TargetID:   t.ID,
				Version:    b.Version,
				Rule:       string(a.Rule),
				RuleName:   a.RuleName,
				FirstIndex: a.FirstIndex,
				Index:      a.Index,
				Points:     a.Points,
				Message:    a.Message,
			}
			if err := s.repo.UpsertAlarmTx(ctx, tx, &alarm); err != nil {
				return err
			}
		}
	}
	for key, id := range keep {
		if _, ok := wanted[key]; !ok {
			staleIDs = append(staleIDs, id)
		}
	}
	return s.repo.DeleteAlarmsByIDsTx(ctx, tx, staleIDs)
}

type alarmKey struct {
	rule       string
	firstIndex int
	index      int
}

// ---- 基准 ------------------------------------------------------------------

// RebaselineInput 显式发起（重新）基准。
//
//	basisStart/basisEnd：计算限值所用基准期子组范围（闭区间，至少 20 组）。
//	省略时默认取当前全部完整子组。
//
// 与「判定生效区间」无关：首版限从第 1 组起判定；重新基准时旧限保留至冻结时刻，
// 新限只判定冻结之后新录入的子组（见 Rebaseline 内 jurisdictionStart）。
type RebaselineInput struct {
	BasisStart *int `json:"basisStart"`
	BasisEnd   *int `json:"basisEnd"`
}

// Rebaseline 冻结一段新基准：旧限停用并记录生效区间，旧告警随旧限留档。
func (s *Service) Rebaseline(ctx context.Context, targetID int64, in RebaselineInput) (*model.Baseline, error) {
	t, err := s.repo.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	values, err := s.repo.ListValues(ctx, targetID)
	if err != nil {
		return nil, err
	}
	groups, incomplete, err := spc.Summarize(measurementsToFloat(values), t.SubgroupSize)
	if err != nil {
		return nil, err
	}
	if len(incomplete) > 0 {
		// 残段不允许进入基准：必须整组齐。
		return nil, FieldError{"endGroup", "末尾有不足一个子组的测量值，请补齐后再基准"}
	}

	basisStart, basisEnd := 1, len(groups)
	if in.BasisStart != nil {
		basisStart = *in.BasisStart
	}
	if in.BasisEnd != nil {
		basisEnd = *in.BasisEnd
	}
	if err := CheckBaselineRange(len(groups), basisStart, basisEnd); err != nil {
		return nil, err
	}

	baselineGroups := groups[basisStart-1 : basisEnd]
	lim, err := spc.CalcLimits(baselineGroups, t.SubgroupSize)
	if err != nil {
		return nil, err
	}
	sigma, err := spc.WithinSigma(lim.RbarCL, t.SubgroupSize)
	if err != nil {
		return nil, err
	}
	capv, err := spc.CalcCapability(lim.XbarCL, sigma, spc.Spec{USL: t.USL, LSL: t.LSL})
	if err != nil {
		return nil, err
	}

	var saved *model.Baseline
	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.LockTargetTx(ctx, tx, targetID); err != nil {
			return err
		}
		active, err := s.repo.ActiveBaselineTx(ctx, tx, targetID)
		if err != nil {
			return err
		}
		// 判定生效起点：首版为第 1 组（含基准期，历史点也按此限着色）；
		// 重新基准为「冻结时总组数 + 1」——旧限管到冻结时刻，新限只管之后的新点。
		jurisdictionStart := 1
		if active != nil {
			jurisdictionStart = len(groups) + 1
			if err := s.repo.DeactivateBaselinesTx(ctx, tx, targetID, len(groups)); err != nil {
				return err
			}
		}
		next, err := s.nextVersionTx(ctx, tx, targetID)
		if err != nil {
			return err
		}
		saved = &model.Baseline{
			TargetID:      targetID,
			Version:       next,
			StartGroup:    jurisdictionStart,
			EndGroup:      nil,
			BasisStart:    basisStart,
			BasisEnd:      basisEnd,
			XbarCL:        lim.XbarCL,
			XbarU:         lim.XbarU,
			XbarL:         lim.XbarL,
			RbarCL:        lim.RbarCL,
			RbarU:         lim.RbarU,
			RbarL:         lim.RbarL,
			Sigma:         sigma,
			Cp:            capv.Cp,
			Cpk:           capv.Cpk,
			SubgroupCount: basisEnd - basisStart + 1,
			Active:        true,
		}
		if err := s.repo.InsertBaselineTx(ctx, tx, saved); err != nil {
			return err
		}
		// 首版：对其生效区间（1..末）整段重放，基准期告警也照常显示；
		// 重新基准：新限区间内此刻还没有点，重放为空，后续追加时增量对齐。
		return s.reconcile(ctx, tx, t, saved, measurementsToFloat(values))
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *Service) nextVersionTx(ctx context.Context, tx pgx.Tx, targetID int64) (int, error) {
	baselines, err := s.repo.ListBaselinesTx(ctx, tx, targetID)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, b := range baselines {
		if b.Version > max {
			max = b.Version
		}
	}
	return max + 1, nil
}

// ---- 辅助 ------------------------------------------------------------------

func measurementsToFloat(ms []model.Measurement) []float64 {
	out := make([]float64, len(ms))
	for i, m := range ms {
		out[i] = m.Value
	}
	return out
}

func toEnabledSet(m map[string]bool) rules.EnabledSet {
	enabled := rules.EnabledSet{}
	for r, on := range m {
		enabled[rules.RuleID(r)] = on
	}
	return enabled
}

func enabledMap(e rules.EnabledSet) map[string]bool {
	out := map[string]bool{}
	for r, on := range e {
		out[string(r)] = on
	}
	return out
}
