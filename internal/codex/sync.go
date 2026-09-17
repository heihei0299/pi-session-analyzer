package codex

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

type SyncResult struct {
	Imported    int         `json:"imported"`
	Skipped     int         `json:"skipped"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

func SyncRollouts(database *db.Database, home string) (SyncResult, error) {
	files, discoveryDiagnostics, err := DiscoverRollouts(home)
	if err != nil {
		return SyncResult{}, err
	}
	discoveryOnly := discoveryDiagnostics
	discoveryOnly.Warnings = append([]string(nil), discoveryDiagnostics.Warnings...)
	result := SyncResult{Diagnostics: discoveryDiagnostics}
	var failures []error
	for _, file := range files {
		rev, err := rolloutRevision(file.Path)
		if err != nil {
			diagnostics := Diagnostics{Source: "codex"}
			diagnostics.Skipped++
			diagnostics.warn("无法读取 %s: %v", file.Path, err)
			mergeDiagnostics(&result.Diagnostics, &diagnostics)
			failures = append(failures, fmt.Errorf("读取 %s: %w", file.Path, err))
			if persistErr := persistFileDiagnostics(database, file.Path, diagnostics); persistErr != nil {
				failures = append(failures, persistErr)
			}
			continue
		}
		unchanged, stored, err := cursorState(database.DB, file.Path, rev)
		if err != nil {
			return result, fmt.Errorf("读取 %s 游标失败: %w", file.Path, err)
		}
		if unchanged {
			// 游标命中即不重扫，但该 revision 的 per-file 诊断必须重放：
			// 覆盖率诊断若只在首次解析时出现，用户第二次查询就看不到漏算了。
			mergeDiagnostics(&result.Diagnostics, &stored)
			continue
		}

		parsed, fileDiagnostics, err := ParseRollout(file)
		fileDiagnostics.Source = "codex"
		if err != nil {
			fileDiagnostics.Skipped++
			fileDiagnostics.warn("无法解析 %s: %v", file.Path, err)
			mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
			failures = append(failures, fmt.Errorf("解析 %s: %w", file.Path, err))
			if persistErr := persistFileDiagnostics(database, file.Path, fileDiagnostics); persistErr != nil {
				failures = append(failures, persistErr)
			}
			continue
		}

		tx, err := database.DB.Begin()
		if err != nil {
			return result, fmt.Errorf("开始同步 %s 事务失败: %w", file.Path, err)
		}
		imported, skipped, err := syncParsedRollout(tx, file, parsed, rev, &fileDiagnostics)
		if err != nil {
			_ = tx.Rollback()
			mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
			failures = append(failures, fmt.Errorf("同步 %s 失败: %w", file.Path, err))
			if persistErr := persistFileDiagnostics(database, file.Path, fileDiagnostics); persistErr != nil {
				failures = append(failures, persistErr)
			}
			continue
		}
		if err := tx.Commit(); err != nil {
			mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
			failures = append(failures, fmt.Errorf("提交 %s 失败: %w", file.Path, err))
			if persistErr := persistFileDiagnostics(database, file.Path, fileDiagnostics); persistErr != nil {
				failures = append(failures, persistErr)
			}
			continue
		}
		result.Imported += imported
		result.Skipped += skipped
		mergeDiagnostics(&result.Diagnostics, &fileDiagnostics)
	}
	// Home 级 discovery 与文件级失败诊断落盘：Query 只读 ledger 不做 discovery，
	// meta 警告靠这些摘要在纯 ledger 查询中重放。
	persisted := discoveryOnly
	persisted.Source = "codex"
	for _, failure := range failures {
		persisted.warn("同步失败: %v", failure)
	}
	if err := persistDiscoveryDiagnostics(database, home, &persisted); err != nil {
		failures = append(failures, err)
	}
	if len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}

func syncParsedRollout(tx *sql.Tx, file RolloutFile, parsed ParsedRollout, rev fileRevision, diagnostics *Diagnostics) (int, int, error) {
	meta := parsed.Meta
	if err := upsertSourceSession(tx, file, meta); err != nil {
		return 0, 0, err
	}
	// Plain and zstd representations share a physical rollout. Remove a stale
	// cursor for the other representation so old failure diagnostics disappear
	// after a successful retry.
	if _, err := tx.Exec(`DELETE FROM session_log_sync WHERE file_path IN (?, ?) AND file_path <> ?`, file.PhysicalID, file.PhysicalID+".zst", file.Path); err != nil {
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
