package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func assertNoMatch(t *testing.T, rel, source string, substrs []string) {
	t.Helper()
	lower := strings.ToLower(source)
	for _, s := range substrs {
		if strings.Contains(lower, strings.ToLower(s)) {
			t.Fatalf("%s must not contain %q (Go-only: no OpenCode runtime)", rel, s)
		}
	}
}

// TestGoOnlyNoOpenCodeRuntime 确认 token-analyzer 不暴露或依赖 OpenCode runtime。
// OpenCode Analyzer 已迁出，本仓库只维护 token-analyzer。
func TestGoOnlyNoOpenCodeRuntime(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"cmd/token-analyzer/main.go",
		"internal/server/server.go",
		"internal/server/webui.html",
	} {
		src := readRepoFile(t, root, rel)
		assertNoMatch(t, rel, src, []string{
			"api/opencode", "handleOpencode", "opencode sync", `data-tab="opencode"`,
			"OPENCODE_AUTH", "OPENCODE_WORKSPACE", "OPENCODE_DATA_DIR",
		})
	}
}

// TestGoOnlyDeletionContract 是 05 的最终 deletion test：
// 无 TS backend、无旧聚合生产路径、无 Codex 回绕、无 Watch 独立统计、无 WebUI 双副本、无运行时耦合。
func TestGoOnlyDeletionContract(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "opencode-analyzer")); !os.IsNotExist(err) {
		t.Fatalf("Go-only contract violated: opencode-analyzer/ must be maintained outside this repository")
	}
	// 无 TS backend：生产 TS 与 Node 构建链不得存在。
	for _, rel := range []string{
		"src/cli.ts", "src/api.ts", "src/server.ts", "src/db.ts",
		"src/session-data.ts", "src/watch.ts", "src/pi-sync.ts", "src/db-aggregation.ts",
		"package.json", "package-lock.json", "tsconfig.json", "tsconfig.build.json",
		".github/workflows/publish.yml",
		"test/parity_test.go", "test/helpers.ts",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("Go-only contract violated: %s must not exist", rel)
		}
	}
	if ents, err := os.ReadDir(filepath.Join(root, "src")); err == nil {
		var leftovers []string
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".ts") {
				leftovers = append(leftovers, e.Name())
			}
		}
		if len(leftovers) > 0 {
			t.Fatalf("Go-only contract violated: src/ still has TS: %v", leftovers)
		}
	}
	// 无 WebUI 双副本：唯一人工维护源为 internal/server/webui.html。
	if _, err := os.Stat(filepath.Join(root, "src", "webui.html")); !os.IsNotExist(err) {
		t.Fatal("Go-only contract violated: src/webui.html second copy must not exist")
	}
	// 无 Watch 独立统计模块。
	if _, err := os.Stat(filepath.Join(root, "internal", "watch")); !os.IsNotExist(err) {
		t.Fatal("Go-only contract violated: internal/watch must not exist (Watch is change→refresh→query)")
	}
	// 生产 Go 不得回绕 Codex SessionFileData（LoadSessionFiles 已删除）。
	// 生产 Query 不得调用旧文件扫描聚合（ReadSessionFilesCached/AnalyzeFile）。
	for _, dir := range []string{"internal/query", "internal/server", "internal/refresh", "cmd/token-analyzer"} {
		ents, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			for _, sym := range []string{"LoadSessionFiles", "ReadSessionFilesCached", "AnalyzeFile"} {
				if strings.Contains(s, sym) {
					t.Fatalf("Go-only contract violated: %s/%s must not use %s in production", dir, e.Name(), sym)
				}
			}
		}
	}
	// module 统一为当前项目名。
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "module github.com/heihei0299/token-analyzer") {
		t.Fatalf("go.mod must be github.com/heihei0299/token-analyzer, got:\n%s", mod)
	}
}
