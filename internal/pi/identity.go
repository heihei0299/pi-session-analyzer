package pi

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"hash"
	"sort"
)

type PiIdentity struct {
	RequestID  string
	SemanticID string
	HasEntryID bool
}

func writeField(h hash.Hash, data []byte) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(data)))
	h.Write(lenBuf[:])
	h.Write(data)
}

func writeString(h hash.Hash, s string) {
	writeField(h, []byte(s))
}

func hashJson(h hash.Hash, v interface{}) {
	if v == nil {
		writeString(h, "null")
		return
	}
	switch x := v.(type) {
	case bool:
		writeString(h, "bool")
		if x {
			writeString(h, "true")
		} else {
			writeString(h, "false")
		}
	case float64:
		writeString(h, "number")
		// use json to get canonical string
		b, _ := json.Marshal(x)
		writeString(h, string(b))
	case string:
		writeString(h, "string")
		writeString(h, x)
	case []interface{}:
		writeString(h, "array")
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(x)))
		h.Write(lenBuf[:])
		for _, e := range x {
			hashJson(h, e)
		}
	case map[string]interface{}:
		writeString(h, "object")
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(keys)))
		h.Write(lenBuf[:])
		for _, k := range keys {
			writeString(h, k)
			hashJson(h, x[k])
		}
	default:
		// fallback
		b, _ := json.Marshal(v)
		writeString(h, string(b))
	}
}

func HashField(data []byte) []byte {
	h := sha256.New()
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(data)))
	h.Write(lenBuf[:])
	h.Write(data)
	return h.Sum(nil)
}

func PiRequestIdentity(entry map[string]interface{}, kind string, usage map[string]interface{}, message map[string]interface{}) PiIdentity {
	// semantic
	h := sha256.New()
	writeString(h, "pi-session-semantic-v1")
	writeString(h, kind)
	if ts, ok := entry["timestamp"].(string); ok && ts != "" {
		writeString(h, "entry_timestamp")
		writeString(h, ts)
	}
	if message != nil {
		if ts, ok := message["timestamp"]; ok {
			writeString(h, "message_timestamp")
			hashJson(h, ts)
		}
		for _, key := range []string{"provider", "model", "responseModel", "responseId", "api", "toolCallId", "toolName", "stopReason", "errorMessage"} {
			if v, ok := message[key]; ok {
				writeString(h, key)
				hashJson(h, v)
			}
		}
		if c, ok := message["content"]; ok {
			writeString(h, "content")
			hashJson(h, c)
		}
	} else {
		if s, ok := entry["summary"]; ok {
			writeString(h, "summary")
			hashJson(h, s)
		}
	}
	writeString(h, "usage")
	hashJson(h, usage)
	semanticID := "pi_session_semantic:" + string(encodeHex(h.Sum(nil)))

	entryID, _ := entry["id"].(string)
	hasEntryID := entryID != ""
	var requestID string
	if hasEntryID {
		rh := sha256.New()
		writeString(rh, "pi-session-request-v3")
		writeString(rh, kind)
		writeString(rh, entryID)
		if ts, ok := entry["timestamp"]; ok {
			hashJson(rh, ts)
		}
		requestID = "pi_session:" + string(encodeHex(rh.Sum(nil)))
	} else {
		requestID = semanticID
	}
	return PiIdentity{RequestID: requestID, SemanticID: semanticID, HasEntryID: hasEntryID}
}

func encodeHex(b []byte) []byte {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return dst
}
