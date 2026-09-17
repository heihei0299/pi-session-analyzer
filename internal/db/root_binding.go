package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrSourceRootMismatch        = errors.New("source root mismatch")
	ErrSourceRootBindingMissing  = errors.New("source root binding missing")
	ErrSourceRootBindingRequired = errors.New("source root binding required")
	ErrSourceRootUnavailable     = errors.New("source root unavailable")
)

// CanonicalSourceRoot validates a configured root and returns its physical path.
func CanonicalSourceRoot(root string) (string, error) {
	return sourceRootIdentity(root)
}

func HasPiHistory(database *Database) (bool, error) {
	return hasPiHistory(database)
}

func validatePinnedRoot(pinnedRoot string) error {
	if strings.TrimSpace(pinnedRoot) == "" {
		return fmt.Errorf("%w: empty root", ErrSourceRootUnavailable)
	}
	if !filepath.IsAbs(pinnedRoot) || filepath.Clean(pinnedRoot) != pinnedRoot {
		return fmt.Errorf("%w: pinned root must be canonical absolute: %q", ErrSourceRootUnavailable, pinnedRoot)
	}
	info, err := os.Stat(pinnedRoot)
	if err != nil {
		return fmt.Errorf("%w: stat %q: %v", ErrSourceRootUnavailable, pinnedRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %q is not a directory", ErrSourceRootUnavailable, pinnedRoot)
	}
	resolved, err := filepath.EvalSymlinks(pinnedRoot)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %v", ErrSourceRootUnavailable, pinnedRoot, err)
	}
	if filepath.Clean(resolved) != pinnedRoot {
		return fmt.Errorf("%w: pinned root changed %q -> %q", ErrSourceRootMismatch, pinnedRoot, resolved)
	}
	return nil
}

func BindPinnedSourceRoot(database *Database, source, pinnedRoot string) error {
	if err := validatePinnedRoot(pinnedRoot); err != nil {
		return err
	}
	var bound string
	err := database.DB.QueryRow(`SELECT root_path FROM source_root_bindings WHERE data_source = ?`, source).Scan(&bound)
	if err == sql.ErrNoRows {
		if source == "pi" {
			hasHistory, err := hasPiHistory(database)
			if err != nil {
				return fmt.Errorf("check existing Pi history: %w", err)
			}
			if hasHistory {
				return fmt.Errorf("%w: Pi history has no root binding; explicit migration or a new ledger is required", ErrSourceRootBindingRequired)
			}
		}
		if _, err := database.DB.Exec(`INSERT INTO source_root_bindings (data_source, root_path) VALUES (?, ?)`, source, pinnedRoot); err != nil {
			return fmt.Errorf("bind %s source root: %w", source, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s source root: %w", source, err)
	}
	if bound != pinnedRoot {
		return sourceRootMismatch(source, bound, pinnedRoot)
	}
	return nil
}

func CheckPinnedSourceRoot(database *Database, source, pinnedRoot string) error {
	if err := validatePinnedRoot(pinnedRoot); err != nil {
		return err
	}
	var bound string
	err := database.DB.QueryRow(`SELECT root_path FROM source_root_bindings WHERE data_source = ?`, source).Scan(&bound)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s ledger has no binding", ErrSourceRootBindingMissing, source)
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("%w: %s ledger has no binding table", ErrSourceRootBindingMissing, source)
		}
		return fmt.Errorf("read %s source root: %w", source, err)
	}
	if bound != pinnedRoot {
		return sourceRootMismatch(source, bound, pinnedRoot)
	}
	return nil
}

func hasPiHistory(database *Database) (bool, error) {
	var exists int
	err := database.DB.QueryRow(`
		SELECT CASE WHEN
			EXISTS (SELECT 1 FROM pi_sessions LIMIT 1)
			OR EXISTS (SELECT 1 FROM proxy_request_logs WHERE app_type = 'pi' AND data_source = 'pi_session' LIMIT 1)
			OR EXISTS (SELECT 1 FROM usage_daily_rollups WHERE app_type = 'pi' LIMIT 1)
		THEN 1 ELSE 0 END`).Scan(&exists)
	return exists == 1, err
}

func sourceRootMismatch(source, bound, requested string) error {
	return fmt.Errorf("%w: %s ledger is bound to %q, requested %q", ErrSourceRootMismatch, source, bound, requested)
}

func sourceRootIdentity(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("%w: empty root", ErrSourceRootUnavailable)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %v", ErrSourceRootUnavailable, root, err)
	}
	root = filepath.Clean(absolute)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("%w: stat %q: %v", ErrSourceRootUnavailable, root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", ErrSourceRootUnavailable, root)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %v", ErrSourceRootUnavailable, root, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %v", ErrSourceRootUnavailable, root, err)
	}
	return filepath.Clean(resolved), nil
}
