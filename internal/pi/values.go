package pi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	return 0, fmt.Errorf("invalid timestamp: %s", s)
}
