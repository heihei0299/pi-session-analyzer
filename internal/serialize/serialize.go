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

func totalsToCSVFields(tot domain.Totals) []string {
	return []string{
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
}

var metricCSVHeaders = []string{
	"requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate",
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
		_ = w.Write(append([]string{"window"}, metricCSVHeaders...))
		_ = w.Write(append([]string{"totals"}, totalsToCSVFields(tot)...))

	case "sessions":
		rows, _ := data.([]domain.SessionRow)
		_ = w.Write(append([]string{"sessionId", "timestamp", "cwd", "model"}, metricCSVHeaders...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.SessionId, r.Timestamp, r.Cwd, r.Model}, totalsToCSVFields(r.Totals)...))
		}

	case "requests":
		rows, _ := data.([]domain.RequestRow)
		_ = w.Write(append([]string{"sessionId", "timestamp", "model"}, metricCSVHeaders...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.SessionId, r.Timestamp, r.Model}, totalsToCSVFields(r.Totals)...))
		}

	case "groups":
		rows, _ := data.([]domain.GroupRow)
		_ = w.Write(append([]string{"model", "cwd"}, metricCSVHeaders...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.Model, r.Cwd}, totalsToCSVFields(r.Totals)...))
		}

	case "period":
		rows, _ := data.([]domain.PeriodRow)
		_ = w.Write(append([]string{"period"}, metricCSVHeaders...))
		for _, r := range rows {
			_ = w.Write(append([]string{r.Period}, totalsToCSVFields(r.Totals)...))
		}
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}
