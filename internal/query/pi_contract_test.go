package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

const piCanonicalFixture = ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + "testdata" + string(filepath.Separator) + "canonical" + string(filepath.Separator) + "pi"
const piCanonicalExpected = ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + "testdata" + string(filepath.Separator) + "canonical" + string(filepath.Separator) + "expected" + string(filepath.Separator) + "pi-api.json"

// TestCanonicalPiQueryContract 是 Pi 的 Go 侧 canonical 验收：同一份 synthetic
// fixture + 同一份 golden expected（与 TS oracle 共用），证明 ledger-backed
// Query Engine 与冻结行为无漂移。只锁业务字段，分页/服务端元字段不在契约内。
func TestCanonicalPiQueryContract(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	cfg := Config{PiDir: piCanonicalFixture, DBPath: dbPath, Source: "pi"}
	if err := refresh.Refresh(refresh.Config{PiDir: cfg.PiDir, DBPath: cfg.DBPath, Source: "pi"}); err != nil {
		t.Fatal(err)
	}

	actual := map[string]any{}
	put := func(name string, body map[string]any) { actual[name] = normalizePiContract(name, body) }

	totals, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	put("totals", toMap(t, totals))

	sessions, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions, SortKey: "sessionId", SortDir: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	put("sessions", toMap(t, sessions))

	requests, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests, SortKey: "timestamp", SortDir: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	put("requests", toMap(t, requests))

	groups, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: "model"})
	if err != nil {
		t.Fatal(err)
	}
	put("groups", toMap(t, groups))

	period, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewPeriod, Period: "day"})
	if err != nil {
		t.Fatal(err)
	}
	put("period", toMap(t, period))

	meta, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewMeta})
	if err != nil {
		t.Fatal(err)
	}
	put("meta", toMap(t, meta))

	// detail 保持用户行为：task 是 main 的子会话，fork 靠路径去重不计入 children。
	detail, err := QueryDetail(cfg, "main")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Session.SessionId != "main" || len(detail.Children) != 1 || detail.Children[0].SessionId != "task" {
		t.Fatalf("detail must expose task as the only child of main: %+v", detail)
	}
	if detail.Totals.ChildrenCount != 1 || !detail.Meta.HasChildren {
		t.Fatalf("detail totals/meta must reflect one child: %+v", detail.Totals)
	}
	var mainReqs, childReqs int
	for _, r := range detail.Requests {
		switch r.Source {
		case "main":
			mainReqs++
		case "child":
			childReqs++
		default:
			t.Fatalf("detail request without main/child source: %+v", r)
		}
	}
	if mainReqs != 7 || childReqs != 1 {
		t.Fatalf("detail requests must be 7 main + 1 child, got %d + %d", mainReqs, childReqs)
	}

	expectedBytes, err := os.ReadFile(piCanonicalExpected)
	if err != nil {
		t.Fatalf("canonical golden missing: %v", err)
	}
	var expected map[string]any
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatal(err)
	}
	// Go-only 后端能力声明：meta.sources 恒为 ["pi", "codex"]，TS oracle 已删除。
	if metaActual, ok := actual["meta"].(map[string]any); ok {
		if metaActual["sessionCount"] != expected["meta"].(map[string]any)["sessionCount"] {
			t.Fatalf("meta.sessionCount drifted: %v", metaActual)
		}
		assertPiContractValue(t, metaActual["dataRange"], expected["meta"].(map[string]any)["dataRange"], "meta.dataRange")
		assertPiContractValue(t, metaActual["sources"], expected["meta"].(map[string]any)["sources"], "meta.sources")
		delete(actual, "meta")
		delete(expected, "meta")
	}
	assertPiContractValue(t, actual, expected, "root")
}

func toMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// normalizePiContract 只锁业务字段（与 TS 42 号测试的 normalize 对齐）。
func normalizePiContract(path string, body map[string]any) any {
	if path == "totals" {
		out := map[string]any{"window": body["window"]}
		for _, k := range []string{"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"} {
			out[k] = body[k]
		}
		// totals 视图也可能带 totals 键（QueryResult 序列化），与 golden 对齐只取顶层。
		if nested, ok := body["totals"].(map[string]any); ok {
			for _, k := range []string{"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"} {
				out[k] = nested[k]
			}
		}
		return out
	}
	if path == "meta" {
		m, _ := body["meta"].(map[string]any)
		if m == nil {
			m = body
		}
		return map[string]any{"sessionCount": m["sessionCount"], "dataRange": m["dataRange"], "sources": m["sources"]}
	}
	rows, _ := body["rows"].([]any)
	keys := []string{"period"}
	switch path {
	case "sessions":
		keys = []string{"sessionId"}
	case "requests":
		keys = []string{"timestamp", "sessionId", "model"}
	case "groups":
		keys = []string{"model", "cwd"}
	}
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			// Go omitempty 会省略 isTask:false， golden 显式含该键：补默认对齐。
			if _, ok := m["isTask"]; !ok {
				m["isTask"] = false
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].(map[string]any), rows[j].(map[string]any)
		for _, k := range keys {
			av, bv := stringOf(a[k]), stringOf(b[k])
			if av != bv {
				return av < bv
			}
		}
		return false
	})
	out := map[string]any{}
	for k, v := range body {
		out[k] = v
	}
	out["rows"] = rows
	// sessions 响应的 totals 参与契约（只取业务字段）；window/by/period/total
	// 与 golden 原样比对，分页字段不在契约内。
	if path == "sessions" {
		if tot, ok := body["totals"].(map[string]any); ok {
			nested := map[string]any{}
			for _, k := range []string{"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"} {
				nested[k] = tot[k]
			}
			out["totals"] = nested
		}
	}
	delete(out, "page")
	delete(out, "size")
	return out
}

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

func assertPiContractValue(t *testing.T, actual, expected any, path string) {
	t.Helper()
	if path == "root" {
		actualMap, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s must be an object, got %T", path, actual)
		}
		expectedMap, ok := expected.(map[string]any)
		if !ok {
			t.Fatalf("%s must be an object, got %T", path, expected)
		}
		for key, value := range expectedMap {
			assertPiContractValue(t, actualMap[key], value, path+"."+key)
		}
		return
	}
	// cost/cacheRate 用明确容差，其余数字精确比较（与 TS 42 号测试一致）。
	if strings.HasSuffix(path, ".cost") || strings.HasSuffix(path, ".cacheRate") {
		a, aok := actual.(float64)
		e, eok := expected.(float64)
		if !aok || !eok {
			t.Fatalf("%s: expected numbers, got %#v vs %#v", path, actual, expected)
		}
		delta := a - e
		if delta < 0 {
			delta = -delta
		}
		if delta > 1e-9 {
			t.Fatalf("%s: got %v, want %v ± 1e-9", path, a, e)
		}
		return
	}
	switch expected := expected.(type) {
	case []any:
		actualArray, ok := actual.([]any)
		if !ok || len(actualArray) != len(expected) {
			t.Fatalf("%s: got %#v, want array length %d", path, actual, len(expected))
		}
		for i := range expected {
			assertPiContractValue(t, actualArray[i], expected[i], path+"[]")
		}
	case map[string]any:
		actualObject, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s must be an object, got %#v", path, actual)
		}
		for key, value := range expected {
			assertPiContractValue(t, actualObject[key], value, path+"."+key)
		}
	default:
		if actual != expected {
			t.Fatalf("%s: got %#v, want %#v", path, actual, expected)
		}
	}
}
