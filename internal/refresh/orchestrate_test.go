package refresh

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRefreshSerializesConcurrentCalls(t *testing.T) {
	piDir := t.TempDir()
	piProject := filepath.Join(piDir, "project")
	if err := os.MkdirAll(piProject, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(piProject, "s.jsonl"), []byte("{\"type\":\"session\",\"id\":\"s\",\"timestamp\":\"2026-09-10T00:00:00Z\",\"cwd\":\"/w\"}\n{\"type\":\"message\",\"id\":\"a1\",\"timestamp\":\"2026-09-10T01:00:00Z\",\"message\":{\"role\":\"assistant\",\"model\":\"m\",\"usage\":{\"input\":1,\"output\":1},\"stopReason\":\"stop\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: piDir, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "pi"}
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = Refresh(cfg)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := LastResult(); got.Err != nil {
		t.Fatalf("last result must record success: %+v", got)
	}
}

func TestRefreshFailureKeepsResult(t *testing.T) {
	notDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: t.TempDir(), CodexDir: notDir, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "codex"}
	if err := Refresh(cfg); err == nil {
		t.Fatal("codex refresh on a non-directory must fail")
	}
	got := LastResult()
	if got.Err == nil {
		t.Fatal("last result must record the failure for meta exposure")
	}
}

func TestTokenAnalyzerDbEnvIsHonored(t *testing.T) {
	envDB := filepath.Join(t.TempDir(), "env-ledger.db")
	t.Setenv("TOKEN_ANALYZER_DB", envDB)
	piDir := t.TempDir()
	piProject := filepath.Join(piDir, "project")
	if err := os.MkdirAll(piProject, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(piProject, "s.jsonl"), []byte("{\"type\":\"session\",\"id\":\"s\",\"timestamp\":\"2026-09-10T00:00:00Z\",\"cwd\":\"/w\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// DBPath 为空时走 TOKEN_ANALYZER_DB（与 TS oracle 一致），且不建默认库。
	if err := Refresh(Config{PiDir: piDir, Source: "pi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(envDB); err != nil {
		t.Fatalf("refresh must create the env ledger: %v", err)
	}
}

func TestFingerprintChangesOnWrite(t *testing.T) {
	piDir := t.TempDir()
	codexDir := t.TempDir()
	before, err := Fingerprint(piDir, codexDir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(piDir, "s.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := Fingerprint(piDir, codexDir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("fingerprint must change when a source file appears")
	}
	again, err := Fingerprint(piDir, codexDir)
	if err != nil {
		t.Fatal(err)
	}
	if after != again {
		t.Fatal("fingerprint must be stable without changes")
	}
}
