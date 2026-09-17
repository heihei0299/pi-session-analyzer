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

func TestRejectsReversedRanges(t *testing.T) {
	if _, err := MakeMessageRange("2026-09-12", "2026-09-11"); err == nil {
		t.Fatal("a range whose since is after until must be rejected")
	}
}

func TestParseTimestampDateTimeFormats(t *testing.T) {
	// 自定义时间范围（时分下拉产生无时区 ISO 本地时间串）
	ms, err := ParseTimestamp("2026-08-05T14:00", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedUtc := "2026-08-05T06:00:00.000Z"
	if actual := time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z"); actual != expectedUtc {
		t.Errorf("expected UTC %s, got %s", expectedUtc, actual)
	}

	// 2026-09-12T00:00
	ms00, err := ParseTimestamp("2026-09-12T00:00", false)
	if err != nil {
		t.Fatalf("unexpected error for 2026-09-12T00:00: %v", err)
	}
	tLocal00 := time.UnixMilli(ms00).In(shanghaiLoc)
	if tLocal00.Year() != 2026 || int(tLocal00.Month()) != 9 || tLocal00.Day() != 12 || tLocal00.Hour() != 0 || tLocal00.Minute() != 0 {
		t.Errorf("expected 2026-09-12 00:00, got %v", tLocal00)
	}

	// 2026-09-12T23:59
	ms59, err := ParseTimestamp("2026-09-12T23:59", true)
	if err != nil {
		t.Fatalf("unexpected error for 2026-09-12T23:59: %v", err)
	}
	tLocal59 := time.UnixMilli(ms59).In(shanghaiLoc)
	if tLocal59.Year() != 2026 || int(tLocal59.Month()) != 9 || tLocal59.Day() != 12 || tLocal59.Hour() != 23 || tLocal59.Minute() != 59 {
		t.Errorf("expected 2026-09-12 23:59, got %v", tLocal59)
	}

	// 带时区后缀原样解析
	msZ, err := ParseTimestamp("2026-08-05T14:00Z", false)
	if err != nil {
		t.Fatalf("unexpected error for 2026-08-05T14:00Z: %v", err)
	}
	if actual := time.UnixMilli(msZ).UTC().Format("2006-01-02T15:04:05.000Z"); actual != "2026-08-05T14:00:00.000Z" {
		t.Errorf("expected UTC 2026-08-05T14:00:00.000Z, got %s", actual)
	}
}
