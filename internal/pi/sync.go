package pi

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/db"
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
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		// increase buffer for long lines
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
		sessionID, _ := header["id"].(string)
		var sessionTimestamp *int64
		if ts, ok := header["timestamp"].(string); ok {
			if t, err := parseTimestampGo(ts); err == nil {
				sessionTimestamp = &t
			}
		}
		seen := make(map[string]*PiRecord)
		identities := make(map[string]PiIdentity)
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
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
			if existing, ok := seen[identity.RequestID]; ok {
				if rec.Output > existing.Output {
					seen[identity.RequestID] = rec
					identities[identity.RequestID] = identity
				}
			} else {
				seen[identity.RequestID] = rec
				identities[identity.RequestID] = identity
			}
		}
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
			_, _ = database.DB.Exec(`INSERT OR IGNORE INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				reqID, rec.Provider, "pi", rec.Model, rec.RequestModel, rec.Model, int64(rec.Input), int64(rec.Output), int64(rec.CacheRead), int64(rec.CacheWrite), 0, formatFloat(rec.CostTotal), 0, rec.StatusCode, rec.ErrorMessage, rec.SessionID, "pi_session", 1, "1.0", rec.CreatedAt, "pi_session")
			imported++
		}
		_, _ = database.DB.Exec(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`, file, rev.ModifiedMs, int64(len(lines)), rev.ModifiedMs/1000, rev.FileSize, int64(rev.TailFingerprint))
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
	// simple
	if f == 0 {
		return "0"
	}
	// use %g
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
