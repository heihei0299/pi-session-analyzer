package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

// refreshAndQuery 先 Refresh 再查 ledger，与 server/CLI 生产路径一致。
func refreshAndQuery(t *testing.T, cfg Config, filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	t.Helper()
	if err := refresh.Refresh(refresh.Config{PiDir: cfg.PiDir, CodexDir: cfg.CodexDir, DBPath: cfg.DBPath, Source: "all"}); err != nil {
		t.Fatal(err)
	}
	return Query(cfg, filter, view)
}

func TestQueryCodexAndAllSources(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(home, "sessions", "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl")
	if err := os.WriteFile(rollout, []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":10,"output_tokens":5}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "pi_s1.jsonl"), []byte(`{"type":"session","id":"pi-1","timestamp":"2026-09-08T12:00:00Z","cwd":"/pi"}
{"type":"message","timestamp":"2026-09-08T12:00:01Z","message":{"role":"assistant","model":"pi-model","usage":{"input":2,"output":3}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: piDir, CodexDir: home, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "codex"}
	res, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals == nil || res.Totals.Requests != 1 || res.Totals.TotalTokens != 15 || res.Totals.CostStatus != "unpriced" {
		t.Fatalf("unexpected codex totals: %+v", res)
	}
	// meta.sources 是后端能力声明（能提供哪些数据源），不是本次查询的参与源。
	if res.Meta == nil || len(res.Meta.Sources) != 2 || res.Meta.Sources[0] != "pi" || res.Meta.Sources[1] != "codex" {
		t.Fatalf("meta must declare backend capabilities: %+v", res.Meta)
	}

	cfg.Source = "all"
	res, err = refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals.Requests != 2 || res.Totals.TotalTokens != 20 {
		t.Fatalf("unexpected all totals: %+v", res.Totals)
	}
	res, err = refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := res.Rows.([]domain.SessionRow)
	if !ok || len(rows) != 2 || rows[0].Source == rows[1].Source {
		t.Fatalf("all sessions should expose both sources: %+v", res.Rows)
	}
	if _, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests}); err == nil || err.Error() != ErrRequestsUnsupported.Error() {
		t.Fatalf("expected stable unsupported requests error, got %v", err)
	}
	emptyCodex := t.TempDir()
	cfg.Source = "codex"
	cfg.CodexDir = emptyCodex
	res, err = refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals.Requests != 0 {
		t.Fatalf("changing codex directory must not reuse old ledger rows: %+v", res.Totals)
	}
}

// 真实同形基线 fixture（真实命名 + 按日目录 + archived_sessions + revert + 非 canonical 干扰）
// 必须整条路径可见：发现 → 解析 → 账本 → totals/sessions 窗口。
func TestQueryCodexRealShapedFixtureBaseline(t *testing.T) {
	cfg := Config{
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
		Source:   "codex",
	}

	totals, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if totals.Totals == nil || totals.Totals.Requests != 3 || totals.Totals.TotalTokens <= 0 {
		t.Fatalf("real-shaped fixture must produce codex totals: %+v", totals)
	}
	// 期望值取自 fixture 自行声明的上游口径：usage.total_tokens 14 + 5 + 15 = 34；
	// input 是非缓存输入（10-2、3、7）= 18，cacheRead 2，因此 cacheRate = 2/20 = 0.1。
	if totals.Totals.TotalTokens != 34 || totals.Totals.Input != 18 || totals.Totals.CacheRead != 2 {
		t.Fatalf("codex totals must match upstream-declared usage: %+v", totals.Totals)
	}
	if totals.Totals.CacheRate != 0.1 {
		t.Fatalf("cacheRate must be computed from non-cached input: %+v", totals.Totals)
	}
	if totals.Totals.CostStatus != "unpriced" {
		t.Fatalf("codex cost must stay unpriced: %+v", totals.Totals)
	}
	if totals.Meta == nil || len(totals.Meta.Sources) != 2 || totals.Meta.Sources[0] != "pi" || totals.Meta.Sources[1] != "codex" {
		t.Fatalf("meta must declare backend capabilities: %+v", totals.Meta)
	}
	warnings := strings.Join(totals.Meta.Warnings, "\n")
	if !strings.Contains(warnings, "notes.jsonl") {
		t.Fatalf("non-canonical fixture file must surface as a diagnostic: %+v", totals.Meta.Warnings)
	}
	if totals.Meta.UncountedSnapshots != 2 {
		t.Fatalf("uncounted snapshot coverage must be machine-readable in meta: %+v", totals.Meta)
	}

	sessions, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := sessions.Rows.([]domain.SessionRow)
	if !ok || len(rows) != 3 {
		t.Fatalf("real-shaped fixture must expose three codex sessions: %+v", sessions.Rows)
	}
	var sessionTokens float64
	for _, row := range rows {
		if row.Source != "codex" || row.SessionId == "" {
			t.Fatalf("codex session row must carry source and session id: %+v", row)
		}
		sessionTokens += row.Totals.TotalTokens
	}
	if sessionTokens != 34 {
		t.Fatalf("sessions window must use the same upstream-aligned totals: %v", sessionTokens)
	}

	groups, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupByModel})
	if err != nil {
		t.Fatal(err)
	}
	groupRows, ok := groups.Rows.([]domain.GroupRow)
	if !ok {
		t.Fatalf("unexpected groups payload: %+v", groups.Rows)
	}
	var groupTokens float64
	for _, row := range groupRows {
		groupTokens += row.Totals.TotalTokens
	}
	if groupTokens != 34 {
		t.Fatalf("groups window must use the same upstream-aligned totals: %v", groupTokens)
	}

	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "pi_s1.jsonl"), []byte(`{"type":"session","id":"pi-1","timestamp":"2026-09-08T12:00:00Z","cwd":"/pi"}
{"type":"message","timestamp":"2026-09-08T12:00:01Z","message":{"role":"assistant","model":"pi-model","usage":{"input":2,"output":3}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.PiDir = piDir
	cfg.Source = "all"
	all, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	// pi 5（2+3）与 codex 34 各自计入一次，合计 39：跨源同名列语义一致后 all 不得双算。
	if all.Totals == nil || all.Totals.TotalTokens != 39 || all.Totals.Requests != 4 {
		t.Fatalf("all source must sum both sources without double counting: %+v", all.Totals)
	}
}

// 覆盖率诊断必须对同一份数据每次查询都成立：游标命中而跳过重扫时，不能把「未计入」变成一次性提示。
func TestQueryCodexCoverageDiagnosticsSurviveRepeatedQueries(t *testing.T) {
	cfg := Config{
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
		Source:   "codex",
	}
	for run := 1; run <= 2; run++ {
		res, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
		if err != nil {
			t.Fatal(err)
		}
		if res.Meta == nil {
			t.Fatalf("run %d: missing query metadata", run)
		}
		warnings := strings.Join(res.Meta.Warnings, "\n")
		if res.Meta.UncountedSnapshots != 2 {
			t.Fatalf("run %d must keep reporting uncounted snapshot coverage: %+v", run, res.Meta)
		}
		if !strings.Contains(warnings, "2 条 token_count 快照") || !strings.Contains(warnings, "未计入") {
			t.Fatalf("run %d must keep the human-readable coverage warning: %+v", run, res.Meta.Warnings)
		}
		if len(res.Meta.Warnings) == 0 || !strings.Contains(warnings, "notes.jsonl") {
			t.Fatalf("run %d must keep reporting non-canonical files: %+v", run, res.Meta.Warnings)
		}
		if res.Totals == nil || res.Totals.TotalTokens != 34 {
			t.Fatalf("run %d: totals must not drift across queries: %+v", run, res.Totals)
		}
	}
}

// period 必须按每条 usage event 的消息 timestamp 归属；All 必须保留 Pi 的已知美元金额，
// 同时用 costStatus=unpriced 标注合计里含未定价源。
func TestQueryCodexPeriodUsesMessageTimestampAndAllKeepsKnownCost(t *testing.T) {
	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "pi_period.jsonl"), []byte(`{"type":"session","id":"pi-period","timestamp":"2026-09-09T12:00:00Z","cwd":"/pi"}
{"type":"message","timestamp":"2026-09-09T12:00:01Z","message":{"role":"assistant","model":"pi-model","usage":{"input":2,"output":3,"cost":{"total":0.25}}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		PiDir:    piDir,
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-period-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
		Source:   "codex",
	}

	codexTotals, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if codexTotals.Totals.TotalTokens != 39 || codexTotals.Totals.Cost != 0 || codexTotals.Totals.CostStatus != "unpriced" {
		t.Fatalf("codex totals should stay fully unpriced: %+v", codexTotals.Totals)
	}

	codexPeriod, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay})
	if err != nil {
		t.Fatal(err)
	}
	codexRows, ok := codexPeriod.Rows.([]domain.PeriodRow)
	if !ok || len(codexRows) != 2 {
		t.Fatalf("cross-day codex rollout must split into two day rows: %#v", codexPeriod.Rows)
	}
	if codexRows[0].Period != "2026-09-08" || codexRows[0].TotalTokens != 13 || codexRows[0].CostStatus != "unpriced" {
		t.Fatalf("unexpected 09-08 period row: %+v", codexRows[0])
	}
	if codexRows[1].Period != "2026-09-09" || codexRows[1].TotalTokens != 26 || codexRows[1].CostStatus != "unpriced" {
		t.Fatalf("unexpected 09-09 period row: %+v", codexRows[1])
	}

	cfg.Source = "all"
	allTotals, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if allTotals.Totals.TotalTokens != 44 || allTotals.Totals.Cost != 0.25 || allTotals.Totals.CostStatus != "unpriced" {
		t.Fatalf("all totals must keep known priced cost while marking unpriced source: %+v", allTotals.Totals)
	}

	allPeriod, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay})
	if err != nil {
		t.Fatal(err)
	}
	allRows, ok := allPeriod.Rows.([]domain.PeriodRow)
	if !ok || len(allRows) != 2 {
		t.Fatalf("all period must keep two split day rows: %#v", allPeriod.Rows)
	}
	if allRows[0].Period != "2026-09-08" || allRows[0].TotalTokens != 13 || allRows[0].Cost != 0 {
		t.Fatalf("unexpected all 09-08 row: %+v", allRows[0])
	}
	if allRows[1].Period != "2026-09-09" || allRows[1].TotalTokens != 31 || allRows[1].Cost != 0.25 {
		t.Fatalf("unexpected all 09-09 row: %+v", allRows[1])
	}
	groups, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupByModel})
	if err != nil {
		t.Fatal(err)
	}
	groupRows, ok := groups.Rows.([]domain.GroupRow)
	if !ok || len(groupRows) != 2 {
		t.Fatalf("all groups must expose both models: %#v", groups.Rows)
	}
	groupTokens := map[string]float64{}
	for _, row := range groupRows {
		groupTokens[row.Model] = row.TotalTokens
	}
	if groupTokens["period-model"] != 39 || groupTokens["pi-model"] != 5 {
		t.Fatalf("unexpected all groups: %+v", groupTokens)
	}

	sessions, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	sessionRows, ok := sessions.Rows.([]domain.SessionRow)
	if !ok || len(sessionRows) != 2 {
		t.Fatalf("all sessions must expose pi and codex rows: %#v", sessions.Rows)
	}
	sourceSet := map[string]bool{}
	for _, row := range sessionRows {
		sourceSet[row.Source] = true
	}
	if !sourceSet["codex"] || !sourceSet["pi"] {
		t.Fatalf("all sessions must expose both sources: %+v", sessionRows)
	}

	meta, err := refreshAndQuery(t, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewMeta})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Meta == nil || len(meta.Meta.Sources) != 2 || meta.Meta.Sources[0] != "pi" || meta.Meta.Sources[1] != "codex" {
		t.Fatalf("meta.sources must declare backend capabilities: %+v", meta.Meta)
	}
}
