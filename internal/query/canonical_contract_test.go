package query

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/codex"
	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

type canonicalCodexSession struct {
	SessionID       string        `json:"sessionId"`
	Timestamp       string        `json:"timestamp"`
	Cwd             string        `json:"cwd"`
	Model           string        `json:"model"`
	Source          string        `json:"source"`
	ParentSessionID string        `json:"parentSessionId,omitempty"`
	Totals          domain.Totals `json:"totals"`
}

type canonicalCodexMeta struct {
	SessionCount       int      `json:"sessionCount"`
	DataRange          any      `json:"dataRange"`
	Sources            []string `json:"sources"`
	UncountedSnapshots int      `json:"uncountedSnapshots"`
}

type canonicalCodexSnapshot struct {
	Totals      domain.Totals           `json:"totals"`
	Sessions    []canonicalCodexSession `json:"sessions"`
	Groups      []domain.GroupRow       `json:"groups"`
	Period      []domain.PeriodRow      `json:"period"`
	Meta        canonicalCodexMeta      `json:"meta"`
	Unsupported string                  `json:"unsupported"`
}

func TestCanonicalCodexQueryContract(t *testing.T) {
	cfg := Config{
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
		Source:   "codex",
	}
	sd := sessiondata.NewSessionData()
	snapshot := canonicalCodexSnapshot{}

	totals, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Totals = *totals.Totals

	sessions, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range sessions.Rows.([]domain.SessionRow) {
		snapshot.Sessions = append(snapshot.Sessions, canonicalCodexSession{
			SessionID: row.SessionId, Timestamp: row.Timestamp, Cwd: row.Cwd, Model: row.Model,
			Source: row.Source, ParentSessionID: row.ParentSessionId, Totals: row.Totals,
		})
	}
	sort.Slice(snapshot.Sessions, func(i, j int) bool {
		left := snapshot.Sessions[i].SessionID + "\x00" + snapshot.Sessions[i].Timestamp
		right := snapshot.Sessions[j].SessionID + "\x00" + snapshot.Sessions[j].Timestamp
		return left < right
	})

	groups, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupByModel})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Groups = groups.Rows.([]domain.GroupRow)
	sort.Slice(snapshot.Groups, func(i, j int) bool { return snapshot.Groups[i].Model < snapshot.Groups[j].Model })

	period, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Period = period.Rows.([]domain.PeriodRow)

	meta, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewMeta})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Meta = canonicalCodexMeta{
		SessionCount: meta.Meta.SessionCount, DataRange: meta.Meta.DataRange, Sources: meta.Meta.Sources,
		UncountedSnapshots: meta.Meta.UncountedSnapshots,
	}
	warnings := strings.Join(meta.Meta.Warnings, "\n")
	for _, marker := range []string{"notes.jsonl", "未计入", "冲突"} {
		if !strings.Contains(warnings, marker) {
			t.Fatalf("canonical diagnostics missing %q: %s", marker, warnings)
		}
	}

	_, unsupported := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests})
	if !errors.Is(unsupported, ErrRequestsUnsupported) {
		t.Fatalf("requests contract changed: %v", unsupported)
	}
	snapshot.Unsupported = unsupported.Error()

	expectedBytes, err := os.ReadFile(filepath.Join("testdata", "canonical-codex-query.json"))
	if err != nil {
		t.Fatalf("canonical golden missing: %v", err)
	}
	var expected canonicalCodexSnapshot
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatal(err)
	}
	assertLedgerOnlyTotals(t, cfg.DBPath, &expected.Totals)
	assertCanonicalJSON(t, snapshot, expected, "root")
}

// assertLedgerOnlyTotals 只读 ledger 行，不访问源文件系统：Query 必须能脱离 rollout 文件复现同一 totals。
func assertLedgerOnlyTotals(t *testing.T, dbPath string, expected *domain.Totals) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	files, err := codex.LoadSessionFiles(database, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// physicalIDs 为空时 LoadSessionFiles 不返回任何行：显式用 DB 全量行构造 ledger-only files。
	if len(files) != 0 {
		t.Fatalf("empty physical-ID set must return no ledger files, got %d", len(files))
	}
	rows, err := database.DB.Query(`SELECT model, session_id, cwd, timestamp_text, physical_rollout_id, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens FROM proxy_request_logs WHERE data_source = 'codex' ORDER BY created_at, request_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	sd := sessiondata.NewSessionData()
	var ledgerFiles []*sessiondata.SessionFileData
	byPhysical := map[string]*sessiondata.SessionFileData{}
	for rows.Next() {
		var model, sessionID, cwd, timestamp, physicalID string
		var input, output, cacheRead, cacheWrite, reasoning int64
		if err := rows.Scan(&model, &sessionID, &cwd, &timestamp, &physicalID, &input, &output, &cacheRead, &cacheWrite, &reasoning); err != nil {
			t.Fatal(err)
		}
		file, ok := byPhysical[physicalID]
		if !ok {
			file = &sessiondata.SessionFileData{SessionId: sessionID, Timestamp: timestamp, Cwd: cwd, Source: "codex"}
			byPhysical[physicalID] = file
			ledgerFiles = append(ledgerFiles, file)
		}
		file.Items = append(file.Items, sessiondata.MessageItem{Timestamp: timestamp, Model: model, Usage: domain.Usage{Input: float64(input), Output: float64(output), CacheRead: float64(cacheRead), CacheWrite: float64(cacheWrite), Reasoning: float64(reasoning)}})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	result, err := sd.QueryFiles("", ledgerFiles, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalJSON(t, result.Totals, expected, "ledger-only.totals")
}

func assertCanonicalJSON(t *testing.T, actual, expected any, path string) {
	t.Helper()
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	var actualValue, expectedValue any
	if err := json.Unmarshal(actualJSON, &actualValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expectedJSON, &expectedValue); err != nil {
		t.Fatal(err)
	}
	assertCanonicalValue(t, actualValue, expectedValue, path)
}

func assertCanonicalValue(t *testing.T, actual, expected any, path string) {
	t.Helper()
	switch expected := expected.(type) {
	case float64:
		actualNumber, ok := actual.(float64)
		if !ok {
			t.Fatalf("%s: expected number, got %T", path, actual)
		}
		delta := actualNumber - expected
		if delta < 0 {
			delta = -delta
		}
		if delta > 1e-9 {
			t.Fatalf("%s: got %v, want %v ± 1e-9", path, actualNumber, expected)
		}
	case []any:
		actualArray, ok := actual.([]any)
		if !ok || len(actualArray) != len(expected) {
			t.Fatalf("%s: got %#v, want array length %d", path, actual, len(expected))
		}
		for i := range expected {
			assertCanonicalValue(t, actualArray[i], expected[i], path+"[]")
		}
	case map[string]any:
		actualObject, ok := actual.(map[string]any)
		if !ok || len(actualObject) != len(expected) {
			t.Fatalf("%s: got %#v, want object with %d fields", path, actual, len(expected))
		}
		for key, value := range expected {
			assertCanonicalValue(t, actualObject[key], value, path+"."+key)
		}
	default:
		if actual != expected {
			t.Fatalf("%s: got %#v, want %#v", path, actual, expected)
		}
	}
}
