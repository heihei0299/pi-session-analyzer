package serialize

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func SerializeJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func totalsToCSVFields(tot domain.Totals, includeStatus bool) []string {
	fields := []string{
		strconv.Itoa(tot.Requests),
		fmt.Sprintf("%.0f", tot.Input),
		fmt.Sprintf("%.0f", tot.Output),
		fmt.Sprintf("%.0f", tot.CacheRead),
		fmt.Sprintf("%.0f", tot.CacheWrite),
		fmt.Sprintf("%.0f", tot.Reasoning),
		fmt.Sprintf("%.0f", tot.TotalTokens),
		fmt.Sprintf("%.4f", tot.Cost),
		fmt.Sprintf("%.4f", tot.CacheRate),
	}
	if includeStatus {
		fields = append(fields, tot.CostStatus)
	}
	return fields
}

var metricCSVHeaders = []string{
	"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate",
}

func metricHeaders(includeStatus bool) []string {
	headers := append([]string(nil), metricCSVHeaders...)
	if includeStatus {
		headers = append(headers, "costStatus")
	}
	return headers
}

func SerializeCSV(window string, data any) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	switch window {
	case "totals":
		tot, ok := data.(domain.Totals)
		if !ok {
			if totPtr, ok2 := data.(*domain.Totals); ok2 {
				tot = *totPtr
			}
		}
		status := tot.CostStatus != ""
		_ = w.Write(append([]string{"window"}, metricHeaders(status)...))
		_ = w.Write(append([]string{"totals"}, totalsToCSVFields(tot, status)...))

	case "sessions":
		rows, _ := data.([]domain.SessionRow)
		status := anySessionStatus(rows)
		_ = w.Write(append([]string{"sessionId", "timestamp", "cwd", "model"}, metricHeaders(status)...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.SessionId, r.Timestamp, r.Cwd, r.Model}, totalsToCSVFields(r.Totals, status)...))
		}

	case "requests":
		rows, _ := data.([]domain.RequestRow)
		status := anyRequestStatus(rows)
		_ = w.Write(append([]string{"sessionId", "timestamp", "model"}, metricHeaders(status)...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.SessionId, r.Timestamp, r.Model}, totalsToCSVFields(r.Totals, status)...))
		}

	case "groups":
		rows, _ := data.([]domain.GroupRow)
		status := anyGroupStatus(rows)
		_ = w.Write(append([]string{"model", "cwd"}, metricHeaders(status)...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.Model, r.Cwd}, totalsToCSVFields(r.Totals, status)...))
		}

	case "period":
		rows, _ := data.([]domain.PeriodRow)
		status := anyPeriodStatus(rows)
		_ = w.Write(append([]string{"period"}, metricHeaders(status)...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.Period}, totalsToCSVFields(r.Totals, status)...))
		}
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

func anySessionStatus(rows []domain.SessionRow) bool {
	for _, row := range rows {
		if row.CostStatus != "" {
			return true
		}
	}
	return false
}

func anyRequestStatus(rows []domain.RequestRow) bool {
	for _, row := range rows {
		if row.CostStatus != "" {
			return true
		}
	}
	return false
}

func anyGroupStatus(rows []domain.GroupRow) bool {
	for _, row := range rows {
		if row.CostStatus != "" {
			return true
		}
	}
	return false
}

func anyPeriodStatus(rows []domain.PeriodRow) bool {
	for _, row := range rows {
		if row.CostStatus != "" {
			return true
		}
	}
	return false
}
