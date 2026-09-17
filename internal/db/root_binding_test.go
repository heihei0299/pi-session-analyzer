package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
