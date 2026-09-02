package timerange

import (
	"testing"
	"time"
)

func TestParseTimestampStrictValidation(t *testing.T) {
	// 非法日期校验
	invalidDates := []string{"2026-02-30", "2026-13-01", "2026-04-31", "not-a-date"}
	for _, s := range invalidDates {
		_, err := ParseTimestamp(s, false)
		if err == nil {
			t.Errorf("expected error for invalid date %s, but got none", s)
		}
	}

	// 合法日期与 endOfDay 校验
	msStart, err := ParseTimestamp("2026-08-01", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msEnd, err := ParseTimestamp("2026-08-01", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证一天整差值 86400000 - 1 ms
	if msEnd-msStart != 86_400_000-1 {
		t.Errorf("expected 86399999ms difference, got %d", msEnd-msStart)
	}

	tStart := time.UnixMilli(msStart).In(shanghaiLoc)
	if tStart.Hour() != 0 || tStart.Minute() != 0 || tStart.Second() != 0 {
		t.Errorf("expected 00:00:00, got %v", tStart)
	}

	tEnd := time.UnixMilli(msEnd).In(shanghaiLoc)
	if tEnd.Hour() != 23 || tEnd.Minute() != 59 || tEnd.Second() != 59 {
		t.Errorf("expected 23:59:59, got %v", tEnd)
	}
}
