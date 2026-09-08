package codex

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/klauspost/compress/zstd"
)

type SessionMeta struct {
	SessionID                   string `json:"sessionId,omitempty"`
	ThreadID                    string `json:"threadId,omitempty"`
	Cwd                         string `json:"cwd,omitempty"`
	Originator                  string `json:"originator,omitempty"`
	CliVersion                  string `json:"cliVersion,omitempty"`
	ModelProvider               string `json:"modelProvider,omitempty"`
	Model                       string `json:"model,omitempty"`
	ParentThreadID              string `json:"parentThreadId,omitempty"`
	ForkedFromID                string `json:"forkedFromId,omitempty"`
	ForkedFromOrdinal           *int64 `json:"forkedFromOrdinal,omitempty"`
	SubagentHistoryStartOrdinal *int64 `json:"subagentHistoryStartOrdinal,omitempty"`
	HistoryBase                 string `json:"historyBase,omitempty"`
	ThreadSource                string `json:"threadSource,omitempty"`
	AgentRole                   string `json:"agentRole,omitempty"`
	AgentPath                   string `json:"agentPath,omitempty"`
	AgentNickname               string `json:"agentNickname,omitempty"`
	Timestamp                   string `json:"timestamp,omitempty"`
}

type UsageEvent struct {
	ResponseID string       `json:"responseId"`
	TurnID     string       `json:"turnId,omitempty"`
	ThreadID   string       `json:"threadId,omitempty"`
	SessionID  string       `json:"sessionId,omitempty"`
	Timestamp  string       `json:"timestamp,omitempty"`
	Model      string       `json:"model"`
	Provider   string       `json:"provider"`
	Usage      domain.Usage `json:"usage"`
	CostStatus string       `json:"costStatus"`
}

type ParsedRollout struct {
	File  RolloutFile
	Meta  SessionMeta
	Usage []UsageEvent
}

func ParseRollout(file RolloutFile) (ParsedRollout, Diagnostics, error) {
	reader, closeReader, err := openRollout(file.Path)
	if err != nil {
		return ParsedRollout{File: file}, Diagnostics{}, err
	}
	defer closeReader()

	parsed := ParsedRollout{File: file}
	var diagnostics Diagnostics
	turnModels := make(map[string]string)
	turnProviders := make(map[string]string)
	lines, incomplete, readErr := readCompleteLines(reader)
	if incomplete {
		diagnostics.Skipped++
		diagnostics.warn("%s: 忽略末尾半行", file.Path)
	}
	for line, lineBytes := range lines {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(lineBytes, &envelope); err != nil {
			diagnostics.Skipped++
			diagnostics.warn("%s:%d: 坏 JSON 行: %v", file.Path, line+1, err)
			continue
		}
		timestamp := stringValue(envelope["timestamp"])
		if strings.TrimSpace(timestamp) == "" {
			diagnostics.Skipped++
			diagnostics.warn("%s:%d: 缺少 rollout timestamp", file.Path, line+1)
			continue
		}
		typeName := stringValue(envelope["type"])
		payload := objectValue(envelope["payload"])
		if payload == nil {
			payload = envelope
		}

		switch typeName {
		case "session_meta":
			parsed.Meta = parseSessionMeta(payload, timestamp)
		case "turn_context":
			turnID := firstString(payload, "turn_id", "turnId", "id")
			model := firstString(payload, "model", "model_id", "modelId")
			provider := firstString(payload, "model_provider", "modelProvider", "provider")
			if turnID != "" && model != "" {
				turnModels[turnID] = model
			}
			if turnID != "" && provider != "" {
				turnProviders[turnID] = provider
			}
		case "token_usage_record":
			event, ok := parseUsageEvent(payload, timestamp)
			if !ok {
				diagnostics.Skipped++
				diagnostics.warn("%s:%d: token_usage_record 缺少可靠 usage 或 response_id", file.Path, line+1)
				continue
			}
			parsed.Usage = append(parsed.Usage, event)
		case "model_reroute":
			diagnostics.warn("%s:%d: Codex model reroute 仅作诊断，不改写 response model", file.Path, line+1)
		case "response_item", "event_msg", "compacted", "inter_agent_communication", "retained_context", "turn_started", "turn_completed", "history_snapshot":
			// Known non-ledger event. It can still carry diagnostics context, but never usage.
		default:
			diagnostics.Skipped++
			diagnostics.warn("%s:%d: 未知 Codex event type %q", file.Path, line+1, typeName)
		}
	}
	if readErr != nil {
		return parsed, diagnostics, readErr
	}
	if parsed.Meta.ThreadID == "" {
		parsed.Meta.ThreadID = parsed.Meta.SessionID
	}
	if parsed.Meta.SessionID == "" {
		parsed.Meta.SessionID = parsed.Meta.ThreadID
	}
	for i := range parsed.Usage {
		event := &parsed.Usage[i]
		if event.TurnID != "" {
			if event.Model == "" {
				event.Model = turnModels[event.TurnID]
			}
			if event.Provider == "" {
				event.Provider = turnProviders[event.TurnID]
			}
		}
		if event.Model == "" {
			event.Model = parsed.Meta.Model
		}
		if event.Model == "" {
			event.Model = "unknown"
		}
		if event.Provider == "" {
			event.Provider = parsed.Meta.ModelProvider
		}
		if event.Provider == "" {
			event.Provider = "unknown"
		}
		if !validTimestamp(event.Timestamp) {
			event.Timestamp = parsed.Meta.Timestamp
		}
		if event.ThreadID == "" {
			event.ThreadID = parsed.Meta.ThreadID
		}
		if event.SessionID == "" {
			event.SessionID = parsed.Meta.SessionID
		}
	}
	return parsed, diagnostics, nil
}

func readCompleteLines(reader io.Reader) ([][]byte, bool, error) {
	br := bufio.NewReaderSize(reader, 64*1024)
	var lines [][]byte
	incomplete := false
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] != '\n' {
			incomplete = true
		}
		if len(line) > 0 && line[len(line)-1] == '\n' {
			lines = append(lines, line[:len(line)-1])
		}
		if err != nil {
			if err == io.EOF {
				return lines, incomplete, nil
			}
			return lines, incomplete, err
		}
	}
}

func openRollout(path string) (io.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	if strings.HasSuffix(path, ".zst") {
		decoder, err := zstd.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, func() {}, err
		}
		return decoder, func() { decoder.Close(); _ = f.Close() }, nil
	}
	return f, func() { _ = f.Close() }, nil
}

func parseSessionMeta(payload map[string]json.RawMessage, envelopeTimestamp string) SessionMeta {
	meta := SessionMeta{
		SessionID:      firstString(payload, "session_id", "sessionId"),
		ThreadID:       firstString(payload, "id", "thread_id", "threadId"),
		Cwd:            firstString(payload, "cwd"),
		Originator:     firstString(payload, "originator"),
		CliVersion:     firstString(payload, "cli_version", "cliVersion"),
		ModelProvider:  firstString(payload, "model_provider", "modelProvider", "provider"),
		Model:          firstString(payload, "model", "model_id", "modelId"),
		ParentThreadID: firstString(payload, "parent_thread_id", "parentThreadId"),
		ForkedFromID:   firstString(payload, "forked_from_id", "forkedFromId"),
		HistoryBase:    firstString(payload, "history_base", "historyBase"),
		ThreadSource:   firstString(payload, "thread_source", "threadSource", "source"),
		AgentRole:      firstString(payload, "agent_role", "agentRole"),
		AgentPath:      firstString(payload, "agent_path", "agentPath"),
		AgentNickname:  firstString(payload, "agent_nickname", "agentNickname"),
		Timestamp:      envelopeTimestamp,
	}
	if raw, ok := payload["forked_from_ordinal_exclusive"]; ok {
		if n, ok := numberValue(raw); ok {
			v := int64(n)
			meta.ForkedFromOrdinal = &v
		}
	} else if raw, ok := payload["forkedFromOrdinalExclusive"]; ok {
		if n, ok := numberValue(raw); ok {
			v := int64(n)
			meta.ForkedFromOrdinal = &v
		}
	}
	for _, key := range []string{"subagent_history_start_ordinal", "subagentHistoryStartOrdinal"} {
		if raw, ok := payload[key]; ok {
			if n, ok := numberValue(raw); ok {
				v := int64(n)
				meta.SubagentHistoryStartOrdinal = &v
				break
			}
		}
	}
	return meta
}

func parseUsageEvent(payload map[string]json.RawMessage, timestamp string) (UsageEvent, bool) {
	usage := objectValue(payload["usage"])
	if usage == nil {
		if info := objectValue(payload["info"]); info != nil {
			usage = objectValue(info["usage"])
		}
	}
	if usage == nil || !hasUsageValue(usage) {
		return UsageEvent{}, false
	}
	responseID := firstString(payload, "response_id", "responseId")
	if responseID == "" {
		return UsageEvent{}, false
	}
	input := number(usage, "input_tokens", "inputTokens")
	cacheRead := number(usage, "cached_input_tokens", "cachedInputTokens", "cache_read_tokens", "cacheReadTokens")
	cacheWrite := number(usage, "cache_write_input_tokens", "cacheWriteInputTokens", "cache_creation_tokens", "cacheCreationTokens")
	output := number(usage, "output_tokens", "outputTokens")
	reasoning := number(usage, "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens")
	return UsageEvent{
		ResponseID: responseID,
		TurnID:     firstString(payload, "turn_id", "turnId"),
		ThreadID:   firstString(payload, "thread_id", "threadId"),
		SessionID:  firstString(payload, "session_id", "sessionId"),
		Timestamp:  timestamp,
		Model:      firstString(payload, "model", "model_id", "modelId"),
		Provider:   firstString(payload, "provider", "model_provider", "modelProvider"),
		Usage: domain.Usage{
			Input:       input,
			CacheRead:   cacheRead,
			CacheWrite:  cacheWrite,
			Output:      output,
			Reasoning:   reasoning,
			TotalTokens: input + cacheRead + output,
		},
		CostStatus: "unpriced",
	}, true
}

func objectValue(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func stringValue(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return ""
}

func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func hasUsageValue(usage map[string]json.RawMessage) bool {
	for _, key := range []string{"input_tokens", "inputTokens", "cached_input_tokens", "cachedInputTokens", "cache_write_input_tokens", "cacheWriteInputTokens", "output_tokens", "outputTokens", "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens"} {
		if raw, ok := usage[key]; ok {
			if _, valid := numberValue(raw); valid {
				return true
			}
		}
	}
	return false
}

func number(m map[string]json.RawMessage, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := numberValue(m[key]); ok {
			return value
		}
	}
	return 0
}

func numberValue(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value float64
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.ParseFloat(text, 64)
		return value, err == nil
	}
	return 0, false
}

func validTimestamp(value string) bool {
	if value == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
