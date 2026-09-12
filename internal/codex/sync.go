package codex

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/timerange"
)

type SyncResult struct {
	Imported    int         `json:"imported"`
	Skipped     int         `json:"skipped"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

type fileRevision struct {
	modifiedMs    int64
	logicalSize   int64
	completeBytes int64
	completeLines int64
	tail          int64
}

func SyncRollouts(database *db.Database, home string) (SyncResult, error) {
	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		return SyncResult{}, err
	}
	discoveryOnly := diagnostics
	result := SyncResult{Diagnostics: diagnostics}
	for _, file := range files {
		rev, err := rolloutRevision(file.Path)
		if err != nil {
			result.Diagnostics.Skipped++
			result.Diagnostics.warn("无法读取 %s: %v", file.Path, err)
			continue
		}
		unchanged, stored, err := cursorState(database.DB, file.Path, rev)
		if err != nil {
			return result, err
		}
		if unchanged {
			// 游标命中即不重扫，但该 revision 的 per-file 诊断必须重放：
			// 覆盖率诊断若只在首次解析时出现，用户第二次查询就看不到漏算了。
			mergeDiagnostics(&result.Diagnostics, &stored)
			continue
		}

		parsed, fileDiagnostics, err := ParseRollout(file)
		if err != nil {
			result.Diagnostics.Skipped++
			fileDiagnostics.warn("无法解析 %s: %v", file.Path, err)
			mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
			continue
		}

		tx, err := database.DB.Begin()
		if err != nil {
			return result, err
		}
		imported, skipped, err := syncParsedRollout(tx, file, parsed, rev, &fileDiagnostics)
		if err != nil {
			_ = tx.Rollback()
			return result, fmt.Errorf("同步 %s 失败: %w", file.Path, err)
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		result.Imported += imported
		result.Skipped += skipped
		mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
	}
	// Home 级 discovery 诊断落盘：Query 只读 ledger 不做 discovery，
	// meta 警告（非 canonical 文件、缺失目录等）靠该行在纯 ledger 查询中重放。
	persistDiscoveryDiagnostics(database, home, &discoveryOnly)
	return result, nil
}

// DiscoverySummaryPath 是 home 级 discovery 诊断在 session_log_sync 中的键，
// 永不与真实 rollout 文件路径冲突。Query 只读 ledger，靠该行重放 discovery 诊断。
func DiscoverySummaryPath(home string) string {
	return filepath.Join(home, ".token-analyzer-discovery")
}

// LoadDiagnostics 重放 home 作用域内的全部诊断（discovery 行 + 各文件行），
// 供 ledger-only 查询拼 meta。表结构与摘要形态归 adapter 所有，query 只调这一处。
func LoadDiagnostics(database *db.Database, home string) Diagnostics {
	var out Diagnostics
	rows, err := database.DB.Query(`SELECT file_path, diagnostics_summary FROM session_log_sync`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var path, summary string
		if err := rows.Scan(&path, &summary); err != nil {
			continue
		}
		if strings.TrimSpace(summary) == "" {
			continue
		}
		if path != DiscoverySummaryPath(home) && !strings.HasPrefix(path, home) {
			continue
		}
		var d Diagnostics
		if err := json.Unmarshal([]byte(summary), &d); err != nil {
			continue
		}
		mergeDiagnostics(&out, &d)
	}
	return out
}

func persistDiscoveryDiagnostics(database *db.Database, home string, diagnostics *Diagnostics) {
	summary, err := json.Marshal(diagnostics)
	if err != nil {
		return
	}
	_, _ = database.DB.Exec(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, diagnostics_summary) VALUES (?, 0, 0, ?, 0, 0, ?)`, DiscoverySummaryPath(home), time.Now().Unix(), string(summary))
}

func syncParsedRollout(tx *sql.Tx, file RolloutFile, parsed ParsedRollout, rev fileRevision, diagnostics *Diagnostics) (int, int, error) {
	meta := parsed.Meta
	if err := upsertSourceSession(tx, file, meta); err != nil {
		return 0, 0, err
	}
	imported, skipped := 0, 0
	for _, event := range parsed.Usage {
		requestID := "codex:" + event.ResponseID
		var oldInput, oldCacheRead, oldCacheWrite, oldOutput, oldReasoning int64
		var exists int
		err := tx.QueryRow(`SELECT input_tokens, cache_read_tokens, cache_creation_tokens, output_tokens, reasoning_tokens FROM proxy_request_logs WHERE request_id = ?`, requestID).Scan(&oldInput, &oldCacheRead, &oldCacheWrite, &oldOutput, &oldReasoning)
		if err == nil {
			skipped++
			if oldInput != int64(event.Usage.Input) || oldCacheRead != int64(event.Usage.CacheRead) || oldCacheWrite != int64(event.Usage.CacheWrite) || oldOutput != int64(event.Usage.Output) || oldReasoning != int64(event.Usage.Reasoning) {
				diagnostics.Conflicts++
				diagnostics.warn("Codex response %s payload 冲突，保留首条 usage", event.ResponseID)
			}
			continue
		}
		if err != sql.ErrNoRows {
			return 0, 0, err
		}
		if err := tx.QueryRow(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`, "codex", requestID).Scan(&exists); err == nil {
			skipped++
			continue
		} else if err != sql.ErrNoRows {
			return 0, 0, err
		}

		createdAt := int64(0)
		if ms, err := timerange.ParseUtcTimestamp(event.Timestamp); err == nil {
			createdAt = ms / 1000
		}
		sessionID := event.ThreadID
		if sessionID == "" {
			sessionID = meta.ThreadID
		}
		if sessionID == "" {
			sessionID = meta.SessionID
		}
		provider := event.Provider
		if provider == "" {
			provider = meta.ModelProvider
		}
		if provider == "" {
			provider = "unknown"
		}
		model := event.Model
		if model == "" {
			model = "unknown"
		}
		_, err = tx.Exec(`INSERT INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, 1)`, "codex", requestID, "codex-response:"+event.ResponseID)
		if err != nil {
			return 0, 0, err
		}
		_, err = tx.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source, physical_rollout_id, kind, reasoning_tokens, cwd, timestamp_text) VALUES (?, ?, 'codex', ?, ?, 'unpriced', ?, ?, ?, ?, 0, '0', 0, 200, NULL, ?, ?, 0, '1.0', ?, 'codex', ?, 'response', ?, ?, ?)`,
			requestID, provider, model, model, int64(event.Usage.Input), int64(event.Usage.Output), int64(event.Usage.CacheRead), int64(event.Usage.CacheWrite), sessionID, provider, createdAt, file.PhysicalID, int64(event.Usage.Reasoning), meta.Cwd, event.Timestamp)
		if err != nil {
			return 0, 0, err
		}
		imported++
	}
	summary, err := json.Marshal(diagnostics)
	if err != nil {
		return 0, 0, err
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, diagnostics_summary) VALUES (?, ?, ?, ?, ?, ?, ?)`, file.Path, rev.modifiedMs, rev.completeLines, time.Now().Unix(), rev.completeBytes, rev.tail, string(summary))
	if err != nil {
		return 0, 0, err
	}
	return imported, skipped, nil
}

func upsertSourceSession(tx *sql.Tx, file RolloutFile, meta SessionMeta) error {
	returned, err := tx.Exec(`INSERT OR REPLACE INTO source_sessions (data_source, physical_id, session_id, thread_id, timestamp_text, cwd, file_path, file_name, model, provider_id, originator, cli_version, parent_thread_id, forked_from_id, forked_from_ordinal, subagent_history_start_ordinal, history_base, thread_source, agent_role, agent_path, agent_nickname) VALUES ('codex', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.PhysicalID, meta.SessionID, meta.ThreadID, meta.Timestamp, meta.Cwd, file.Path, filepath.Base(file.Path), meta.Model, meta.ModelProvider, meta.Originator, meta.CliVersion, meta.ParentThreadID, meta.ForkedFromID, meta.ForkedFromOrdinal, meta.SubagentHistoryStartOrdinal, meta.HistoryBase, meta.ThreadSource, meta.AgentRole, meta.AgentPath, meta.AgentNickname)
	if err != nil {
		return err
	}
	_ = returned
	return nil
}

func rolloutRevision(path string) (fileRevision, error) {
	st, err := os.Stat(path)
	if err != nil {
		return fileRevision{}, err
	}
	data, err := readRolloutBytes(path)
	if err != nil {
		return fileRevision{}, err
	}
	completeBytes := int64(len(data))
	completeLines := int64(0)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		idx := strings.LastIndexByte(string(data), '\n')
		if idx < 0 {
			completeBytes = 0
		} else {
			completeBytes = int64(idx + 1)
		}
	}
	if completeBytes > 0 {
		completeLines = int64(strings.Count(string(data[:completeBytes]), "\n"))
	}
	return fileRevision{
		modifiedMs:    st.ModTime().UnixMilli(),
		logicalSize:   int64(len(data)),
		completeBytes: completeBytes,
		completeLines: completeLines,
		tail:          fingerprint(data),
	}, nil
}

func readRolloutBytes(path string) ([]byte, error) {
	reader, closeReader, err := openRollout(path)
	if err != nil {
		return nil, err
	}
	defer closeReader()
	return io.ReadAll(reader)
}

func fingerprint(data []byte) int64 {
	if len(data) > 4096 {
		data = data[len(data)-4096:]
	}
	sum := sha256.Sum256(data)
	var result int64
	for _, b := range sum[:8] {
		result = (result << 8) | int64(b)
	}
	return result
}

// cursorState 判定文件 revision 是否与游标一致；一致时一并返回该 revision 的 per-file 诊断摘要。
func cursorState(database *sql.DB, path string, rev fileRevision) (bool, Diagnostics, error) {
	var modified, offset, tail sql.NullInt64
	var summary sql.NullString
	err := database.QueryRow(`SELECT last_modified, last_byte_offset, last_tail_fingerprint, diagnostics_summary FROM session_log_sync WHERE file_path = ?`, path).Scan(&modified, &offset, &tail, &summary)
	if err == sql.ErrNoRows {
		return false, Diagnostics{}, nil
	}
	if err != nil {
		return false, Diagnostics{}, err
	}
	matched := modified.Valid && offset.Valid && tail.Valid && modified.Int64 == rev.modifiedMs && offset.Int64 == rev.completeBytes && tail.Int64 == rev.tail && rev.logicalSize == rev.completeBytes
	if !matched {
		return false, Diagnostics{}, nil
	}
	stored := Diagnostics{}
	if summary.Valid && strings.TrimSpace(summary.String) != "" {
		// 摘要是诊断通道，损坏时降级为「无诊断」，不能因此中断同步。
		_ = json.Unmarshal([]byte(summary.String), &stored)
	}
	return true, stored, nil
}

func mergeDiagnostics(target, source *Diagnostics) {
	target.Warnings = append(target.Warnings, source.Warnings...)
	target.Skipped += source.Skipped
	target.Conflicts += source.Conflicts
	target.UncountedSnapshots += source.UncountedSnapshots
}
