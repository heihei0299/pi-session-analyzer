package pi

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
)

type PiFileRevision struct {
	FileSize        int64
	TailFingerprint uint32
	Complete        bool
	ModifiedMs      int64
}

func tailFingerprintGo(buf []byte) uint32 {
	h := sha256.New()
	label := []byte("pi-session-tail-v1")
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(label)))
	h.Write(lenBuf[:])
	h.Write(label)
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(buf)))
	h.Write(lenBuf[:])
	h.Write(buf)
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

func PiFileRevisionOf(path string) (PiFileRevision, error) {
	st, err := os.Stat(path)
	if err != nil {
		return PiFileRevision{}, err
	}
	fileSize := st.Size()
	modifiedMs := st.ModTime().UnixMilli()
	tailLen := fileSize
	if tailLen > 4096 {
		tailLen = 4096
	}
	var tail []byte
	complete := true
	if tailLen > 0 {
		f, err := os.Open(path)
		if err != nil {
			return PiFileRevision{}, err
		}
		defer f.Close()
		buf := make([]byte, tailLen)
		_, err = f.ReadAt(buf, fileSize-tailLen)
		if err != nil {
			return PiFileRevision{}, err
		}
		tail = buf
		complete = buf[len(buf)-1] == '\n'
	}
	return PiFileRevision{
		FileSize:        fileSize,
		TailFingerprint: tailFingerprintGo(tail),
		Complete:        complete,
		ModifiedMs:      modifiedMs,
	}, nil
}

func tailFingerprintAtGo(path string, offset int64) (uint32, error) {
	l := offset
	if l > 4096 {
		l = 4096
	}
	if l <= 0 {
		return tailFingerprintGo([]byte{}), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, l)
	_, err = f.ReadAt(buf, offset-l)
	if err != nil {
		return 0, err
	}
	return tailFingerprintGo(buf), nil
}

type SyncResult struct {
	Imported int
	Skipped  int
}

func SyncPiUsage(database *db.Database, files []string) (SyncResult, error) {
	var imported, skipped int
	for _, file := range files {
		rev, err := PiFileRevisionOf(file)
		if err != nil {
			continue
		}
		var cursorLastByte int64
		var cursorLastLine int64
		var cursorTail uint32
		var hasCursor bool
		var canSeek bool
		var nByte, nTail, nLine sql.NullInt64
		err = database.DB.QueryRow(`SELECT last_byte_offset, last_tail_fingerprint, last_line_offset FROM session_log_sync WHERE file_path = ?`, file).Scan(&nByte, &nTail, &nLine)
		if err == nil && nByte.Valid && nTail.Valid {
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
			continue
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
				continue
			}
			br := bufio.NewReader(fh)
			hl, err := br.ReadString('\n')
			fh.Close()
			if err != nil && len(hl) == 0 {
				continue
			}
			headerLine := strings.TrimSpace(hl)
			if headerLine == "" {
				continue
			}
			var header map[string]interface{}
			if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
				continue
			}
			if header["type"] != "session" {
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
				continue
			}
			buf := make([]byte, suffixLen)
			_, err = f2.ReadAt(buf, cursorLastByte)
			f2.Close()
			if err != nil {
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
			nt, _ := tailFingerprintAtGo(file, newCommittedByte)
			newTail = nt
		} else {
			f, err := os.Open(file)
			if err != nil {
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
			if len(lines) == 0 {
				continue
			}
			var header map[string]interface{}
			if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
				continue
			}
			if header["type"] != "session" {
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
			contentBytes, _ := os.ReadFile(file)
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
			nt, _ := tailFingerprintAtGo(file, newCommittedByte)
			newTail = nt
		}
		seen := make(map[string]*PiRecord)
		identities := make(map[string]PiIdentity)
		tsTextByID := make(map[string]string)
		for _, line := range linesToProcess {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
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
		pricingByModel := loadModelPricing(database)
		_, _ = database.DB.Exec(`BEGIN`)
		for reqID, rec := range seen {
			identity := identities[reqID]
			var exists int
			err := database.DB.QueryRow(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`, "pi_session", reqID).Scan(&exists)
			if err == nil {
				skipped++
				continue
			}
			if !identity.HasEntryID {
				err = database.DB.QueryRow(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND semantic_id = ?`, "pi_session", identity.SemanticID).Scan(&exists)
				if err == nil {
					skipped++
					continue
				}
			}
			_, _ = database.DB.Exec(`INSERT OR IGNORE INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`, "pi_session", reqID, identity.SemanticID, boolToInt(identity.HasEntryID))
			cost := costForPiRecord(rec, pricingByModel[rec.Model])
			_, _ = database.DB.Exec(`INSERT OR IGNORE INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source, kind, reasoning_tokens, cwd, timestamp_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				reqID, rec.Provider, "pi", rec.Model, rec.RequestModel, rec.Model, int64(rec.Input), int64(rec.Output), int64(rec.CacheRead), int64(rec.CacheWrite), 0, formatFloat(cost), 0, rec.StatusCode, nullableString(rec.ErrorMessage), rec.SessionID, "pi_session", 1, "1.0", rec.CreatedAt, "pi_session", string(rec.Kind), int64(rec.Reasoning), headerCwd, tsTextByID[reqID])
			imported++
		}
		parentStr := ""
		if parentSessionID != nil {
			parentStr = *parentSessionID
		}
		fileBase := file
		if idx := strings.LastIndex(file, "/"); idx != -1 {
			fileBase = file[idx+1:]
		}
		isTask := 0
		if strings.Contains(file, "/tasks/") || strings.Contains(file, "\\tasks\\") {
			isTask = 1
		}
		displayName := displayNameOfPiFile(fileBase, extractFirstUserTextPiFile(file))
		_, _ = database.DB.Exec(`INSERT OR REPLACE INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name, is_task, parent_session_id) VALUES (?, ?, ?, ?, ?, ?, ?)`, sessionID, headerTs, headerCwd, fileBase, displayName, isTask, parentStr)
		if parentStr == "" {
			_, _ = database.DB.Exec(`UPDATE pi_sessions SET parent_session_id = NULL WHERE session_id = ? AND parent_session_id = ''`, sessionID)
		}
		_, _ = database.DB.Exec(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`, file, rev.ModifiedMs, newCommittedLines, rev.ModifiedMs/1000, newCommittedByte, int64(newTail))
		_, _ = database.DB.Exec(`COMMIT`)
	}
	return SyncResult{Imported: imported, Skipped: skipped}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(parseFloatStr(f), "0"), ".")
}

func parseFloatStr(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func parseTimestampGo(s string) (int64, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Unix(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix(), nil
	}
	return 0, nil
}

func timeParse(s string) (int64, error) {
	return parseTimestampGo(s)
}
