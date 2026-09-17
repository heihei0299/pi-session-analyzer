package pi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/heihei0299/token-analyzer/internal/db"
)

// Diagnostics describes source problems that did not invalidate other records.
// Source is persisted so Pi and Codex summaries cannot be mixed in a shared DB.
type Diagnostics struct {
	Source   string   `json:"source,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Skipped  int      `json:"skipped,omitempty"`
}

func (d *Diagnostics) warn(format string, args ...any) {
	d.Warnings = append(d.Warnings, fmt.Sprintf(format, args...))
}

func mergeDiagnostics(target, source *Diagnostics) {
	target.Warnings = append(target.Warnings, source.Warnings...)
	target.Skipped += source.Skipped
}

func decodeDiagnostics(summary string) (Diagnostics, error) {
	var diagnostics Diagnostics
	if strings.TrimSpace(summary) == "" {
		return diagnostics, nil
	}
	if err := json.Unmarshal([]byte(summary), &diagnostics); err != nil {
		return Diagnostics{}, err
	}
	return diagnostics, nil
}

// LoadDiagnostics replays Pi file summaries for ledger-only Query metadata.
func LoadDiagnostics(database *db.Database, root string) (Diagnostics, error) {
	var out Diagnostics
	rows, err := database.DB.Query(`SELECT file_path, diagnostics_summary FROM session_log_sync`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var path, summary string
		if err := rows.Scan(&path, &summary); err != nil {
			return out, err
		}
		if strings.TrimSpace(summary) == "" || !piPathWithin(root, path) {
			continue
		}
		diagnostics, err := decodeDiagnostics(summary)
		if err != nil {
			return out, fmt.Errorf("decode Pi diagnostics for %s: %w", path, err)
		}
		if diagnostics.Source != "" && diagnostics.Source != "pi" {
			continue
		}
		mergeDiagnostics(&out, &diagnostics)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func piPathWithin(root, candidate string) bool {
	rootAbs, err := canonicalPath(root)
	if err != nil {
		return false
	}
	candidateAbs, err := canonicalPath(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	return absolute, nil
}

// persistFileDiagnostics updates only the diagnostic summary; an existing
// cursor remains unchanged so the failed file is retried on the next refresh.
func persistFileDiagnostics(database *db.Database, path string, diagnostics Diagnostics) error {
	var storedSummary sql.NullString
	err := database.DB.QueryRow(`SELECT diagnostics_summary FROM session_log_sync WHERE file_path = ?`, path).Scan(&storedSummary)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		stored, decodeErr := decodeDiagnostics(storedSummary.String)
		if decodeErr != nil {
			return fmt.Errorf("decode existing diagnostics for %s: %w", path, decodeErr)
		}
		if stored.Source != "" && stored.Source != diagnostics.Source {
			return nil
		}
		mergeDiagnostics(&stored, &diagnostics)
		if stored.Source == "" {
			stored.Source = diagnostics.Source
		}
		diagnostics = stored
	}
	summary, err := json.Marshal(diagnostics)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec(`INSERT INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, sync_semantics_version, diagnostics_summary) VALUES (?, 0, 0, 0, 0, 0, 0, ?) ON CONFLICT(file_path) DO UPDATE SET diagnostics_summary = excluded.diagnostics_summary`, path, string(summary))
	return err
}
