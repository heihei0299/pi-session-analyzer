package sessiondata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func TestForkDeduplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-analyzer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个普通的 parent session
	parentFile := filepath.Join(tmpDir, "parent_001.jsonl")
	parentContent := `{"type":"session","id":"001","timestamp":"2026-08-01T10:00:00Z","cwd":"/project/a"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":100,"output":50}}}
`
	if err := os.WriteFile(parentFile, []byte(parentContent), 0644); err != nil {
		t.Fatalf("failed to write parent file: %v", err)
	}

	// 创建一个 fork session，包含复制历史（ts < forkTs）以及 fork 后产生的新消息（ts >= forkTs）
	forkFile := filepath.Join(tmpDir, "fork_002.jsonl")
	forkContent := `{"type":"session","id":"002","timestamp":"2026-08-01T12:00:00Z","cwd":"/project/a","parentSession":"` + parentFile + `"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":100,"output":50}}}
{"type":"message","timestamp":"2026-08-01T12:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":200,"output":80}}}
`
	if err := os.WriteFile(forkFile, []byte(forkContent), 0644); err != nil {
		t.Fatalf("failed to write fork file: %v", err)
	}

	sd := NewSessionData()
	// 解析 parent
	pData, err := sd.AnalyzeFile(parentFile)
	if err != nil || pData == nil {
		t.Fatalf("failed to analyze parent: %v", err)
	}
	if len(pData.Items) != 1 {
		t.Errorf("expected 1 item in parent, got %d", len(pData.Items))
	}

	// 解析 fork，验证历史消息被剔除（ADR-0001）
	fData, err := sd.AnalyzeFile(forkFile)
	if err != nil || fData == nil {
		t.Fatalf("failed to analyze fork: %v", err)
	}
	if len(fData.Items) != 1 {
		t.Fatalf("expected 1 item in fork after dedup, got %d", len(fData.Items))
	}
	if fData.Items[0].Usage.Input != 200 {
		t.Errorf("expected 200 input in fork, got %f", fData.Items[0].Usage.Input)
	}

	// 聚合两者总量
	res, err := sd.Query(tmpDir, Filter{}, View{Kind: ViewTotals})
	if err != nil {
		t.Fatalf("query totals failed: %v", err)
	}
	// 期望总量：parent(100 in, 50 out) + fork(200 in, 80 out) = 300 in, 130 out, totalTokens = 430
	if res.Totals.TotalTokens != 430 {
		t.Errorf("expected 430 totalTokens, got %f", res.Totals.TotalTokens)
	}
	if res.Totals.Requests != 2 {
		t.Errorf("expected 2 requests, got %d", res.Totals.Requests)
	}
}

func TestSkipResidualNonSessionFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-analyzer-residual-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	badFile := filepath.Join(tmpDir, "bad.jsonl")
	badContent := `{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant"}}`
	if err := os.WriteFile(badFile, []byte(badContent), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}

	sd := NewSessionData()
	data, err := sd.AnalyzeFile(badFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil for non-session header file, got %+v", data)
	}
}

func TestPeriodAggregation(t *testing.T) {
	sd := NewSessionData()
	// 2026-08-05 是周三，对应周一应为 2026-08-03
	kDay, _ := sd.PeriodKey("2026-08-05T10:00:00Z", domain.PeriodDay)
	if kDay != "2026-08-05" {
		t.Errorf("expected 2026-08-05 for day, got %s", kDay)
	}
	kMonth, _ := sd.PeriodKey("2026-08-05T10:00:00Z", domain.PeriodMonth)
	if kMonth != "2026-08-01" {
		t.Errorf("expected 2026-08-01 for month, got %s", kMonth)
	}
	kWeek, _ := sd.PeriodKey("2026-08-05T10:00:00Z", domain.PeriodWeek)
	if kWeek != "2026-08-03" {
		t.Errorf("expected 2026-08-03 for week, got %s", kWeek)
	}
}
