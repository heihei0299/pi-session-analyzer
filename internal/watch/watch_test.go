package watch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func TestIncrementalAppendAndReplace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-analyzer-watch-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionFile := filepath.Join(tmpDir, "session_001.jsonl")
	// 初始文件：仅包含 Header 和一条消息
	initialContent := `{"type":"session","id":"001","timestamp":"2026-08-01T10:00:00Z","cwd":"/p"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","usage":{"input":100,"output":50}}}
`
	if err := os.WriteFile(sessionFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	reader := NewIncrementalReader(tmpDir)
	tot := domain.EmptyTotals()

	// 第一次读取
	incs, err := reader.ReadIncrements()
	if err != nil {
		t.Fatalf("read increments failed: %v", err)
	}
	ApplyIncrements(&tot, incs)

	if tot.Requests != 1 || tot.TotalTokens != 150 {
		t.Fatalf("expected 1 request & 150 tokens, got %d req, %f tokens", tot.Requests, tot.TotalTokens)
	}

	// 无新数据时再次读取，增量应为空
	incs2, _ := reader.ReadIncrements()
	if len(incs2) != 0 {
		t.Fatalf("expected 0 increments without changes, got %d", len(incs2))
	}

	// 模拟追加一条新消息
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append failed: %v", err)
	}
	appendContent := `{"type":"message","timestamp":"2026-08-01T10:10:00Z","message":{"role":"assistant","usage":{"input":200,"output":100}}}
`
	if _, err := f.WriteString(appendContent); err != nil {
		t.Fatalf("write append failed: %v", err)
	}
	f.Close()

	// 增量续读
	incs3, err := reader.ReadIncrements()
	if err != nil {
		t.Fatalf("read increments after append failed: %v", err)
	}
	if len(incs3) != 1 {
		t.Fatalf("expected 1 new increment, got %d", len(incs3))
	}
	ApplyIncrements(&tot, incs3)

	if tot.Requests != 2 || tot.TotalTokens != 450 { // 150 + 300 = 450
		t.Fatalf("expected 2 requests & 450 tokens, got %d req, %f tokens", tot.Requests, tot.TotalTokens)
	}

	// 模拟文件被截断重写为只有一条较小的记录
	rewrittenContent := `{"type":"session","id":"001","timestamp":"2026-08-01T10:00:00Z","cwd":"/p"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","usage":{"input":10,"output":5}}}
`
	if err := os.WriteFile(sessionFile, []byte(rewrittenContent), 0644); err != nil {
		t.Fatalf("rewrite file failed: %v", err)
	}

	// 截断读取（应有负补偿）
	incs4, err := reader.ReadIncrements()
	if err != nil {
		t.Fatalf("read increments after truncate failed: %v", err)
	}
	ApplyIncrements(&tot, incs4)

	// 重写后只有一条 15 tokens 的记录，总量应准确重置为 1 请求、15 tokens
	if tot.Requests != 1 || tot.TotalTokens != 15 {
		t.Fatalf("expected 1 request & 15 tokens after truncate, got %d req, %f tokens", tot.Requests, tot.TotalTokens)
	}
}
