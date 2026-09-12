package pi

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

// parseEntryMs 解析 Pi 条目时间戳为毫秒（与 TS Date.parse 同语义的最小实现）。
func parseEntryMs(s string) (int64, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("invalid timestamp: %s", s)
}

// shouldReplacePiRecord 与 TS oracle 同规则：有 stop 结论的覆盖无结论的，
// 结论状态相同才按 output 取大者。
func shouldReplacePiRecord(existing, rec *PiRecord) bool {
	existingStop := existing.StopReason != ""
	newStop := rec.StopReason != ""
	if newStop && !existingStop {
		return true
	}
	if newStop != existingStop {
		return false
	}
	return rec.Output > existing.Output
}

// nullableString 空字符串存 NULL（与 TS null 语义一致）。
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

type modelPricing struct {
	inputPerM       float64
	outputPerM      float64
	cacheReadPerM   float64
	cacheCreatePerM float64
}

func loadModelPricing(database *db.Database) map[string]modelPricing {
	out := map[string]modelPricing{}
	rows, err := database.DB.Query(`SELECT model_id, input_cost_per_million, output_cost_per_million, cache_read_cost_per_million, cache_creation_cost_per_million FROM model_pricing`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, in, outC, cr, cw string
		if err := rows.Scan(&id, &in, &outC, &cr, &cw); err != nil {
			continue
		}
		out[id] = modelPricing{
			inputPerM:       toNum(in),
			outputPerM:      toNum(outC),
			cacheReadPerM:   toNum(cr),
			cacheCreatePerM: toNum(cw),
		}
	}
	return out
}

func toNum(s string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return n
}

// costForPiRecord 费用回算（直切 TS costForRecord）：上报 cost 优先，否则按 pricing 回算。
func costForPiRecord(rec *PiRecord, pricing modelPricing) float64 {
	if rec.CostTotal > 0 {
		return rec.CostTotal
	}
	if pricing == (modelPricing{}) {
		return 0
	}
	return rec.Input*pricing.inputPerM/1e6 +
		rec.Output*pricing.outputPerM/1e6 +
		rec.CacheRead*pricing.cacheReadPerM/1e6 +
		rec.CacheWrite*pricing.cacheCreatePerM/1e6
}

// extractFirstUserTextPiFile 提取首条 user 文本（供 displayName，与 TS 同逻辑）。
func extractFirstUserTextPiFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue
		}
		if line == "" {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		var typ string
		_ = json.Unmarshal(entry["type"], &typ)
		if typ != "message" {
			continue
		}
		var msg struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(entry["message"], &msg); err != nil {
			continue
		}
		if msg.Role != "user" {
			continue
		}
		for _, part := range msg.Content {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text)
			}
		}
	}
	return ""
}

// displayNameOfPiFile 直接复用 SessionData.DisplayNameOf（与 TS oracle 的
// pi-sync → defaultSessionData.displayNameOf 一致，不另起第二份规则）。
func displayNameOfPiFile(fileName, firstUserText string) string {
	return sessiondata.DefaultSessionData.DisplayNameOf(fileName, firstUserText)
}
