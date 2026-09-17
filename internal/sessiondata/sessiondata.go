package sessiondata

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

var isoDatePrefixRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}`)

var ErrUnknownSource = errors.New("unknown source")

// NormalizeSource validates a requested source and applies the default used by
// every Refresh and Query entry point.
func NormalizeSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "pi", nil
	}
	switch source {
	case "pi", "codex", "all":
		return source, nil
	default:
		return "", fmt.Errorf("%w: 未知 source: %s（支持 pi|codex|all）", ErrUnknownSource, source)
	}
}

type Filter struct {
	Model     string               `json:"model,omitempty"`
	Source    string               `json:"source,omitempty"`
	Cwd       string               `json:"cwd,omitempty"`
	TimeRange *timerange.TimeRange `json:"timeRange,omitempty"`
}

type ViewKind string

const (
	ViewTotals   ViewKind = "totals"
	ViewSessions ViewKind = "sessions"
	ViewRequests ViewKind = "requests"
	ViewGroups   ViewKind = "groups"
	ViewPeriod   ViewKind = "period"
	ViewMeta     ViewKind = "meta"
)

type View struct {
	Kind    ViewKind       `json:"kind"`
	By      domain.GroupBy `json:"by,omitempty"`
	Period  domain.Period  `json:"period,omitempty"`
	Page    int            `json:"page,omitempty"`
	Size    int            `json:"size,omitempty"`
	SortKey string         `json:"sortKey,omitempty"`
	SortDir string         `json:"sortDir,omitempty"` // "asc" | "desc"
}

type QueryResult struct {
	Window string         `json:"window"`
	Totals *domain.Totals `json:"totals,omitempty"`
	By     domain.GroupBy `json:"by,omitempty"`
	Period domain.Period  `json:"period,omitempty"`
	Rows   any            `json:"rows"`
	Total  int            `json:"total"`
	Page   int            `json:"page,omitempty"`
	Size   int            `json:"size,omitempty"`
	Meta   *QueryMeta     `json:"meta,omitempty"`
}

type QueryMeta struct {
	Dir                string         `json:"dir"`
	SessionCount       int            `json:"sessionCount"`
	DataRange          DataRangeValue `json:"dataRange"`
	Sources            []string       `json:"sources,omitempty"`
	Warnings           []string       `json:"warnings,omitempty"`
	CoverageStatus     string         `json:"coverageStatus,omitempty"`
	UncountedSnapshots int            `json:"uncountedSnapshots,omitempty"`
}

type DataRangeValue struct {
	Since *string `json:"since"`
	Until *string `json:"until"`
}

func NormalizeCwd(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	abs = strings.TrimRight(abs, "/\\")
	if abs == "" {
		abs = "/"
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return strings.TrimRight(eval, "/\\")
	}
	return abs
}

func DisplayNameOf(fileName, firstUserText string) string {
	base := strings.TrimSuffix(fileName, ".jsonl")
	idx := strings.LastIndex(base, "_")
	prefix := fileName
	if idx > 0 && idx < len(base)-1 {
		prefix = base[:idx]
	}
	if isoDatePrefixRegex.MatchString(prefix) {
		if strings.TrimSpace(firstUserText) != "" {
			return strings.TrimSpace(firstUserText)
		}
		return prefix
	}
	return prefix
}

func PeriodKey(timestamp string, p domain.Period) (string, error) {
	ts, err := timerange.ParseUtcTimestamp(timestamp)
	if err != nil {
		return "", err
	}
	t := time.UnixMilli(ts).UTC()
	y := t.Year()
	m := int(t.Month())
	d := t.Day()

	switch p {
	case domain.PeriodDay:
		return fmt.Sprintf("%04d-%02d-%02d", y, m, d), nil
	case domain.PeriodMonth:
		return fmt.Sprintf("%04d-%02d-01", y, m), nil
	case domain.PeriodWeek:
		weekday := int(t.Weekday())
		dow := (weekday + 6) % 7 // Monday = 0, Sunday = 6
		monday := t.AddDate(0, 0, -dow)
		return fmt.Sprintf("%04d-%02d-%02d", monday.Year(), int(monday.Month()), monday.Day()), nil
	}
	return "", fmt.Errorf("unknown period: %s", p)
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareLexical(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareBool(a, b bool) int {
	if !a && b {
		return -1
	}
	if a && !b {
		return 1
	}
	return 0
}

func compareTotalsMetric(a, b domain.Totals, key string) (int, bool) {
	switch key {
	case "requests":
		return compareInt(a.Requests, b.Requests), true
	case "input":
		return compareFloat(a.Input, b.Input), true
	case "output":
		return compareFloat(a.Output, b.Output), true
	case "cache":
		return compareFloat(a.CacheRead+a.CacheWrite, b.CacheRead+b.CacheWrite), true
	case "cacheRead":
		return compareFloat(a.CacheRead, b.CacheRead), true
	case "cacheWrite":
		return compareFloat(a.CacheWrite, b.CacheWrite), true
	case "reasoning":
		return compareFloat(a.Reasoning, b.Reasoning), true
	case "totalTokens":
		return compareFloat(a.TotalTokens, b.TotalTokens), true
	case "cost":
		return compareFloat(a.Cost, b.Cost), true
	case "cacheRate":
		return compareFloat(a.CacheRate, b.CacheRate), true
	default:
		return 0, false
	}
}

func sortBefore(cmp int, desc bool) bool {
	if cmp == 0 {
		return false
	}
	if desc {
		return cmp > 0
	}
	return cmp < 0
}

func SortSessionRows(rows []domain.SessionRow, key string, desc bool) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]
		cmp, ok := compareTotalsMetric(a.Totals, b.Totals, key)
		if !ok || cmp == 0 {
			cmp = compareSessionField(a, b, key)
		}
		if cmp == 0 {
			cmp = compareLexical(a.Timestamp, b.Timestamp)
		}
		if cmp == 0 {
			cmp = compareSessionStable(a, b)
		}
		return sortBefore(cmp, desc)
	})
}

func SortRequestRows(rows []domain.RequestRow, key string, desc bool) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]
		cmp, ok := compareTotalsMetric(a.Totals, b.Totals, key)
		if !ok || cmp == 0 {
			cmp = compareRequestField(a, b, key)
		}
		if cmp == 0 {
			cmp = compareLexical(a.Timestamp, b.Timestamp)
		}
		if cmp == 0 {
			cmp = compareRequestStable(a, b)
		}
		return sortBefore(cmp, desc)
	})
}

func compareSessionField(a, b domain.SessionRow, key string) int {
	switch key {
	case "sessionId":
		return compareLexical(a.SessionId, b.SessionId)
	case "displayName":
		return compareLexical(a.DisplayName, b.DisplayName)
	case "cwd":
		return compareLexical(a.Cwd, b.Cwd)
	case "model":
		return compareLexical(a.Model, b.Model)
	case "timestamp":
		return compareLexical(a.Timestamp, b.Timestamp)
	default:
		return compareLexical(a.Timestamp, b.Timestamp)
	}
}

func compareSessionStable(a, b domain.SessionRow) int {
	for _, pair := range [][2]string{
		{a.SessionId, b.SessionId},
		{a.Source, b.Source},
		{a.FileName, b.FileName},
		{a.DisplayName, b.DisplayName},
		{a.Cwd, b.Cwd},
		{a.CwdNorm, b.CwdNorm},
		{a.Model, b.Model},
		{a.ParentSessionId, b.ParentSessionId},
	} {
		if cmp := compareLexical(pair[0], pair[1]); cmp != 0 {
			return cmp
		}
	}
	if cmp := compareBool(a.IsTask, b.IsTask); cmp != 0 {
		return cmp
	}
	for _, metric := range []string{"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"} {
		if cmp, ok := compareTotalsMetric(a.Totals, b.Totals, metric); ok && cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareRequestField(a, b domain.RequestRow, key string) int {
	switch key {
	case "sessionId":
		return compareLexical(a.SessionId, b.SessionId)
	case "displayName":
		return compareLexical(a.DisplayName, b.DisplayName)
	case "model":
		return compareLexical(a.Model, b.Model)
	case "timestamp":
		return compareLexical(a.Timestamp, b.Timestamp)
	default:
		return compareLexical(a.Timestamp, b.Timestamp)
	}
}

func compareRequestStable(a, b domain.RequestRow) int {
	for _, pair := range [][2]string{
		{a.SessionId, b.SessionId},
		{a.Source, b.Source},
		{a.SourceSessionId, b.SourceSessionId},
		{a.Timestamp, b.Timestamp},
		{a.Model, b.Model},
		{a.DisplayName, b.DisplayName},
	} {
		if cmp := compareLexical(pair[0], pair[1]); cmp != 0 {
			return cmp
		}
	}
	for _, metric := range []string{"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"} {
		if cmp, ok := compareTotalsMetric(a.Totals, b.Totals, metric); ok && cmp != 0 {
			return cmp
		}
	}
	return 0
}

type SessionDetailTotals struct {
	Main          domain.Totals `json:"main"`
	Merged        domain.Totals `json:"merged"`
	ChildrenCount int           `json:"childrenCount"`
}

type SessionDetailMeta struct {
	HasChildren bool `json:"hasChildren"`
}

type SessionDetailResult struct {
	Session  domain.SessionRow   `json:"session"`
	Children []domain.SessionRow `json:"children"`
	Totals   SessionDetailTotals `json:"totals"`
	Requests []domain.RequestRow `json:"requests"`
	Meta     SessionDetailMeta   `json:"meta"`
}

var ErrSessionNotFound = fmt.Errorf("session not found")
