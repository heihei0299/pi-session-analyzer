package piaudit

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

func MonthBounds(year, month int, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0).Add(-time.Nanosecond)
}

// TotalsForMonth reads only the Pi usage needed by the standalone audit view.
// It deliberately duplicates the small file boundary instead of importing token-analyzer internals.
func TotalsForMonth(dir string, year, month int) (opencode.LocalTotals, error) {
	start, end := MonthBounds(year, month, time.Local)
	return TotalsForRange(dir, start, end)
}

func TotalsForRange(dir string, start, end time.Time) (opencode.LocalTotals, error) {
	totals := opencode.LocalTotals{}
	if dir == "" {
		return totals, nil
	}
	files, err := collectSessionFiles(dir)
	if err != nil {
		return opencode.LocalTotals{}, err
	}
	for _, path := range files {
		fileTotals, err := totalsFromFile(path, start, end)
		if err != nil {
			return opencode.LocalTotals{}, err
		}
		totals.Requests += fileTotals.Requests
		totals.Input += fileTotals.Input
		totals.Output += fileTotals.Output
		totals.CacheRead += fileTotals.CacheRead
		totals.CacheWrite += fileTotals.CacheWrite
		totals.Reasoning += fileTotals.Reasoning
		totals.Cost += fileTotals.Cost
	}
	totals.TotalTokens = totals.Input + totals.CacheRead + totals.Output
	return totals, err
}

func RecordsInMonth(records []opencode.UsageRecord, year, month int) []opencode.UsageRecord {
	start, end := MonthBounds(year, month, time.Local)
	out := make([]opencode.UsageRecord, 0, len(records))
	for _, record := range records {
		timestamp, ok := ParseTimestamp(record.TimeCreated)
		if !ok || timestamp.Before(start) || timestamp.After(end) {
			continue
		}
		out = append(out, record)
	}
	return out
}

func totalsFromFile(path string, start, end time.Time) (opencode.LocalTotals, error) {
	file, err := os.Open(path)
	if err != nil {
		return opencode.LocalTotals{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	firstLine := true
	var forkAt time.Time
	forkEnabled := false
	var totals opencode.LocalTotals
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			if firstLine {
				return opencode.LocalTotals{}, nil
			}
			continue
		}
		if firstLine {
			firstLine = false
			if stringValue(entry["type"]) != "session" {
				return opencode.LocalTotals{}, nil
			}
			parent := stringValue(entry["parentSession"])
			if strings.Contains(parent, "/") || strings.Contains(parent, "\\") {
				if timestamp, ok := ParseTimestamp(stringValue(entry["timestamp"])); ok {
					forkAt = timestamp
					forkEnabled = true
				}
			}
			continue
		}
		if stringValue(entry["type"]) != "message" {
			continue
		}
		message, ok := entry["message"].(map[string]any)
		if !ok || stringValue(message["role"]) != "assistant" {
			continue
		}
		if forkEnabled {
			if timestamp, ok := ParseTimestamp(stringValue(entry["timestamp"])); ok && timestamp.Before(forkAt) {
				continue
			}
		}
		usage, ok := message["usage"].(map[string]any)
		if !ok {
			continue
		}
		if timestamp, ok := ParseTimestamp(stringValue(entry["timestamp"])); ok && (timestamp.Before(start) || timestamp.After(end)) {
			continue
		}
		totals.Requests++
		totals.Input += number(usage["input"])
		totals.Output += number(usage["output"])
		totals.CacheRead += number(usage["cacheRead"])
		totals.CacheWrite += number(usage["cacheWrite"])
		totals.Reasoning += number(usage["reasoning"])
		if cost, ok := usage["cost"].(map[string]any); ok {
			totals.Cost += number(cost["total"])
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return opencode.LocalTotals{}, err
	}
	totals.TotalTokens = totals.Input + totals.CacheRead + totals.Output
	return totals, nil
}

func collectSessionFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			files = append(files, filepath.Join(root, entry.Name()))
		}
	}
	// Flat roots are authoritative. Otherwise use the native projectDirectories depth.
	if len(files) == 0 {
		for _, project := range entries {
			if !project.IsDir() {
				continue
			}
			projectDir := filepath.Join(root, project.Name())
			projectFiles, readErr := os.ReadDir(projectDir)
			if readErr != nil {
				continue
			}
			for _, entry := range projectFiles {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
					files = append(files, filepath.Join(projectDir, entry.Name()))
				}
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func ParseTimestamp(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, true
	}
	parsed, err = time.Parse("2006-01-02T15:04:05.999999999", value)
	return parsed, err == nil
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func number(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	case int:
		return float64(value)
	default:
		return 0
	}
}
