package pi

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/heihei0299/token-analyzer/internal/db"
)

const PiSyncSemanticsVersion = 2

type SyncResult struct {
	Imported    int
	Skipped     int
	Diagnostics Diagnostics
}

func SyncPiUsage(database *db.Database, files []string) (SyncResult, error) {
	var imported, skipped int
	diagnostics := Diagnostics{Source: "pi"}
	pricingByModel, err := loadModelPricing(database)
	if err != nil {
		return SyncResult{Diagnostics: diagnostics}, fmt.Errorf("读取 Pi 定价表失败: %w", err)
	}
	var failures []error
	for _, file := range files {
		fileDiagnostics := Diagnostics{Source: "pi"}
		recordFailure := func(cause error) {
			fileDiagnostics.Skipped++
			fileDiagnostics.warn("同步 %s 失败: %v", file, cause)
			mergeDiagnostics(&diagnostics, &fileDiagnostics)
			failures = append(failures, fmt.Errorf("同步 %s: %w", file, cause))
			if persistErr := persistFileDiagnostics(database, file, fileDiagnostics); persistErr != nil {
				failures = append(failures, persistErr)
			}
		}
		rev, err := PiFileRevisionOf(file)
		if err != nil {
			recordFailure(err)
			continue
		}
		var cursorLastByte int64
		var cursorLastLine int64
		var cursorTail uint32
		var hasCursor bool
		var canSeek bool
		var nByte, nTail, nLine, nSemantics sql.NullInt64
		var storedSummary sql.NullString
		err = database.DB.QueryRow(`SELECT last_byte_offset, last_tail_fingerprint, last_line_offset, sync_semantics_version, diagnostics_summary FROM session_log_sync WHERE file_path = ?`, file).Scan(&nByte, &nTail, &nLine, &nSemantics, &storedSummary)
		if err != nil && err != sql.ErrNoRows {
			return SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}, fmt.Errorf("读取 %s 游标失败: %w", file, err)
		}
		var storedDiagnostics Diagnostics
		if err == nil {
			storedDiagnostics, err = decodeDiagnostics(storedSummary.String)
			if err != nil {
				return SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}, fmt.Errorf("读取 %s 诊断失败: %w", file, err)
			}
		}
		if err == nil && storedDiagnostics.Source != "" && storedDiagnostics.Source != "pi" {
			recordFailure(fmt.Errorf("诊断摘要属于 %s source", storedDiagnostics.Source))
			continue
		}
		if err == nil && nByte.Valid && nTail.Valid && nSemantics.Valid && nSemantics.Int64 == PiSyncSemanticsVersion {
			cursorLastByte = nByte.Int64
			cursorTail = uint32(nTail.Int64)
			if nLine.Valid {
				cursorLastLine = nLine.Int64
			}
			hasCursor = true
			if rev.FileSize >= cursorLastByte {
				actual, err2 := tailFingerprintAtGo(file, cursorLastByte)
				if err2 == nil && actual == cursorTail {
					canSeek = true
				}
			}
		}
		if hasCursor && canSeek && rev.FileSize == cursorLastByte && rev.TailFingerprint == cursorTail {
			mergeDiagnostics(&diagnostics, &storedDiagnostics)
			continue
		}
		if hasCursor && canSeek {
			mergeDiagnostics(&fileDiagnostics, &storedDiagnostics)
		}
		var linesToProcess []string
		var newCommittedByte int64
		var newCommittedLines int64
		var newTail uint32
		var sessionID string
		var sessionTimestamp *int64
		var headerTs string
		var headerCwd string
		var parentSessionID *string
		var forkTsMs *int64
		if canSeek && hasCursor {
			// read header
			fh, err := os.Open(file)
			if err != nil {
				recordFailure(err)
				continue
			}
			br := bufio.NewReader(fh)
			hl, err := br.ReadString('\n')
			fh.Close()
			if err != nil && len(hl) == 0 {
				recordFailure(err)
				continue
			}
			headerLine := strings.TrimSpace(hl)
			if headerLine == "" {
				recordFailure(fmt.Errorf("empty session header"))
				continue
			}
			var header map[string]interface{}
			if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
				recordFailure(fmt.Errorf("invalid session header: %w", err))
				continue
			}
			if header["type"] != "session" {
				recordFailure(fmt.Errorf("invalid session header type"))
				continue
			}
			if v, ok := header["id"].(string); ok {
				sessionID = v
			}
			if v, ok := header["timestamp"].(string); ok {
				headerTs = v
				if t, err := parseTimestampGo(v); err == nil {
					sessionTimestamp = &t
				}
			}
			if v, ok := header["cwd"].(string); ok {
				headerCwd = v
			}
			if v, ok := header["parentSession"].(string); ok && v != "" {
				parentSessionID = &v
				// fork 去重（与 TS oracle 同语义）：parentSession 为文件路径形态时，
				// 早于 fork 点的消息是复制历史，不计入账本。
				if strings.Contains(v, "/") || strings.Contains(v, "\\") {
					if ts, ok := header["timestamp"].(string); ok {
						if ms, err := parseEntryMs(ts); err == nil {
							forkTsMs = &ms
						}
					}
				}
			}
			suffixLen := rev.FileSize - cursorLastByte
			if suffixLen <= 0 {
				continue
			}
			f2, err := os.Open(file)
			if err != nil {
				recordFailure(err)
				continue
			}
			buf := make([]byte, suffixLen)
			_, err = f2.ReadAt(buf, cursorLastByte)
			f2.Close()
			if err != nil {
				recordFailure(err)
				continue
			}
			suffixStr := string(buf)
			parts := strings.Split(suffixStr, "\n")
			completeNewLines := int64(len(parts) - 1)
			if completeNewLines <= 0 {
				continue
			}
			linesToProcess = parts[:len(parts)-1]
			if rev.Complete {
				newCommittedByte = rev.FileSize
			} else {
				lastNL := strings.LastIndex(suffixStr, "\n")
				if lastNL != -1 {
					newCommittedByte = cursorLastByte + int64(lastNL) + 1
				} else {
					newCommittedByte = cursorLastByte
				}
			}
			newCommittedLines = cursorLastLine + completeNewLines
			nt, err := tailFingerprintAtGo(file, newCommittedByte)
			if err != nil {
				recordFailure(err)
				continue
			}
			newTail = nt
		} else {
			f, err := os.Open(file)
			if err != nil {
				recordFailure(err)
				continue
			}
			scanner := bufio.NewScanner(f)
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 10*1024*1024)
			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			f.Close()
			if scanErr := scanner.Err(); scanErr != nil {
				recordFailure(scanErr)
				continue
			}
			if len(lines) == 0 {
				recordFailure(fmt.Errorf("empty session file"))
				continue
			}
			var header map[string]interface{}
			if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
				recordFailure(fmt.Errorf("invalid session header: %w", err))
				continue
			}
			if header["type"] != "session" {
				recordFailure(fmt.Errorf("invalid session header type"))
				continue
			}
			if v, ok := header["id"].(string); ok {
				sessionID = v
			}
			if v, ok := header["timestamp"].(string); ok {
				headerTs = v
				if t, err := parseTimestampGo(v); err == nil {
					sessionTimestamp = &t
				}
			}
			if v, ok := header["cwd"].(string); ok {
				headerCwd = v
			}
			if v, ok := header["parentSession"].(string); ok && v != "" {
				parentSessionID = &v
				// fork 去重（与 TS oracle 同语义）：parentSession 为文件路径形态时，
				// 早于 fork 点的消息是复制历史，不计入账本。
				if strings.Contains(v, "/") || strings.Contains(v, "\\") {
					if ts, ok := header["timestamp"].(string); ok {
						if ms, err := parseEntryMs(ts); err == nil {
							forkTsMs = &ms
						}
					}
				}
			}
			if len(lines) > 1 {
				if rev.Complete {
					linesToProcess = lines[1:]
				} else {
					if len(lines) > 1 {
						linesToProcess = lines[1 : len(lines)-1]
					} else {
						linesToProcess = []string{}
					}
				}
			} else {
				linesToProcess = []string{}
			}
			contentBytes, err := os.ReadFile(file)
			if err != nil {
				recordFailure(err)
				continue
			}
			contentStr := string(contentBytes)
			if rev.Complete {
				newCommittedByte = rev.FileSize
				newCommittedLines = int64(len(lines))
			} else {
				lastNL := strings.LastIndex(contentStr, "\n")
				if lastNL != -1 {
					newCommittedByte = int64(lastNL) + 1
				} else {
					newCommittedByte = 0
				}
				parts := strings.Split(contentStr, "\n")
				newCommittedLines = int64(len(parts) - 1)
			}
			nt, err := tailFingerprintAtGo(file, newCommittedByte)
			if err != nil {
				recordFailure(err)
				continue
			}
			newTail = nt
		}
		seen := make(map[string]*PiRecord)
		identities := make(map[string]PiIdentity)
		tsTextByID := make(map[string]string)
		warnedMissingPricing := make(map[string]bool)
		for lineNo, line := range linesToProcess {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				fileDiagnostics.Skipped++
				fileDiagnostics.warn("%s:%d: 坏 JSON 行: %v", file, lineNo+1, err)
				continue
			}
			if forkTsMs != nil {
				if tsStr, ok := entry["timestamp"].(string); ok && tsStr != "" {
					if ms, err := parseEntryMs(tsStr); err == nil && ms < *forkTsMs {
						continue
					}
				}
			}
			rec := ParsePiUsageRecord(entry, sessionID, sessionTimestamp, rev.ModifiedMs)
			if rec == nil {
				if piEntryHasUsage(entry) {
					fileDiagnostics.Skipped++
					fileDiagnostics.warn("%s:%d: 跳过无效 usage 记录", file, lineNo+1)
				}
				continue
			}
			var usageRaw map[string]interface{}
			if entry["type"] == "message" {
				if msg, ok := entry["message"].(map[string]interface{}); ok {
					if u, ok := msg["usage"].(map[string]interface{}); ok {
						usageRaw = u
					}
				}
			} else {
				if u, ok := entry["usage"].(map[string]interface{}); ok {
					usageRaw = u
				}
			}
			if usageRaw == nil {
				usageRaw = make(map[string]interface{})
			}
			var msg map[string]interface{}
			if m, ok := entry["message"].(map[string]interface{}); ok {
				msg = m
			}
			identity := PiRequestIdentity(entry, string(rec.Kind), usageRaw, msg)
			tsText, _ := entry["timestamp"].(string)
			// 同 requestId 文件内替换规则（与 TS oracle 一致）：有 stop 结论的覆盖无结论的，
			// 结论状态相同才按 output 取大者。
			if existing, ok := seen[identity.RequestID]; ok {
				if shouldReplacePiRecord(existing, rec) {
					seen[identity.RequestID] = rec
					identities[identity.RequestID] = identity
					tsTextByID[identity.RequestID] = tsText
				}
			} else {
				seen[identity.RequestID] = rec
				identities[identity.RequestID] = identity
				tsTextByID[identity.RequestID] = tsText
			}
		}
		tx, err := database.DB.Begin()
		if err != nil {
			return SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}, fmt.Errorf("开始同步 %s 事务失败: %w", file, err)
		}
		fileImported, fileSkipped := 0, 0
		rollback := func(cause error) (SyncResult, error) {
			_ = tx.Rollback()
			fileDiagnostics.Skipped++
			fileDiagnostics.warn("同步 %s 失败: %v", file, cause)
			mergeDiagnostics(&diagnostics, &fileDiagnostics)
			return SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}, fmt.Errorf("同步 %s 失败: %w", file, cause)
		}
		for reqID, rec := range seen {
			identity := identities[reqID]
			var existingStop sql.NullString
			var existingOutput int64
			err := tx.QueryRow(`SELECT stop_reason, output_tokens FROM proxy_request_logs WHERE request_id = ? AND data_source = ?`, reqID, "pi_session").Scan(&existingStop, &existingOutput)
			if err == nil {
				existing := &PiRecord{StopReason: existingStop.String, Output: float64(existingOutput)}
				if !shouldReplacePiRecord(existing, rec) {
					fileSkipped++
					continue
				}
				pricing, configured := pricingByModel[rec.Model]
				if !configured && rec.CostTotal <= 0 && !warnedMissingPricing[rec.Model] {
					fileDiagnostics.warn("%s: model %q 未配置价格，cost=0", file, rec.Model)
					warnedMissingPricing[rec.Model] = true
				}
				cost := costForPiRecord(rec, pricing)
				_, err = tx.Exec(`UPDATE proxy_request_logs SET provider_id = ?, app_type = ?, model = ?, request_model = ?, pricing_model = ?, input_tokens = ?, output_tokens = ?, cache_read_tokens = ?, cache_creation_tokens = ?, input_token_semantics = ?, total_cost_usd = ?, latency_ms = ?, status_code = ?, stop_reason = ?, error_message = ?, session_id = ?, provider_type = ?, is_streaming = ?, cost_multiplier = ?, created_at = ?, data_source = ?, kind = ?, reasoning_tokens = ?, cwd = ?, timestamp_text = ? WHERE request_id = ? AND data_source = ?`,
					rec.Provider, "pi", rec.Model, rec.RequestModel, rec.Model, int64(rec.Input), int64(rec.Output), int64(rec.CacheRead), int64(rec.CacheWrite), 0, formatFloat(cost), 0, rec.StatusCode, rec.StopReason, nullableString(rec.ErrorMessage), rec.SessionID, "pi_session", 1, "1.0", rec.CreatedAt, "pi_session", string(rec.Kind), int64(rec.Reasoning), headerCwd, tsTextByID[reqID], reqID, "pi_session")
				if err != nil {
					return rollback(err)
				}
				if _, err = tx.Exec(`INSERT OR REPLACE INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`, "pi_session", reqID, identity.SemanticID, boolToInt(identity.HasEntryID)); err != nil {
					return rollback(err)
				}
				fileImported++
				continue
			}
			if err != sql.ErrNoRows {
				return rollback(err)
			}

			var exists int
			err = tx.QueryRow(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`, "pi_session", reqID).Scan(&exists)
			if err == nil {
				fileSkipped++
				continue
			}
			if err != sql.ErrNoRows {
				return rollback(err)
			}
			if !identity.HasEntryID {
				err = tx.QueryRow(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND semantic_id = ?`, "pi_session", identity.SemanticID).Scan(&exists)
				if err == nil {
					fileSkipped++
					continue
				}
				if err != sql.ErrNoRows {
					return rollback(err)
				}
			}
			if _, err = tx.Exec(`INSERT INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`, "pi_session", reqID, identity.SemanticID, boolToInt(identity.HasEntryID)); err != nil {
				return rollback(err)
			}
			pricing, configured := pricingByModel[rec.Model]
			if !configured && rec.CostTotal <= 0 && !warnedMissingPricing[rec.Model] {
				fileDiagnostics.warn("%s: model %q 未配置价格，cost=0", file, rec.Model)
				warnedMissingPricing[rec.Model] = true
			}
			cost := costForPiRecord(rec, pricing)
			if _, err = tx.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, stop_reason, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source, kind, reasoning_tokens, cwd, timestamp_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				reqID, rec.Provider, "pi", rec.Model, rec.RequestModel, rec.Model, int64(rec.Input), int64(rec.Output), int64(rec.CacheRead), int64(rec.CacheWrite), 0, formatFloat(cost), 0, rec.StatusCode, rec.StopReason, nullableString(rec.ErrorMessage), rec.SessionID, "pi_session", 1, "1.0", rec.CreatedAt, "pi_session", string(rec.Kind), int64(rec.Reasoning), headerCwd, tsTextByID[reqID]); err != nil {
				return rollback(err)
			}
			fileImported++
		}
		parentStr := ""
		if parentSessionID != nil {
			parentStr = *parentSessionID
		}
		fileBase := filepath.Base(file)
		isTask := 0
		if strings.Contains(file, "/tasks/") || strings.Contains(file, "\\tasks\\") {
			isTask = 1
		}
		displayName := displayNameOfPiFile(fileBase, extractFirstUserTextPiFile(file))
		if _, err = tx.Exec(`INSERT OR REPLACE INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name, is_task, parent_session_id) VALUES (?, ?, ?, ?, ?, ?, ?)`, sessionID, headerTs, headerCwd, fileBase, displayName, isTask, parentStr); err != nil {
			return rollback(err)
		}
		if parentStr == "" {
			if _, err = tx.Exec(`UPDATE pi_sessions SET parent_session_id = NULL WHERE session_id = ? AND parent_session_id = ''`, sessionID); err != nil {
				return rollback(err)
			}
		}
		summary, err := json.Marshal(fileDiagnostics)
		if err != nil {
			return rollback(err)
		}
		if _, err = tx.Exec(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, sync_semantics_version, diagnostics_summary) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, file, rev.ModifiedMs, newCommittedLines, rev.ModifiedMs/1000, newCommittedByte, int64(newTail), PiSyncSemanticsVersion, string(summary)); err != nil {
			return rollback(err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			fileDiagnostics.Skipped++
			fileDiagnostics.warn("提交 %s 失败: %v", file, err)
			mergeDiagnostics(&diagnostics, &fileDiagnostics)
			return SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}, fmt.Errorf("提交 %s 失败: %w", file, err)
		}
		imported += fileImported
		skipped += fileSkipped
		mergeDiagnostics(&diagnostics, &fileDiagnostics)
	}
	result := SyncResult{Imported: imported, Skipped: skipped, Diagnostics: diagnostics}
	if len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}
