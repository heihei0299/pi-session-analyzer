package codex

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func DiscoverySummaryPath(home string) string {
	return filepath.Join(home, ".token-analyzer-discovery")
}

// LoadDiagnostics 重放 home 作用域内的全部诊断（discovery 行 + 各文件行），
// 供 ledger-only 查询拼 meta。表结构与摘要形态归 adapter 所有，query 只调这一处。
func LoadDiagnostics(database *db.Database, home string) (Diagnostics, error) {
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
		if strings.TrimSpace(summary) == "" || (path != DiscoverySummaryPath(home) && !PathWithin(home, path)) {
			continue
		}
		var d Diagnostics
		if err := json.Unmarshal([]byte(summary), &d); err != nil {
			return out, fmt.Errorf("decode diagnostics for %s: %w", path, err)
		}
		if d.Source != "" && d.Source != "codex" {
			continue
		}
		mergeDiagnostics(&out, &d)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func persistDiscoveryDiagnostics(database *db.Database, home string, diagnostics *Diagnostics) error {
	summary, err := json.Marshal(diagnostics)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec(`INSERT INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, sync_semantics_version, diagnostics_summary) VALUES (?, 0, 0, ?, 0, 0, 0, ?) ON CONFLICT(file_path) DO UPDATE SET diagnostics_summary = excluded.diagnostics_summary`, DiscoverySummaryPath(home), time.Now().Unix(), string(summary))
	return err
}

// persistFileDiagnostics records a failure without changing an existing cursor.
// A missing row gets a zero cursor, so the next refresh still retries the file.
func persistFileDiagnostics(database *db.Database, path string, diagnostics Diagnostics) error {
	summary, err := json.Marshal(diagnostics)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec(`INSERT INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, sync_semantics_version, diagnostics_summary) VALUES (?, 0, 0, 0, 0, 0, 0, ?) ON CONFLICT(file_path) DO UPDATE SET diagnostics_summary = excluded.diagnostics_summary`, path, string(summary))
	return err
}

func mergeDiagnostics(target, source *Diagnostics) {
	target.Warnings = append(target.Warnings, source.Warnings...)
	target.Skipped += source.Skipped
	target.Conflicts += source.Conflicts
	target.UncountedSnapshots += source.UncountedSnapshots
}
