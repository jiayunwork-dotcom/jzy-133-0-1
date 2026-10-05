// Package repo 封装 PostgreSQL 数据访问。业务计算不放在这里。
package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"spcapp/internal/model"
)

// ErrNotFound 按主键查不到档案。
var ErrNotFound = errors.New("监控对象不存在")

// Repository 数据仓储。
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// WithTx 在一个事务里执行 fn；fn 返回错误时回滚。
func (r *Repository) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ---- targets ---------------------------------------------------------------

const targetCols = `id, name, machine, dimension, subgroup_size, usl, lsl, rules, created_at`

func scanTarget(row pgx.Row) (*model.Target, error) {
	var t model.Target
	if err := row.Scan(&t.ID, &t.Name, &t.Machine, &t.Dimension,
		&t.SubgroupSize, &t.USL, &t.LSL, &t.Rules, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateTarget 新建档案。
func (r *Repository) CreateTarget(ctx context.Context, t *model.Target) error {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO targets (name, machine, dimension, subgroup_size, usl, lsl, rules)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+targetCols,
		t.Name, t.Machine, t.Dimension, t.SubgroupSize, t.USL, t.LSL, rulesParam(t.Rules))
	got, err := scanTarget(row)
	if err != nil {
		return err
	}
	*t = *got
	return nil
}

// UpdateTarget 更新档案配置（判异开关、规格等）。测量值与冻结限不受影响。
func (r *Repository) UpdateTarget(ctx context.Context, t *model.Target) (*model.Target, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE targets SET name=$2, machine=$3, dimension=$4, subgroup_size=$5,
		        usl=$6, lsl=$7, rules=$8
		 WHERE id=$1 RETURNING `+targetCols,
		t.ID, t.Name, t.Machine, t.Dimension, t.SubgroupSize, t.USL, t.LSL, rulesParam(t.Rules))
	got, err := scanTarget(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return got, err
}

// ListTargets 列出全部档案。
func (r *Repository) ListTargets(ctx context.Context) ([]model.Target, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+targetCols+` FROM targets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Target{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTarget 按主键取档案。
func (r *Repository) GetTarget(ctx context.Context, id int64) (*model.Target, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+targetCols+` FROM targets WHERE id=$1`, id)
	t, err := scanTarget(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// rulesParam 保证 nil map 也能落成 JSON 对象。
func rulesParam(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}

// ---- measurements ----------------------------------------------------------

// LockTargetTx 对档案加事务级咨询锁：同一档案的并发录入在此串行，
// 不同档案互不阻塞。配合 seq 唯一约束，子组不丢不重。
func (r *Repository) LockTargetTx(ctx context.Context, tx pgx.Tx, targetID int64) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, targetID)
	return err
}

// CountValuesTx 返回档案已有测量值个数（持锁时即下一批 seq 的起点）。
func (r *Repository) CountValuesTx(ctx context.Context, tx pgx.Tx, targetID int64) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM measurements WHERE target_id=$1`, targetID).Scan(&n)
	return n, err
}

// InsertValuesTx 一次粘贴/录入的整列数连续占段 [startSeq, startSeq+len)。
// seq 唯一索引兜底任何并发越界。
func (r *Repository) InsertValuesTx(ctx context.Context, tx pgx.Tx, targetID int64,
	values []float64, startSeq int64, operator string) error {
	seqs := make([]int64, len(values))
	for i := range values {
		seqs[i] = startSeq + int64(i)
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO measurements (target_id, value, seq, operator)
		 SELECT $1, v, s, $4 FROM unnest($2::double precision[], $3::bigint[]) AS t(v, s)`,
		targetID, values, seqs, operator)
	return err
}

// ListValues 按 seq 升序返回全部测量值（切组、重放的唯一数据源）。
func (r *Repository) ListValues(ctx context.Context, targetID int64) ([]model.Measurement, error) {
	return r.ListValuesQ(ctx, r.pool, targetID)
}

// ListValuesTx 事务版本：录入事务内读到的就是插入后的完整序列。
func (r *Repository) ListValuesTx(ctx context.Context, tx pgx.Tx, targetID int64) ([]model.Measurement, error) {
	return r.ListValuesQ(ctx, tx, targetID)
}

// querier 连接池与事务共同满足的查询接口。
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func scanMeasurement(row pgx.Row) (model.Measurement, error) {
	var m model.Measurement
	err := row.Scan(&m.ID, &m.TargetID, &m.Value, &m.Seq, &m.Operator, &m.CreatedAt)
	return m, err
}

// ListValuesQ querier 版本，连接池/事务共用。
func (r *Repository) ListValuesQ(ctx context.Context, q querier, targetID int64) ([]model.Measurement, error) {
	rows, err := q.Query(ctx,
		`SELECT id, target_id, value, seq, operator, created_at
		 FROM measurements WHERE target_id=$1 ORDER BY seq`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Measurement{}
	for rows.Next() {
		m, err := scanMeasurement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- baselines -------------------------------------------------------------

const baselineCols = `id, target_id, version, start_group, end_group,
	basis_start, basis_end,
	xbar_cl, xbar_u, xbar_l, rbar_cl, rbar_u, rbar_l, sigma,
	cp, cpk, subgroup_count, active, created_at`

func scanBaseline(row pgx.Row) (model.Baseline, error) {
	var b model.Baseline
	err := row.Scan(&b.ID, &b.TargetID, &b.Version, &b.StartGroup, &b.EndGroup,
		&b.BasisStart, &b.BasisEnd,
		&b.XbarCL, &b.XbarU, &b.XbarL, &b.RbarCL, &b.RbarU, &b.RbarL, &b.Sigma,
		&b.Cp, &b.Cpk, &b.SubgroupCount, &b.Active, &b.CreatedAt)
	return b, err
}

// ActiveBaseline 取当前生效基准；没有时返回 (nil,nil)。
func (r *Repository) ActiveBaseline(ctx context.Context, targetID int64) (*model.Baseline, error) {
	return r.ActiveBaselineQ(ctx, r.pool, targetID)
}

// ActiveBaselineTx 事务版本。
func (r *Repository) ActiveBaselineTx(ctx context.Context, tx pgx.Tx, targetID int64) (*model.Baseline, error) {
	return r.ActiveBaselineQ(ctx, tx, targetID)
}

// ActiveBaselineQ querier 版本。
func (r *Repository) ActiveBaselineQ(ctx context.Context, q querier, targetID int64) (*model.Baseline, error) {
	row := q.QueryRow(ctx,
		`SELECT `+baselineCols+` FROM baselines WHERE target_id=$1 AND active`, targetID)
	b, err := scanBaseline(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBaselines 返回档案全部基准（含已停用的旧限），按版本升序。
func (r *Repository) ListBaselines(ctx context.Context, targetID int64) ([]model.Baseline, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+baselineCols+` FROM baselines WHERE target_id=$1 ORDER BY version`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Baseline{}
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeactivateBaselinesTx 关闭档案当前生效基准，记录其结束子组。
func (r *Repository) DeactivateBaselinesTx(ctx context.Context, tx pgx.Tx,
	targetID int64, endGroup int) error {
	_, err := tx.Exec(ctx,
		`UPDATE baselines SET active=false, end_group=$2
		 WHERE target_id=$1 AND active`, targetID, endGroup)
	return err
}

// InsertBaselineTx 冻结一条新基准并返回其 id/created_at。
func (r *Repository) InsertBaselineTx(ctx context.Context, tx pgx.Tx, b *model.Baseline) error {
	row := tx.QueryRow(ctx,
		`INSERT INTO baselines
		 (target_id, version, start_group, end_group, basis_start, basis_end,
		  xbar_cl, xbar_u, xbar_l,
		  rbar_cl, rbar_u, rbar_l, sigma, cp, cpk, subgroup_count, active)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		 RETURNING id, created_at`,
		b.TargetID, b.Version, b.StartGroup, b.EndGroup, b.BasisStart, b.BasisEnd,
		b.XbarCL, b.XbarU, b.XbarL, b.RbarCL, b.RbarU, b.RbarL, b.Sigma,
		b.Cp, b.Cpk, b.SubgroupCount, b.Active)
	return row.Scan(&b.ID, &b.CreatedAt)
}

// ---- alarms ----------------------------------------------------------------

// UpsertAlarmTx 幂等写入告警；同基准同规则同窗口已存在则不动（保证重放稳定）。
func (r *Repository) UpsertAlarmTx(ctx context.Context, tx pgx.Tx, a *model.Alarm) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO alarms
		 (baseline_id, target_id, version, rule, rule_name,
		  first_index, idx, points, message)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (baseline_id, rule, first_index, idx) DO NOTHING`,
		a.BaselineID, a.TargetID, a.Version, a.Rule, a.RuleName,
		a.FirstIndex, a.Index, a.Points, a.Message)
	return err
}

// DeleteAlarmsForRulesTx 删除当前生效基准下指定规则的全部告警。
// 用于在档案中关闭某条规则（关闭即不再判该规则，历史告警随之隐藏）。
func (r *Repository) DeleteAlarmsForRulesTx(ctx context.Context, tx pgx.Tx,
	targetID int64, rules []string) error {
	if len(rules) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx,
		`DELETE FROM alarms
		 WHERE target_id=$1
		   AND rule = ANY($2)
		   AND baseline_id = (SELECT id FROM baselines WHERE target_id=$1 AND active)`,
		targetID, rules)
	return err
}

// ListAlarms 返回档案全部基准下的告警，按版本与子组排序。
func (r *Repository) ListAlarms(ctx context.Context, targetID int64) ([]model.Alarm, error) {
	return r.listAlarmsQ(ctx, r.pool,
		`SELECT a.id, a.baseline_id, a.version, a.rule, a.rule_name,
		        a.first_index, a.idx, a.points, a.message, a.created_at
		 FROM alarms a WHERE a.target_id=$1
		 ORDER BY a.version, a.idx, a.rule`, targetID)
}

// ListAlarmsForBaselineTx 事务内列出某基准下现存告警（reconcile 对齐用）。
func (r *Repository) ListAlarmsForBaselineTx(ctx context.Context, tx pgx.Tx, baselineID int64) ([]model.Alarm, error) {
	return r.listAlarmsQ(ctx, tx,
		`SELECT a.id, a.baseline_id, a.version, a.rule, a.rule_name,
		        a.first_index, a.idx, a.points, a.message, a.created_at
		 FROM alarms a WHERE a.baseline_id=$1
		 ORDER BY a.idx, a.rule`, baselineID)
}

func (r *Repository) listAlarmsQ(ctx context.Context, q querier, sqlText string, arg int64) ([]model.Alarm, error) {
	rows, err := q.Query(ctx, sqlText, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Alarm{}
	for rows.Next() {
		var a model.Alarm
		if err := rows.Scan(&a.ID, &a.BaselineID, &a.Version, &a.Rule, &a.RuleName,
			&a.FirstIndex, &a.Index, &a.Points, &a.Message, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlarmsByIDsTx 按主键批量删除（reconcile 中规则关闭/窗口不再命中时）。
func (r *Repository) DeleteAlarmsByIDsTx(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `DELETE FROM alarms WHERE id = ANY($1)`, ids)
	return err
}

// ListBaselinesTx 事务版本。
func (r *Repository) ListBaselinesTx(ctx context.Context, tx pgx.Tx, targetID int64) ([]model.Baseline, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+baselineCols+` FROM baselines WHERE target_id=$1 ORDER BY version`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Baseline{}
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
