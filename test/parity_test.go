package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type TotalsOutput struct {
	Window      string  `json:"window"`
	Requests    int     `json:"requests"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	Reasoning   float64 `json:"reasoning"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        float64 `json:"cost"`
	CacheRate   float64 `json:"cacheRate"`
}

func TestNodeAndGoParity(t *testing.T) {
	// 确认 node 存在
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found, skipping parity test")
	}

	tmpDir, err := os.MkdirTemp("", "token-analyzer-parity-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	parentFile := filepath.Join(tmpDir, "parent_p1.jsonl")
	parentContent := `{"type":"session","id":"p1","timestamp":"2026-08-01T08:00:00.000Z","cwd":"/proj/demo"}
{"type":"message","timestamp":"2026-08-01T08:05:00.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":100,"output":50,"cacheRead":200,"cacheWrite":20,"reasoning":10,"cost":{"total":0.05}}}}
{"type":"message","timestamp":"2026-08-01T08:10:00.000Z","message":{"role":"assistant","model":"deepseek-chat","usage":{"input":300,"output":150,"cacheRead":100,"cacheWrite":0,"reasoning":30,"cost":{"total":0.02}}}}
`
	if err := os.WriteFile(parentFile, []byte(parentContent), 0644); err != nil {
		t.Fatalf("failed to write parent file: %v", err)
	}

	forkFile := filepath.Join(tmpDir, "fork_f1.jsonl")
	// Fork 会话：包含复制历史（08:05 和 08:10 复制父消息）以及 fork 后产生的新消息（09:05 新消息）
	forkContent := `{"type":"session","id":"f1","timestamp":"2026-08-01T09:00:00.000Z","cwd":"/proj/demo","parentSession":"` + parentFile + `"}
{"type":"message","timestamp":"2026-08-01T08:05:00.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":100,"output":50,"cacheRead":200,"cacheWrite":20,"reasoning":10,"cost":{"total":0.05}}}}
{"type":"message","timestamp":"2026-08-01T09:05:00.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":400,"output":200,"cacheRead":500,"cacheWrite":50,"reasoning":40,"cost":{"total":0.10}}}}
`
	if err := os.WriteFile(forkFile, []byte(forkContent), 0644); err != nil {
		t.Fatalf("failed to write fork file: %v", err)
	}

	// 1. 运行 Node CLI 获取 totals
	cmdNode := exec.Command("node", "src/cli.ts", "totals", "--format", "json", "--dir", tmpDir)
	cmdNode.Dir = filepath.Dir(tmpDir) // run in repo
	repoRoot, _ := filepath.Abs("../")
	cmdNode.Dir = repoRoot
	outNode, err := cmdNode.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI failed: %v, out: %s", err, string(outNode))
	}

	var nodeTotals TotalsOutput
	if err := json.Unmarshal(outNode, &nodeTotals); err != nil {
		t.Fatalf("failed to parse node output: %v, raw: %s", err, string(outNode))
	}

	// 2. 运行 Go 二进制获取 totals
	goBin := filepath.Join(repoRoot, "dist", "token-analyzer-go")
	cmdGo := exec.Command(goBin, "totals", "--format", "json", "--dir", tmpDir)
	outGo, err := cmdGo.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI failed: %v, out: %s", err, string(outGo))
	}

	var goTotals TotalsOutput
	if err := json.Unmarshal(outGo, &goTotals); err != nil {
		t.Fatalf("failed to parse go output: %v, raw: %s", err, string(outGo))
	}

	// 3. 断言比对两者数据完全一致
	if nodeTotals.Requests != goTotals.Requests {
		t.Errorf("Requests mismatch: Node=%d, Go=%d", nodeTotals.Requests, goTotals.Requests)
	}
	if nodeTotals.Input != goTotals.Input {
		t.Errorf("Input mismatch: Node=%f, Go=%f", nodeTotals.Input, goTotals.Input)
	}
	if nodeTotals.Output != goTotals.Output {
		t.Errorf("Output mismatch: Node=%f, Go=%f", nodeTotals.Output, goTotals.Output)
	}
	if nodeTotals.CacheRead != goTotals.CacheRead {
		t.Errorf("CacheRead mismatch: Node=%f, Go=%f", nodeTotals.CacheRead, goTotals.CacheRead)
	}
	if nodeTotals.CacheWrite != goTotals.CacheWrite {
		t.Errorf("CacheWrite mismatch: Node=%f, Go=%f", nodeTotals.CacheWrite, goTotals.CacheWrite)
	}
	if nodeTotals.Reasoning != goTotals.Reasoning {
		t.Errorf("Reasoning mismatch: Node=%f, Go=%f", nodeTotals.Reasoning, goTotals.Reasoning)
	}
	if nodeTotals.TotalTokens != goTotals.TotalTokens {
		t.Errorf("TotalTokens mismatch: Node=%f, Go=%f", nodeTotals.TotalTokens, goTotals.TotalTokens)
	}
	if nodeTotals.Cost != goTotals.Cost {
		t.Errorf("Cost mismatch: Node=%f, Go=%f", nodeTotals.Cost, goTotals.Cost)
	}
	if nodeTotals.CacheRate != goTotals.CacheRate {
		t.Errorf("CacheRate mismatch: Node=%f, Go=%f", nodeTotals.CacheRate, goTotals.CacheRate)
	}
}
