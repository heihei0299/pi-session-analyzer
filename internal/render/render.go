package render

import (
	"fmt"
	"strings"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func metricHeaders() []string {
	return []string{"请求数", "输入", "输出", "缓存读", "缓存写", "推理", "总 token", "花费", "缓存率"}
}

func metricWidths() []int {
	return []int{8, 10, 10, 10, 10, 10, 12, 10, 8}
}

func metricValues(t domain.Totals) []string {
	rateStr := fmt.Sprintf("%.1f%%", t.CacheRate*100)
	costStr := fmt.Sprintf("$%.4f", t.Cost)
	if t.CostStatus != "" {
		costStr = t.CostStatus
	} else if t.Cost == 0 {
		costStr = "$0.00"
	}
	return []string{
		fmt.Sprintf("%d", t.Requests),
		fmt.Sprintf("%.0f", t.Input),
		fmt.Sprintf("%.0f", t.Output),
		fmt.Sprintf("%.0f", t.CacheRead),
		fmt.Sprintf("%.0f", t.CacheWrite),
		fmt.Sprintf("%.0f", t.Reasoning),
		fmt.Sprintf("%.0f", t.TotalTokens),
		costStr,
		rateStr,
	}
}

func RenderTotalsTable(t domain.Totals) string {
	headers := metricHeaders()
	vals := metricValues(t)
	widths := metricWidths()
	return renderRows([][]string{headers, vals}, widths)
}

func RenderSessionTable(rows []domain.SessionRow) string {
	headers := append([]string{"会话ID", "时间戳", "cwd", "模型"}, metricHeaders()...)
	widths := append([]int{24, 26, 32, 12}, metricWidths()...)

	var allRows [][]string
	allRows = append(allRows, headers)
	for _, r := range rows {
		vals := append([]string{r.SessionId, r.Timestamp, r.Cwd, r.Model}, metricValues(r.Totals)...)
		allRows = append(allRows, vals)
	}
	return renderRows(allRows, widths)
}

func RenderRequestTable(rows []domain.RequestRow) string {
	headers := append([]string{"会话ID", "时间戳", "模型"}, metricHeaders()...)
	widths := append([]int{24, 26, 12}, metricWidths()...)

	var allRows [][]string
	allRows = append(allRows, headers)
	for _, r := range rows {
		vals := append([]string{r.SessionId, r.Timestamp, r.Model}, metricValues(r.Totals)...)
		allRows = append(allRows, vals)
	}
	return renderRows(allRows, widths)
}

func RenderGroupTable(rows []domain.GroupRow, by domain.GroupBy) string {
	byModel := by == domain.GroupByModel || by == domain.GroupByModelCwd
	byCwd := by == domain.GroupByCwd || by == domain.GroupByModelCwd

	var keyCols []string
	var keyWidths []int
	if byModel {
		keyCols = append(keyCols, "模型")
		keyWidths = append(keyWidths, 12)
	}
	if byCwd {
		keyCols = append(keyCols, "cwd")
		keyWidths = append(keyWidths, 32)
	}

	headers := append(keyCols, metricHeaders()...)
	widths := append(keyWidths, metricWidths()...)

	var allRows [][]string
	allRows = append(allRows, headers)
	for _, r := range rows {
		var prefix []string
		if byModel {
			prefix = append(prefix, r.Model)
		}
		if byCwd {
			prefix = append(prefix, r.Cwd)
		}
		allRows = append(allRows, append(prefix, metricValues(r.Totals)...))
	}
	return renderRows(allRows, widths)
}

func RenderPeriodTable(rows []domain.PeriodRow, period domain.Period) string {
	pHeader := "日期"
	if period == domain.PeriodWeek {
		pHeader = "周起始"
	} else if period == domain.PeriodMonth {
		pHeader = "月份"
	}
	headers := append([]string{pHeader}, metricHeaders()...)
	widths := append([]int{12}, metricWidths()...)

	var allRows [][]string
	allRows = append(allRows, headers)
	for _, r := range rows {
		vals := append([]string{r.Period}, metricValues(r.Totals)...)
		allRows = append(allRows, vals)
	}
	return renderRows(allRows, widths)
}

func renderRows(rows [][]string, widths []int) string {
	var sb strings.Builder
	for i, row := range rows {
		var cells []string
		for colIdx, cell := range row {
			w := 10
			if colIdx < len(widths) {
				w = widths[colIdx]
			}
			cells = append(cells, padCell(cell, w))
		}
		sb.WriteString(strings.Join(cells, "  "))
		sb.WriteString("\n")
		if i == 0 {
			var divCells []string
			for colIdx := range row {
				w := 10
				if colIdx < len(widths) {
					w = widths[colIdx]
				}
				divCells = append(divCells, strings.Repeat("-", w))
			}
			sb.WriteString(strings.Join(divCells, "  "))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func padCell(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}
