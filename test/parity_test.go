package test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type TotalsOutput struct {
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

type GenericRow map[string]any

type QueryOutput struct {
	Window string       `json:"window"`
	Rows   []GenericRow `json:"rows"`
	Total  int          `json:"total"`
}

func TestNodeAndGoParity(t *testing.T) {
	// 创建测试临时目录与真实会话文件结构
	tmpDir, err := os.MkdirTemp("", "parity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	parentFile := filepath.Join(tmpDir, "session_parent123.jsonl")
	forkFile := filepath.Join(tmpDir, "session_fork456.jsonl")

	parentContent := `{"type":"session","id":"parent123","timestamp":"2026-08-01T10:00:00.000Z","cwd":"/repo"}
{"type":"message","timestamp":"2026-08-01T10:01:00.000Z","message":{"role":"user","content":"hello"}}
{"type":"message","timestamp":"2026-08-01T10:01:05.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":10,"reasoning":0,"totalTokens":170,"cost":{"total":0.005}}}}
`
	if err := os.WriteFile(parentFile, []byte(parentContent), 0644); err != nil {
		t.Fatalf("failed to write parent file: %v", err)
	}

	forkContent := fmt.Sprintf(`{"type":"session","id":"fork456","timestamp":"2026-08-01T10:05:00.000Z","cwd":"/repo","parentSession":%q}
{"type":"message","timestamp":"2026-08-01T10:01:00.000Z","message":{"role":"user","content":"hello"}}
{"type":"message","timestamp":"2026-08-01T10:01:05.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":10,"reasoning":0,"totalTokens":170,"cost":{"total":0.005}}}}
{"type":"message","timestamp":"2026-08-01T10:06:00.000Z","message":{"role":"user","content":"next"}}
{"type":"message","timestamp":"2026-08-01T10:06:05.000Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":200,"output":80,"cacheRead":50,"cacheWrite":0,"reasoning":0,"totalTokens":330,"cost":{"total":0.01}}}}
`, parentFile)

	if err := os.WriteFile(forkFile, []byte(forkContent), 0644); err != nil {
		t.Fatalf("failed to write fork file: %v", err)
	}

	repoRoot, _ := filepath.Abs("../")
	goBin := filepath.Join(repoRoot, "dist", "token-analyzer-go")

	// 1. 全量 Totals 对账
	cmdNode := exec.Command("node", "src/cli.ts", "totals", "--format", "json", "--dir", tmpDir)
	cmdNode.Dir = repoRoot
	outNode, err := cmdNode.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI totals failed: %v, out: %s", err, string(outNode))
	}

	cmdGo := exec.Command(goBin, "totals", "--format", "json", "--dir", tmpDir)
	outGo, err := cmdGo.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI totals failed: %v, out: %s", err, string(outGo))
	}

	var nodeTotals, goTotals TotalsOutput
	if err := json.Unmarshal(outNode, &nodeTotals); err != nil {
		t.Fatalf("parse node totals failed: %v", err)
	}
	if err := json.Unmarshal(outGo, &goTotals); err != nil {
		t.Fatalf("parse go totals failed: %v", err)
	}

	if nodeTotals.Requests != goTotals.Requests || nodeTotals.TotalTokens != goTotals.TotalTokens || nodeTotals.Cost != goTotals.Cost {
		t.Fatalf("Totals mismatch! Node: %+v, Go: %+v", nodeTotals, goTotals)
	}

	// 2. 全量 Sessions 对账
	cmdNodeSess := exec.Command("node", "src/cli.ts", "sessions", "--format", "json", "--dir", tmpDir)
	cmdNodeSess.Dir = repoRoot
	outNodeSess, err := cmdNodeSess.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI sessions failed: %v", err)
	}

	cmdGoSess := exec.Command(goBin, "sessions", "--format", "json", "--dir", tmpDir)
	outGoSess, err := cmdGoSess.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI sessions failed: %v", err)
	}

	var nodeSess, goSess QueryOutput
	_ = json.Unmarshal(outNodeSess, &nodeSess)
	_ = json.Unmarshal(outGoSess, &goSess)
	if len(nodeSess.Rows) != len(goSess.Rows) || len(goSess.Rows) != 2 {
		t.Fatalf("Sessions length mismatch: Node=%d, Go=%d", len(nodeSess.Rows), len(goSess.Rows))
	}

	// 3. 全量 Requests 对账
	cmdNodeReq := exec.Command("node", "src/cli.ts", "requests", "--format", "json", "--dir", tmpDir)
	cmdNodeReq.Dir = repoRoot
	outNodeReq, err := cmdNodeReq.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI requests failed: %v", err)
	}

	cmdGoReq := exec.Command(goBin, "requests", "--format", "json", "--dir", tmpDir)
	outGoReq, err := cmdGoReq.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI requests failed: %v", err)
	}

	var nodeReq, goReq QueryOutput
	_ = json.Unmarshal(outNodeReq, &nodeReq)
	_ = json.Unmarshal(outGoReq, &goReq)
	if len(nodeReq.Rows) != len(goReq.Rows) || len(goReq.Rows) != 2 {
		t.Fatalf("Requests length mismatch: Node=%d, Go=%d", len(nodeReq.Rows), len(goReq.Rows))
	}

	// 4. 全量 Groups 对账
	cmdNodeGrp := exec.Command("node", "src/cli.ts", "totals", "--by", "model", "--format", "json", "--dir", tmpDir)
	cmdNodeGrp.Dir = repoRoot
	outNodeGrp, err := cmdNodeGrp.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI groups failed: %v", err)
	}

	cmdGoGrp := exec.Command(goBin, "totals", "--by", "model", "--format", "json", "--dir", tmpDir)
	outGoGrp, err := cmdGoGrp.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI groups failed: %v", err)
	}

	var nodeGrp, goGrp QueryOutput
	_ = json.Unmarshal(outNodeGrp, &nodeGrp)
	_ = json.Unmarshal(outGoGrp, &goGrp)
	if len(nodeGrp.Rows) != len(goGrp.Rows) || len(goGrp.Rows) != 1 {
		t.Fatalf("Groups length mismatch: Node=%d, Go=%d", len(nodeGrp.Rows), len(goGrp.Rows))
	}

	// 5. 全量 Period 对账
	cmdNodePer := exec.Command("node", "src/cli.ts", "totals", "--period", "day", "--format", "json", "--dir", tmpDir)
	cmdNodePer.Dir = repoRoot
	outNodePer, err := cmdNodePer.CombinedOutput()
	if err != nil {
		t.Fatalf("node CLI period failed: %v", err)
	}

	cmdGoPer := exec.Command(goBin, "totals", "--period", "day", "--format", "json", "--dir", tmpDir)
	outGoPer, err := cmdGoPer.CombinedOutput()
	if err != nil {
		t.Fatalf("go CLI period failed: %v", err)
	}

	var nodePer, goPer QueryOutput
	_ = json.Unmarshal(outNodePer, &nodePer)
	_ = json.Unmarshal(outGoPer, &goPer)
	if len(nodePer.Rows) != len(goPer.Rows) || len(goPer.Rows) != 1 {
		t.Fatalf("Period length mismatch: Node=%d, Go=%d", len(nodePer.Rows), len(goPer.Rows))
	}
}
