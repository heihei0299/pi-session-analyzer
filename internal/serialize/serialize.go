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
		_ = w.Write([]string{"window", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"})
		_ = w.Write([]string{
			"totals",
			strconv.Itoa(tot.Requests),
			fmt.Sprintf("%.0f", tot.Input),
			fmt.Sprintf("%.0f", tot.Output),
			fmt.Sprintf("%.0f", tot.CacheRead),
			fmt.Sprintf("%.0f", tot.CacheWrite),
			fmt.Sprintf("%.0f", tot.Reasoning),
			fmt.Sprintf("%.0f", tot.TotalTokens),
			fmt.Sprintf("%.4f", tot.Cost),
			fmt.Sprintf("%.4f", tot.CacheRate),
		})
	case "sessions":
		rows, _ := data.([]domain.SessionRow)
		_ = w.Write([]string{"sessionId", "timestamp", "cwd", "model", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"})
		for _, r := range rows {
			_ = w.Write([]string{
				r.SessionId,
				r.Timestamp,
				r.Cwd,
				r.Model,
				strconv.Itoa(r.Requests),
				fmt.Sprintf("%.0f", r.Input),
				fmt.Sprintf("%.0f", r.Output),
				fmt.Sprintf("%.0f", r.CacheRead),
				fmt.Sprintf("%.0f", r.CacheWrite),
				fmt.Sprintf("%.0f", r.Reasoning),
				fmt.Sprintf("%.0f", r.TotalTokens),
				fmt.Sprintf("%.4f", r.Cost),
				fmt.Sprintf("%.4f", r.CacheRate),
			})
		}
	case "requests":
		rows, _ := data.([]domain.RequestRow)
		_ = w.Write([]string{"sessionId", "timestamp", "model", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"})
		for _, r := range rows {
			_ = w.Write([]string{
				r.SessionId,
				r.Timestamp,
				r.Model,
				strconv.Itoa(r.Requests),
				fmt.Sprintf("%.0f", r.Input),
				fmt.Sprintf("%.0f", r.Output),
				fmt.Sprintf("%.0f", r.CacheRead),
				fmt.Sprintf("%.0f", r.CacheWrite),
				fmt.Sprintf("%.0f", r.Reasoning),
				fmt.Sprintf("%.0f", r.TotalTokens),
				fmt.Sprintf("%.4f", r.Cost),
				fmt.Sprintf("%.4f", r.CacheRate),
			})
		}
	case "groups":
		rows, _ := data.([]domain.GroupRow)
		_ = w.Write([]string{"model", "cwd", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"})
		for _, r := range rows {
			_ = w.Write([]string{
				r.Model,
				r.Cwd,
				strconv.Itoa(r.Requests),
				fmt.Sprintf("%.0f", r.Input),
				fmt.Sprintf("%.0f", r.Output),
				fmt.Sprintf("%.0f", r.CacheRead),
				fmt.Sprintf("%.0f", r.CacheWrite),
				fmt.Sprintf("%.0f", r.Reasoning),
				fmt.Sprintf("%.0f", r.TotalTokens),
				fmt.Sprintf("%.4f", r.Cost),
				fmt.Sprintf("%.4f", r.CacheRate),
			})
		}
	case "period":
		rows, _ := data.([]domain.PeriodRow)
		_ = w.Write([]string{"period", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"})
		for _, r := range rows {
			_ = w.Write([]string{
				r.Period,
				strconv.Itoa(r.Requests),
				fmt.Sprintf("%.0f", r.Input),
				fmt.Sprintf("%.0f", r.Output),
				fmt.Sprintf("%.0f", r.CacheRead),
				fmt.Sprintf("%.0f", r.CacheWrite),
				fmt.Sprintf("%.0f", r.Reasoning),
				fmt.Sprintf("%.0f", r.TotalTokens),
				fmt.Sprintf("%.4f", r.Cost),
				fmt.Sprintf("%.4f", r.CacheRate),
			})
		}
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}
