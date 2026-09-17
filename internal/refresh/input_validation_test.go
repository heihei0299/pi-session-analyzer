package refresh

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestRefreshRejectsUnavailablePiRootBeforeOpeningLedger(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TOKEN_ANALYZER_DB", "")
	fileRoot := filepath.Join(t.TempDir(), "pi-file")
	if err := os.WriteFile(fileRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := []string{
		"",
		filepath.Join(t.TempDir(), "missing"),
		fileRoot,
	}
	dangling := filepath.Join(t.TempDir(), "pi-dangling")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing-target"), dangling); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	roots = append(roots, dangling)

	for _, root := range roots {
		dbPath := filepath.Join(t.TempDir(), "ledger.db")
		err := Refresh(Config{PiDir: root, DBPath: dbPath, Source: "pi"})
		if !errors.Is(err, db.ErrSourceRootUnavailable) {
			t.Fatalf("root %q must be rejected as unavailable: %v", root, err)
		}
		if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
			t.Fatalf("unavailable root must not create a ledger: stat error=%v", statErr)
		}
	}
}

func TestRefreshRejectsUnknownSourceBeforeOpeningLedger(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	err := Refresh(Config{PiDir: t.TempDir(), DBPath: dbPath, Source: "typo"})
	if err == nil {
		t.Fatal("unknown source must be rejected")
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("invalid refresh must not create a ledger: stat error=%v", statErr)
	}
}
