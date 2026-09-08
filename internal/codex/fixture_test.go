package codex

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
)

func TestSyntheticFixtureCoversPlainArchiveAndRevertedRollouts(t *testing.T) {
	codexHome := filepath.Join("testdata", "codex-home")
	files, discoveryDiagnostics, err := DiscoverRollouts(codexHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || discoveryDiagnostics.Skipped != 0 {
		t.Fatalf("unexpected synthetic fixture discovery: files=%+v diagnostics=%+v", files, discoveryDiagnostics)
	}

	parsedByName := make(map[string]ParsedRollout, len(files))
	diagnosticsByName := make(map[string]Diagnostics, len(files))
	for _, file := range files {
		parsed, diagnostics, err := ParseRollout(file)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Usage) == 0 {
			t.Fatalf("fixture rollout has no reliable usage: %s", file.Path)
		}
		name := filepath.Base(file.Path)
		parsedByName[name] = parsed
		diagnosticsByName[name] = diagnostics
	}

	plainName := "rollout-2026-09-08T12-00-00Z-thread-fixture.jsonl"
	plain, ok := parsedByName[plainName]
	if !ok {
		t.Fatalf("plain fixture missing: %s", plainName)
	}
	if plain.Meta.SessionID != "fixture-session" || plain.Meta.ThreadID != "fixture-thread" || plain.Meta.ForkedFromID != "base-thread" || plain.Meta.ForkedFromOrdinal == nil || *plain.Meta.ForkedFromOrdinal != 4 || plain.Meta.ThreadSource != "subagent" {
		t.Fatalf("fork metadata not mapped: %+v", plain.Meta)
	}
	if len(plain.Usage) != 2 || plain.Usage[0].ResponseID != "fixture-response-1" || plain.Usage[1].ResponseID != "fixture-response-1" || plain.Usage[0].Model != "fixture-model" || plain.Usage[1].Model != "fixture-model" {
		t.Fatalf("duplicate fixture usage not preserved for sync dedup: %+v", plain.Usage)
	}
	plainWarnings := strings.Join(diagnosticsByName[plainName].Warnings, "\n")
	if diagnosticsByName[plainName].Skipped != 3 || !strings.Contains(plainWarnings, "缺少可靠 usage") || !strings.Contains(plainWarnings, "model reroute") || !strings.Contains(plainWarnings, "未知 Codex event type") || !strings.Contains(plainWarnings, "坏 JSON 行") {
		t.Fatalf("plain fixture diagnostics incomplete: %+v", diagnosticsByName[plainName])
	}

	archiveName := "rollout-2026-09-07T12-00-00Z-thread-archive_fixture-1.jsonl"
	archive, ok := parsedByName[archiveName]
	if !ok || len(archive.Usage) != 1 || archive.Usage[0].ResponseID != "archive-response-1" || archive.Usage[0].Model != "unknown" {
		t.Fatalf("archive fixture mapping incomplete: %+v", archive)
	}

	revertedName := "rollout-2026-09-08T12-01-00Z-thread-fixture_revert-1.jsonl"
	reverted, ok := parsedByName[revertedName]
	if !ok {
		t.Fatalf("reverted fixture missing: %s", revertedName)
	}
	if reverted.File.PhysicalID == plain.File.PhysicalID || reverted.Meta.SessionID != "fixture-session" || reverted.Meta.ThreadID != "fixture-thread" || reverted.Meta.HistoryBase != "fixture-history-base" || len(reverted.Usage) != 1 || reverted.Usage[0].ResponseID != "fixture-revert-response" {
		t.Fatalf("reverted rollout identity or metadata incomplete: %+v", reverted)
	}

	database, err := db.Open(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	syncResult, err := SyncRollouts(database, codexHome)
	if err != nil {
		t.Fatal(err)
	}
	if syncResult.Imported != 3 || syncResult.Diagnostics.Conflicts != 1 || !strings.Contains(strings.Join(syncResult.Diagnostics.Warnings, "\n"), "冲突") {
		t.Fatalf("fixture sync diagnostics incomplete: %+v", syncResult)
	}
}
