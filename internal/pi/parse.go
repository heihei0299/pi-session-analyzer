package pi

import (
	"encoding/json"
	"strings"
	"time"
)

type PiKind string

const (
	KindAssistant     PiKind = "assistant"
	KindToolResult    PiKind = "tool_result"
	KindCompaction    PiKind = "compaction"
	KindBranchSummary PiKind = "branch_summary"
)

type PiRecord struct {
	Kind         PiKind
	Input        float64
	Output       float64
	CacheRead    float64
	CacheWrite   float64
	Provider     string
	RequestModel string
	Model        string
	StatusCode   int
	ErrorMessage string
	CreatedAt    int64
	SessionID    string
	CostTotal    float64
}

func toFinite(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		if x != x || x > 1e308 || x < -1e308 { // NaN or Inf
			return 0
		}
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

func parseTimestamp(v interface{}) *int64 {
	switch x := v.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			ts := t.Unix()
			return &ts
		}
		// try without timezone?
		if t, err := time.Parse("2006-01-02T15:04:05.999Z", x); err == nil {
			ts := t.Unix()
			return &ts
		}
	case float64:
		ts := int64(x)
		if ts > 1e12 {
			ts /= 1000
		} else if ts > 1e9 {
			ts /= 1000
		}
		return &ts
	case int64:
		ts := x
		if ts > 1e12 {
			ts /= 1000
		}
		return &ts
	}
	return nil
}

func truncateLabel(s string) string {
	if len(s) <= 512 {
		return s
	}
	// 保 UTF-8 边界
	for len(s) > 512 {
		s = s[:len(s)-1]
		for len(s) > 0 && (s[len(s)-1]&0xC0) == 0x80 {
			s = s[:len(s)-1]
		}
		if len(s) <= 512 {
			break
		}
		s = s[:512]
	}
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}

// ParsePiUsageRecord 解析四载体，门控 has_billable||has_cost||failed
func ParsePiUsageRecord(entry map[string]interface{}, sessionID string, sessionTimestamp *int64, fileMtime int64) *PiRecord {
	typ, _ := entry["type"].(string)
	var kind PiKind
	var usageRaw map[string]interface{}
	var message map[string]interface{}
	var stopReason string

	switch typ {
	case "message":
		msg, ok := entry["message"].(map[string]interface{})
		if !ok {
			return nil
		}
		message = msg
		role, _ := msg["role"].(string)
		switch role {
		case "assistant":
			kind = KindAssistant
			if u, ok := msg["usage"].(map[string]interface{}); ok {
				usageRaw = u
			}
			stopReason, _ = msg["stopReason"].(string)
		case "toolResult":
			kind = KindToolResult
			if u, ok := msg["usage"].(map[string]interface{}); ok {
				usageRaw = u
			}
		default:
			return nil
		}
	case "compaction":
		kind = KindCompaction
		if u, ok := entry["usage"].(map[string]interface{}); ok {
			usageRaw = u
		}
	case "branch_summary":
		kind = KindBranchSummary
		if u, ok := entry["usage"].(map[string]interface{}); ok {
			usageRaw = u
		}
	default:
		return nil
	}
	if usageRaw == nil {
		return nil
	}
	input := toFinite(usageRaw["input"])
	output := toFinite(usageRaw["output"])
	cacheRead := toFinite(usageRaw["cacheRead"])
	cacheWrite := toFinite(usageRaw["cacheWrite"])
	var costTotal float64
	if c, ok := usageRaw["cost"].(map[string]interface{}); ok {
		costTotal = toFinite(c["total"])
	}
	hasBillable := input > 0 || output > 0 || cacheRead > 0 || cacheWrite > 0
	hasCost := costTotal > 0
	failed := stopReason == "error" || stopReason == "aborted"
	if !hasBillable && !hasCost && !failed {
		return nil
	}
	var provider, requestModel, model string
	if kind == KindAssistant {
		if p, ok := message["provider"].(string); ok && p != "" {
			provider = truncateLabel(p)
		} else {
			provider = "_pi_session"
		}
		if rm, ok := message["model"].(string); ok && rm != "" {
			requestModel = truncateLabel(rm)
		} else {
			requestModel = "unknown"
		}
		if rm, ok := message["responseModel"].(string); ok && rm != "" {
			model = truncateLabel(rm)
		} else {
			model = requestModel
		}
	} else {
		provider = "_pi_session"
		requestModel = "unknown"
		model = "unknown"
	}
	var createdAt int64
	if ts := parseTimestamp(entry["timestamp"]); ts != nil {
		createdAt = *ts
	} else if message != nil {
		if ts := parseTimestamp(message["timestamp"]); ts != nil {
			createdAt = *ts
		} else if sessionTimestamp != nil {
			createdAt = *sessionTimestamp
		} else {
			createdAt = fileMtime / 1000
		}
	} else if sessionTimestamp != nil {
		createdAt = *sessionTimestamp
	} else {
		createdAt = fileMtime / 1000
	}
	// 夹逼
	if createdAt < -62167219200 {
		createdAt = -62167219200
	}
	if createdAt > 253402300799 {
		createdAt = 253402300799
	}
	statusCode := 200
	errorMessage := ""
	if failed {
		if stopReason == "aborted" {
			statusCode = 499
			errorMessage = "Pi request aborted"
		} else {
			statusCode = 500
			if em, ok := message["errorMessage"].(string); ok && strings.TrimSpace(em) != "" {
				errorMessage = em
			} else {
				errorMessage = "Pi request failed"
			}
		}
	}
	return &PiRecord{
		Kind:         kind,
		Input:        input,
		Output:       output,
		CacheRead:    cacheRead,
		CacheWrite:   cacheWrite,
		Provider:     provider,
		RequestModel: requestModel,
		Model:        model,
		StatusCode:   statusCode,
		ErrorMessage: errorMessage,
		CreatedAt:    createdAt,
		SessionID:    sessionID,
		CostTotal:    costTotal,
	}
}
