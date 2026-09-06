package pi

import "testing"

func makeIdentityEntry(id *string, usage map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"type":      "message",
		"timestamp": "2026-07-31T01:58:29.810Z",
		"message": map[string]interface{}{
			"role":       "assistant",
			"provider":   "p",
			"model":      "m",
			"usage":      usage,
			"stopReason": "stop",
			"content":    []interface{}{map[string]interface{}{"type": "text", "text": "ok"}},
		},
	}
	if id != nil {
		base["id"] = *id
	}
	return base
}

func TestSameIdSamePayload(t *testing.T) {
	usage := map[string]interface{}{"input": float64(10), "output": float64(5), "cacheRead": float64(3), "cacheWrite": float64(2), "totalTokens": float64(20), "cost": map[string]interface{}{"total": float64(0.1)}}
	id := "a1"
	e1 := makeIdentityEntry(&id, usage)
	e2 := makeIdentityEntry(&id, usage)
	id1 := PiRequestIdentity(e1, "assistant", usage, e1["message"].(map[string]interface{}))
	id2 := PiRequestIdentity(e2, "assistant", usage, e2["message"].(map[string]interface{}))
	if id1.RequestID != id2.RequestID || id1.SemanticID != id2.SemanticID || !id1.HasEntryID {
		t.Fatalf("same id same payload should be equal %+v vs %+v", id1, id2)
	}
}

func TestNoId(t *testing.T) {
	usage := map[string]interface{}{"input": float64(10), "output": float64(5), "cacheRead": float64(3), "cacheWrite": float64(2), "totalTokens": float64(20), "cost": map[string]interface{}{"total": float64(0.1)}}
	e := makeIdentityEntry(nil, usage)
	id := PiRequestIdentity(e, "assistant", usage, e["message"].(map[string]interface{}))
	if id.RequestID != id.SemanticID || id.HasEntryID {
		t.Fatalf("no id should have request==semantic and hasEntry false %+v", id)
	}
}

func TestSamePayloadDifferentId(t *testing.T) {
	usage := map[string]interface{}{"input": float64(10), "output": float64(5), "cacheRead": float64(3), "cacheWrite": float64(2), "totalTokens": float64(20), "cost": map[string]interface{}{"total": float64(0.1)}}
	id1 := "a1"
	id2 := "a2"
	e1 := makeIdentityEntry(&id1, usage)
	e2 := makeIdentityEntry(&id2, usage)
	pi1 := PiRequestIdentity(e1, "assistant", usage, e1["message"].(map[string]interface{}))
	pi2 := PiRequestIdentity(e2, "assistant", usage, e2["message"].(map[string]interface{}))
	if pi1.RequestID == pi2.RequestID {
		t.Fatalf("different id should have different requestID")
	}
	if pi1.SemanticID != pi2.SemanticID {
		t.Fatalf("same payload different id should have same semantic")
	}
}

func TestHashField(t *testing.T) {
	h1 := HashField([]byte("hello"))
	h2 := HashField([]byte("hello"))
	if string(h1) != string(h2) {
		t.Fatalf("hashField should be deterministic")
	}
}

func TestDifferentKind(t *testing.T) {
	usage := map[string]interface{}{"input": float64(10), "output": float64(5), "cacheRead": float64(3), "cacheWrite": float64(2), "totalTokens": float64(20), "cost": map[string]interface{}{"total": float64(0.1)}}
	id := "a1"
	e := makeIdentityEntry(&id, usage)
	idA := PiRequestIdentity(e, "assistant", usage, e["message"].(map[string]interface{}))
	idB := PiRequestIdentity(e, "tool_result", usage, e["message"].(map[string]interface{}))
	if idA.SemanticID == idB.SemanticID {
		t.Fatalf("different kind should have different semantic")
	}
}
