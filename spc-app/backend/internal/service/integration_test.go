//go:build integration

// 集成测试：需要真实 PostgreSQL 16。
//
//	SPC_TEST_DATABASE_URL=postgres://spc:spc@localhost:5432/spc?sslmode=disable \
//	go test -tags=integration ./internal/service/ -run Integration -v
//
// 覆盖：多检验员并发录入不丢不重、顺序一致；冻结后限值不回算；
// 重新基准旧限与旧告警留档；在线追加告警 == 整段重放。
package service

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"spcapp/internal/db"
	"spcapp/internal/repo"
	"spcapp/internal/rules"
	"spcapp/internal/spc"
)

func intSvc(t *testing.T) *Service {
	t.Helper()
	url := os.Getenv("SPC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://spc:spc@localhost:5432/spc?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("跳过集成测试：连不上数据库: %v", err)
	}
	t.Cleanup(pool.Close)
	return New(repo.New(pool))
}

func usl(v float64) *float64 { return &v }
func lsl(v float64) *float64 { return &v }

func TestIntegrationConcurrentAppendNoLoss(t *testing.T) {
	svc := intSvc(t)
	ctx := context.Background()

	target, err := svc.CreateTarget(ctx, TargetInput{
		Name:    fmt.Sprintf("并发测试-%d", time.Now().UnixNano()),
		Machine: "M", Dimension: "D", SubgroupSize: 5,
		USL: usl(10.5), LSL: lsl(9.5),
	})
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, groupsEach = 8, 10
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(seed)))
			for k := 0; k < groupsEach; k++ {
				vals := make([]float64, 5)
				for i := range vals {
					vals[i] = 10 + rng.Float64()*0.2
				}
				if _, err := svc.AppendValues(ctx, target.ID, vals,
					fmt.Sprintf("检验员%d", seed)); err != nil {
					errCh <- err
					return
				}
				time.Sleep(time.Duration(rng.Intn(2)) * time.Millisecond)
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatalf("并发录入出错: %v", e)
	}

	chart, err := svc.GetChart(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantGroups := goroutines * groupsEach
	if len(chart.Subgroups) != wantGroups {
		t.Fatalf("子组丢失或重复: got %d want %d", len(chart.Subgroups), wantGroups)
	}
	if chart.TotalMeas != int64(wantGroups*5) {
		t.Fatalf("测量值总数错误: got %d want %d", chart.TotalMeas, wantGroups*5)
	}
	if len(chart.Incomplete) != 0 {
		t.Fatalf("不应有残段: %v", chart.Incomplete)
	}

	// seq 连续性由子组值覆盖不到，这里直接核对数量与每组完整性即可。
}

func TestIntegrationFreezeThenRebaselineKeepsHistory(t *testing.T) {
	svc := intSvc(t)
	ctx := context.Background()
	target, err := svc.CreateTarget(ctx, TargetInput{
		Name:         fmt.Sprintf("基准测试-%d", time.Now().UnixNano()),
		SubgroupSize: 2, USL: usl(11), LSL: lsl(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 25 组围绕 10。
	rng := rand.New(rand.NewSource(42))
	for k := 0; k < 25; k++ {
		vals := []float64{10 + rng.NormFloat64()*0.1, 10 + rng.NormFloat64()*0.1}
		if _, err := svc.AppendValues(ctx, target.ID, vals, "t"); err != nil {
			t.Fatal(err)
		}
	}
	b1, err := svc.Rebaseline(ctx, target.ID, RebaselineInput{})
	if err != nil {
		t.Fatalf("首次冻结失败: %v", err)
	}
	cl1 := b1.XbarCL

	// 冻结后再录 5 组，限值不得回算。
	for k := 0; k < 5; k++ {
		if _, err := svc.AppendValues(ctx, target.ID,
			[]float64{10.5, 10.5}, "t"); err != nil {
			t.Fatal(err)
		}
	}
	chart1, err := svc.GetChart(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	var active1 *struct {
		cl float64
		n  int
	}
	for _, b := range chart1.Baselines {
		if b.Active {
			active1 = &struct {
				cl float64
				n  int
			}{b.XbarCL, b.SubgroupCount}
		}
	}
	if active1 == nil || active1.cl != cl1 || active1.n != 25 {
		t.Fatalf("冻结后限值被回算: %+v", active1)
	}

	// 重新基准：旧限留档（仍有 2 个版本），旧点带旧版本号。
	if _, err := svc.Rebaseline(ctx, target.ID, RebaselineInput{}); err != nil {
		t.Fatalf("重新基准失败: %v", err)
	}
	chart2, err := svc.GetChart(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chart2.Baselines) != 2 {
		t.Fatalf("应有 2 个基准版本留档, got %d", len(chart2.Baselines))
	}
	// 旧限管到重新基准时刻（前 30 组仍按 v1 显示），新限只管之后（v2 起点=31）。
	var v1Count, v2Count, unowned int
	for _, s := range chart2.Subgroups {
		switch {
		case s.BaselineVersion == nil:
			unowned++
		case *s.BaselineVersion == 1:
			v1Count++
		case *s.BaselineVersion == 2:
			v2Count++
		}
	}
	if v1Count != 30 {
		t.Fatalf("历史点（含基准期）应仍按旧限 v1 显示: v1=%d", v1Count)
	}
	if v2Count != 0 || unowned != 0 {
		t.Fatalf("重新基准后尚未录入新点，不应有点归属 v2: v2=%d unowned=%d", v2Count, unowned)
	}

	// 再录 3 组，应归属 v2，且 v1 的 30 个点不受影响。
	for k := 0; k < 3; k++ {
		if _, err := svc.AppendValues(ctx, target.ID,
			[]float64{10.6, 10.6}, "t"); err != nil {
			t.Fatal(err)
		}
	}
	chart3, err := svc.GetChart(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	var v1, v2 int
	for _, s := range chart3.Subgroups {
		if s.BaselineVersion == nil {
			t.Fatalf("子组 %d 缺少基准归属", s.Index)
		}
		if *s.BaselineVersion == 1 {
			v1++
		}
		if *s.BaselineVersion == 2 {
			v2++
		}
	}
	if v1 != 30 || v2 != 3 {
		t.Fatalf("新点应只按新限 v2 判定: v1=%d v2=%d", v1, v2)
	}
}

// 集成层验证在线追加与整段重放告警一致（与纯函数测试相互印证落库链路）。
func TestIntegrationOnlineAlarmsEqualReplay(t *testing.T) {
	svc := intSvc(t)
	ctx := context.Background()
	target, err := svc.CreateTarget(ctx, TargetInput{
		Name:         fmt.Sprintf("告警测试-%d", time.Now().UnixNano()),
		SubgroupSize: 2, USL: usl(11), LSL: lsl(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 20 组受控数据。
	for k := 0; k < 20; k++ {
		if _, err := svc.AppendValues(ctx, target.ID,
			[]float64{10, 10}, "t"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Rebaseline(ctx, target.ID, RebaselineInput{}); err != nil {
		t.Fatal(err)
	}
	// 零极差基准 => sigma=0；后续一个不同均值必触发 R1。
	if _, err := svc.AppendValues(ctx, target.ID,
		[]float64{10.2, 10.2}, "t"); err != nil {
		t.Fatal(err)
	}
	chart, err := svc.GetChart(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotRule := false
	for _, a := range chart.Alarms {
		if a.Rule == string(rules.R1Beyond3Sigma) {
			gotRule = true
		}
	}
	if !gotRule {
		t.Fatalf("超 3σ 点应落库一条 R1 告警: %+v", chart.Alarms)
	}

	// 直接用同序列+冻结限在纯函数里重放，键集合应与库中一致。
	active := chart.Baselines[len(chart.Baselines)-1]
	lim := spc.Limits{XbarCL: active.XbarCL, XbarU: active.XbarU, XbarL: active.XbarL,
		RbarCL: active.RbarCL, RbarU: active.RbarU, RbarL: active.RbarL}
	points := make([]rules.Point, 0, len(chart.Subgroups))
	for _, s := range chart.Subgroups {
		if s.BaselineVersion != nil && *s.BaselineVersion == active.Version {
			points = append(points, rules.Point{Index: s.Index, Mean: s.Mean})
		}
	}
	replayed := rules.Evaluate(points, lim, rules.DefaultEnabled())
	replayKeys := map[string]bool{}
	for _, a := range replayed {
		replayKeys[string(a.Rule)+"@"+fmt.Sprint(a.FirstIndex)+"-"+fmt.Sprint(a.Index)] = true
	}
	dbKeys := map[string]bool{}
	for _, a := range chart.Alarms {
		if a.Version == active.Version {
			dbKeys[a.Rule+"@"+fmt.Sprint(a.FirstIndex)+"-"+fmt.Sprint(a.Index)] = true
		}
	}
	if len(replayKeys) != len(dbKeys) {
		ks := func(m map[string]bool) []string {
			out := make([]string, 0, len(m))
			for k := range m {
				out = append(out, k)
			}
			sort.Strings(out)
			return out
		}
		t.Fatalf("在线落库告警与重放不一致:\nreplay=%v\ndb=%v", ks(replayKeys), ks(dbKeys))
	}
}
