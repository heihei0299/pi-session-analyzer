package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// canonicalRoot mirrors the production flow: callers canonicalize a configured
// root once and then use only the pinned API.
func canonicalRoot(t *testing.T, root string) string {
	t.Helper()
	canonical, err := CanonicalSourceRoot(root)
	if err != nil {
		t.Fatalf("canonicalize root %q: %v", root, err)
	}
	return canonical
}

func TestSourceRootBindingIgnoresUnownedPiProxyRows(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.DB.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, latency_ms, status_code, created_at, data_source) VALUES ('shared-row', 'shared', 'pi', 'model', 0, 200, 0, 'proxy')`); err != nil {
		t.Fatal(err)
	}
	if err := BindPinnedSourceRoot(database, "pi", canonicalRoot(t, t.TempDir())); err != nil {
		t.Fatalf("unowned shared proxy rows must not block first Pi binding: %v", err)
	}
}

func TestSourceRootBindingUsesPhysicalIdentity(t *testing.T) {
	target := canonicalRoot(t, t.TempDir())
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}

	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := BindPinnedSourceRoot(database, "pi", target); err != nil {
		t.Fatal(err)
	}
	if canonicalRoot(t, link) != target {
		t.Fatalf("lexical symlink must canonicalize to the bound physical root")
	}
	if err := CheckPinnedSourceRoot(database, "pi", target); err != nil {
		t.Fatalf("the bound physical root must match itself: %v", err)
	}
}

func TestSourceRootBindingRejectsSymlinkTargetSwap(t *testing.T) {
	targetA := canonicalRoot(t, t.TempDir())
	targetB := canonicalRoot(t, t.TempDir())
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(targetA, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}

	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := BindPinnedSourceRoot(database, "pi", canonicalRoot(t, link)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Skipf("symlink target swap is not supported: %v", err)
	}
	if err := CheckPinnedSourceRoot(database, "pi", canonicalRoot(t, link)); !errors.Is(err, ErrSourceRootMismatch) {
		t.Fatalf("changing a symlink target must reject the root: %v", err)
	}
}

func TestPinnedBindingRejectsNonCanonicalRoot(t *testing.T) {
	target := canonicalRoot(t, t.TempDir())
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := BindPinnedSourceRoot(database, "pi", link); !errors.Is(err, ErrSourceRootMismatch) {
		t.Fatalf("a lexical symlink root must not be pinned without canonicalization: %v", err)
	}
	if err := CheckPinnedSourceRoot(database, "pi", link); !errors.Is(err, ErrSourceRootMismatch) {
		t.Fatalf("pinned checks must reject a lexical symlink root: %v", err)
	}
}
