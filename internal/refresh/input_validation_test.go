package refresh

import (
	"os"
	"path/filepath"
	"testing"
)

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
