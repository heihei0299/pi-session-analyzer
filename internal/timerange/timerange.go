package timerange

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var (
	dateOnlyRegex = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	tzSuffixRegex = regexp.MustCompile(`(?:Z|[+-]\d{2}:?\d{2})$`)
	shanghaiLoc   *time.Location
)

func init() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	shanghaiLoc = loc
}

// ParseUtcTimestamp 解析 UTC 时间戳字符串（用于会话与消息内的 ISO 时间）
func ParseUtcTimestamp(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	target := s
	if !tzSuffixRegex.MatchString(target) {
		target = target + "Z"
	}
	t, err := time.Parse(time.RFC3339Nano, target)
	if err != nil {
		t, err = time.Parse(time.RFC3339, target)
		if err != nil {
			return 0, fmt.Errorf("invalid ISO timestamp: %s", s)
		}
	}
	return t.UnixMilli(), nil
}

// ParseTimestamp 解析时间筛选参数（支持 YYYY-MM-DD 或 ISO 完整字符串，纯日期按本地时区解释）
func ParseTimestamp(s string, endOfDay bool) (int64, error) {
	if m := dateOnlyRegex.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])

		if mo < 1 || mo > 12 {
			return 0, fmt.Errorf("无效时间: %s（支持 ISO 日期或时间戳）", s)
		}
		// 严格日历校验
		tCheck := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, shanghaiLoc)
		if tCheck.Year() != y || int(tCheck.Month()) != mo || tCheck.Day() != d {
			return 0, fmt.Errorf("无效时间: %s（支持 ISO 日期或时间戳）", s)
		}

		if endOfDay {
			tEnd := time.Date(y, time.Month(mo), d, 23, 59, 59, 999*1e6, shanghaiLoc)
			return tEnd.UnixMilli(), nil
		}
		return tCheck.UnixMilli(), nil
	}

	// 完整时间戳字符串
	target := s
	t, err := time.Parse(time.RFC3339Nano, target)
	if err != nil {
		t, err = time.Parse(time.RFC3339, target)
		if err != nil {
			return 0, fmt.Errorf("无效时间: %s（支持 ISO 日期或时间戳）", s)
		}
	}
	return t.UnixMilli(), nil
}

type RangeKind string

const (
	KindSession RangeKind = "session"
	KindMessage RangeKind = "message"
)

type TimeRange struct {
	Kind    RangeKind `json:"kind"`
	Since   string    `json:"since,omitempty"`
	Until   string    `json:"until,omitempty"`
	SinceMs *int64    `json:"-"`
	UntilMs *int64    `json:"-"`
}

func MakeSessionRange(since, until string) (*TimeRange, error) {
	return makeRange(KindSession, since, until)
}

func MakeMessageRange(since, until string) (*TimeRange, error) {
	return makeRange(KindMessage, since, until)
}

func makeRange(kind RangeKind, since, until string) (*TimeRange, error) {
	if since == "" && until == "" {
		return nil, nil
	}
	var sinceMs, untilMs *int64
	if since != "" {
		ms, err := ParseTimestamp(since, false)
		if err != nil {
			return nil, err
		}
		sinceMs = &ms
	}
	if until != "" {
		ms, err := ParseTimestamp(until, true)
		if err != nil {
			return nil, err
		}
		untilMs = &ms
	}
	return &TimeRange{
		Kind:    kind,
		Since:   since,
		Until:   until,
		SinceMs: sinceMs,
		UntilMs: untilMs,
	}, nil
}
