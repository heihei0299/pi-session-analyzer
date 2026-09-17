package pi

import "testing"

func makeAssistantEntry(overrides map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"type":      "message",
		"id":        "a1",
		"timestamp": "2026-07-31T01:58:29.810Z",
		"message": map[string]interface{}{
			"role":          "assistant",
			"provider":      "fixture-provider",
			"model":         "fixture-model",
			"responseModel": "actual-model",
			"responseId":    "resp-1",
			"stopReason":    "stop",
			"usage": map[string]interface{}{
				"input":       float64(10),
				"output":      float64(5),
				"cacheRead":   float64(3),
				"cacheWrite":  float64(2),
				"totalTokens": float64(20),
				"cost":        map[string]interface{}{"total": float64(0.1)},
			},
			"timestamp": float64(1700000000000),
			"content":   []interface{}{map[string]interface{}{"type": "text", "text": "ok"}},
		},
	}
	for k, v := range overrides {
		base[k] = v
	}
	return base
}

func TestParseAssistant(t *testing.T) {
	rec := ParsePiUsageRecord(makeAssistantEntry(nil), "session-a", int64Ptr(1700000000), 1700000000000)
	if rec == nil {
		t.Fatalf("expected record")
	}
	if rec.Kind != KindAssistant || rec.Input != 10 || rec.Output != 5 || rec.CacheRead != 3 || rec.CacheWrite != 2 {
		t.Fatalf("unexpected rec %+v", rec)
	}
	if rec.Provider != "fixture-provider" || rec.RequestModel != "fixture-model" || rec.Model != "actual-model" {
		t.Fatalf("provider/model mismatch %+v", rec)
	}
}

func TestParseToolResult(t *testing.T) {
	entry := map[string]interface{}{
		"type":      "message",
		"id":        "t1",
		"timestamp": "2026-07-31T01:58:30.810Z",
		"message": map[string]interface{}{
			"role":       "toolResult",
			"toolCallId": "tool-1",
			"toolName":   "nested",
			"content":    []interface{}{},
			"usage":      map[string]interface{}{"input": float64(3), "output": float64(4), "cacheRead": float64(1), "cacheWrite": float64(1), "totalTokens": float64(9), "cost": map[string]interface{}{"total": float64(0)}},
		},
	}
	rec := ParsePiUsageRecord(entry, "session-a", int64Ptr(1700000000), 1700000000000)
	if rec == nil || rec.Kind != KindToolResult || rec.Input != 3 {
		t.Fatalf("toolResult parse failed %+v", rec)
	}
}

func TestParseCompaction(t *testing.T) {
	entry := map[string]interface{}{"type": "compaction", "id": "c1", "timestamp": "2026-07-31T01:58:31.810Z", "summary": "summary", "usage": map[string]interface{}{"input": float64(11), "output": float64(12), "cacheRead": float64(2), "cacheWrite": float64(3), "totalTokens": float64(28), "cost": map[string]interface{}{"total": float64(0)}}}
	rec := ParsePiUsageRecord(entry, "session-a", int64Ptr(1700000000), 1700000000000)
	if rec == nil || rec.Kind != KindCompaction || rec.Input != 11 {
		t.Fatalf("compaction failed %+v", rec)
	}
}

func TestParseBranchSummary(t *testing.T) {
	entry := map[string]interface{}{"type": "branch_summary", "id": "b1", "timestamp": "2026-07-31T01:58:32.810Z", "summary": "branch summary", "usage": map[string]interface{}{"input": float64(13), "output": float64(14), "cacheRead": float64(4), "cacheWrite": float64(5), "totalTokens": float64(36), "cost": map[string]interface{}{"total": float64(0)}}}
	rec := ParsePiUsageRecord(entry, "session-a", int64Ptr(1700000000), 1700000000000)
	if rec == nil || rec.Kind != KindBranchSummary {
		t.Fatalf("branch_summary failed %+v", rec)
	}
}

func TestGateAllZero(t *testing.T) {
	entry := map[string]interface{}{
		"type": "message", "id": "a1", "timestamp": "2026-07-31T01:58:29.810Z",
		"message": map[string]interface{}{"role": "assistant", "provider": "p", "model": "m", "usage": map[string]interface{}{"input": float64(0), "output": float64(0), "cacheRead": float64(0), "cacheWrite": float64(0), "totalTokens": float64(0), "cost": map[string]interface{}{"total": float64(0)}}, "stopReason": "stop"},
	}
	if rec := ParsePiUsageRecord(entry, "session-a", int64Ptr(1700000000), 1700000000000); rec != nil {
		t.Fatalf("should be nil for all zero")
	}
}

func TestGateFailed(t *testing.T) {
	entry := map[string]interface{}{
		"type": "message", "id": "a1", "timestamp": "2026-07-31T01:58:29.810Z",
		"message": map[string]interface{}{"role": "assistant", "provider": "p", "model": "m", "usage": map[string]interface{}{"input": float64(0), "output": float64(0), "cacheRead": float64(0), "cacheWrite": float64(0), "totalTokens": float64(0), "cost": map[string]interface{}{"total": float64(0)}}, "stopReason": "error"},
	}
	rec := ParsePiUsageRecord(entry, "session-a", int64Ptr(1700000000), 1700000000000)
	if rec == nil || rec.StatusCode != 500 {
		t.Fatalf("failed should be kept %+v", rec)
	}
}

func int64Ptr(v int64) *int64 { return &v }
