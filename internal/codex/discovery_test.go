package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRollout(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 真实 Codex home 的布局：rollout 命名不含 Z 后缀、按年/月/日分目录，sessions 与 archived_sessions 都可能有记录。
func TestDiscoverRolloutsFindsRealWorldLayoutAndPrefersPlainSibling(t *testing.T) {
	home := t.TempDir()
	plain := filepath.Join(home, "sessions", "2026", "09", "09", "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001.jsonl")
	compressed := plain + ".zst"
	reverted := filepath.Join(home, "sessions", "2026", "09", "09", "rollout-2026-09-09T18-46-00-00000000-0000-7000-8000-000000000001_00000000-0000-7000-8000-000000000002.jsonl")
	archived := filepath.Join(home, "archived_sessions", "2026", "09", "07", "rollout-2026-09-07T10-00-00-00000000-0000-7000-8000-000000000003.jsonl")
	distractor := filepath.Join(home, "sessions", "2026", "09", "09", "notes.jsonl")
	for _, path := range []string{plain, compressed, reverted, archived, distractor} {
		writeRollout(t, path)
	}

	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 logical rollouts (plain sibling collapses the compressed one), got %d: %+v", len(files), files)
	}
	if files[0].Path != archived || files[1].Path != plain || files[2].Path != reverted {
		t.Fatalf("expected sorted archived/plain/reverted paths, got %+v", files)
	}
	if files[1].Compressed {
		t.Fatalf("plain sibling must win: %+v", files[1])
	}
	if files[1].PhysicalID != plain {
		t.Fatalf("plain physical id must be the plain path: %+v", files[1])
	}
	if diagnostics.Skipped != 1 {
		t.Fatalf("expected exactly one skip diagnostic for the non-canonical file: %+v", diagnostics)
	}
	if !strings.Contains(strings.Join(diagnostics.Warnings, "\n"), "notes.jsonl") {
		t.Fatalf("skip diagnostic should name the skipped file: %+v", diagnostics.Warnings)
	}
}

func TestDiscoverRolloutsKeepsCompressedOnlyRollout(t *testing.T) {
	home := t.TempDir()
	compressed := filepath.Join(home, "sessions", "2026", "09", "09", "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001.jsonl.zst")
	writeRollout(t, compressed)

	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !files[0].Compressed {
		t.Fatalf("compressed-only rollout must be discovered as compressed: %+v", files)
	}
	if files[0].PhysicalID != strings.TrimSuffix(compressed, ".zst") {
		t.Fatalf("compressed physical id must drop the compression suffix: %+v", files[0])
	}
	if diagnostics.Skipped != 0 {
		t.Fatalf("compressed-only rollout must not be reported as skipped: %+v", diagnostics)
	}
}

// canonical 命名 grammar：前缀 + 秒级时间戳（日期与时间均以 '-' 分隔，上游写的是本地墙体时间）+ thread id，
// 可选 '_<rollout id>'（revert 形态），可选 '.zst'；历史形态的秒后 'Z' 继续被接受。
func TestDiscoverRolloutsRolloutNameGrammar(t *testing.T) {
	cases := []struct {
		name                 string
		accept               bool
		expectSkipDiagnostic bool // 被拒的 rollout 产物必须计入 skip 诊断，而不是静默消失
	}{
		{name: "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001.jsonl", accept: true},
		{name: "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001.jsonl.zst", accept: true},
		{name: "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001_00000000-0000-7000-8000-000000000002.jsonl", accept: true},
		{name: "rollout-2026-09-08T12-00-00Z-thread-fixture.jsonl", accept: true},
		{name: "rollout-2026-09-08T12-00-00Z-thread-fixture_revert-1.jsonl", accept: true},
		{name: "ignore.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-09-09T18-45-16.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-09-09T18-45-16-.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-13-45-16-00000000-0000-7000-8000-000000000001.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-not-a-timestamp-00000000-0000-7000-8000-000000000001.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-09-09T18:45:16-00000000-0000-7000-8000-000000000001.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-09-09T18-45-16-thread_a_b.jsonl", expectSkipDiagnostic: true},
		{name: "rollout-2026-09-09T18-45-16-00000000-0000-7000-8000-000000000001.jsonl.gz", expectSkipDiagnostic: true},
		{name: "README.md", expectSkipDiagnostic: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, filepath.Join(home, "sessions", tc.name))
			files, diagnostics, err := DiscoverRollouts(home)
			if err != nil {
				t.Fatal(err)
			}
			if tc.accept {
				if len(files) != 1 {
					t.Fatalf("expected %q to be canonical, got %+v", tc.name, files)
				}
				if diagnostics.Skipped != 0 {
					t.Fatalf("accepted rollout must not be reported as skipped: %+v", diagnostics)
				}
				return
			}
			if len(files) != 0 {
				t.Fatalf("expected %q to be rejected, got %+v", tc.name, files)
			}
			if tc.expectSkipDiagnostic && diagnostics.Skipped != 1 {
				t.Fatalf("rejected rollout artifact must be reported as skipped: %+v", diagnostics)
			}
			if !tc.expectSkipDiagnostic && diagnostics.Skipped != 0 {
				t.Fatalf("unrelated file must stay silent: %+v", diagnostics)
			}
		})
	}
}

// 目录存在但没有任何 rollout 根目录时，用户必须看到警告而不是一个静默的空窗口。
func TestPathWithinUsesPathComponents(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".codex")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "root", path: root, want: true},
		{name: "child", path: filepath.Join(root, "sessions", "rollout.jsonl"), want: true},
		{name: "sibling prefix", path: root + "-old", want: false},
		{name: "parent", path: filepath.Join(root, ".."), want: false},
		{name: "sibling", path: filepath.Join(filepath.Dir(root), "other"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathWithin(root, tc.path); got != tc.want {
				t.Fatalf("PathWithin(%q, %q) = %v, want %v", root, tc.path, got, tc.want)
			}
		})
	}
}

func TestDiscoverRolloutsWarnsWhenRolloutRootsAreMissing(t *testing.T) {
	home := t.TempDir()
	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatalf("empty codex home must not be a server error: %v", err)
	}
	if len(files) != 0 || diagnostics.Skipped != 0 {
		t.Fatalf("empty codex home must discover nothing: files=%+v diagnostics=%+v", files, diagnostics)
	}
	if len(diagnostics.Warnings) != 1 || !strings.Contains(diagnostics.Warnings[0], "sessions") {
		t.Fatalf("empty codex home must warn about missing rollout roots: %+v", diagnostics)
	}
}
