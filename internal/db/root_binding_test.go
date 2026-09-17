package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceRootBindingIgnoresUnownedPiProxyRows(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.DB.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, latency_ms, status_code, created_at, data_source) VALUES ('shared-row', 'shared', 'pi', 'model', 0, 200, 0, 'proxy')`); err != nil {
		t.Fatal(err)
	}
	if err := BindSourceRoot(database, "pi", t.TempDir()); err != nil {
		t.Fatalf("unowned shared proxy rows must not block first Pi binding: %v", err)
	}
}

func TestSourceRootBindingUsesPhysicalIdentity(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}

	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := BindSourceRoot(database, "pi", target); err != nil {
		t.Fatal(err)
	}
	if err := CheckSourceRoot(database, "pi", link); err != nil {
		t.Fatalf("lexical paths for one physical root must match: %v", err)
	}
}

func TestSourceRootBindingRejectsSymlinkTargetSwap(t *testing.T) {
	targetA := t.TempDir()
	targetB := t.TempDir()
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(targetA, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}

	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := BindSourceRoot(database, "pi", link); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Skipf("symlink target swap is not supported: %v", err)
	}
	if err := CheckSourceRoot(database, "pi", link); !errors.Is(err, ErrSourceRootMismatch) {
		t.Fatalf("changing a symlink target must reject the root: %v", err)
	}
}
