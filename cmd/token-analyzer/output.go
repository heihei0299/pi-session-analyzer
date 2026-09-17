package main

import (
	"fmt"
	"os"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/render"
	"github.com/heihei0299/token-analyzer/internal/serialize"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func printQueryResult(format string, res *sessiondata.QueryResult, view sessiondata.View) error {
	if res.Meta != nil {
		for _, warning := range res.Meta.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
		}
	}

	switch format {
	case "json":
		bytes, _ := serialize.SerializeJSON(jsonOutputData(res))
		fmt.Println(string(bytes))
	case "csv":
		var data any
		if res.Rows != nil {
			data = res.Rows
		} else if res.Totals != nil {
			data = *res.Totals
		}
		bytes, err := serialize.SerializeCSV(string(view.Kind), data)
		if err != nil {
			return fmt.Errorf("CSV 序列化失败: %w", err)
		}
		fmt.Print(string(bytes))
	default:
		if rows, ok := res.Rows.([]domain.SessionRow); ok {
			fmt.Print(render.RenderSessionTable(rows))
		} else if rows, ok := res.Rows.([]domain.RequestRow); ok {
			fmt.Print(render.RenderRequestTable(rows))
		} else if rows, ok := res.Rows.([]domain.GroupRow); ok {
			fmt.Print(render.RenderGroupTable(rows, view.By))
		} else if rows, ok := res.Rows.([]domain.PeriodRow); ok {
			fmt.Print(render.RenderPeriodTable(rows, view.Period))
		} else if res.Totals != nil {
			fmt.Print(render.RenderTotalsTable(*res.Totals))
		}
	}
	return nil
}

func jsonOutputData(res *sessiondata.QueryResult) map[string]any {
	var outData map[string]any
	if res.Window == "totals" && res.Rows == nil && res.Totals != nil {
		outData = map[string]any{
			"window":      "totals",
			"requests":    res.Totals.Requests,
			"input":       res.Totals.Input,
			"output":      res.Totals.Output,
			"cacheRead":   res.Totals.CacheRead,
			"cacheWrite":  res.Totals.CacheWrite,
			"reasoning":   res.Totals.Reasoning,
			"totalTokens": res.Totals.TotalTokens,
			"cost":        res.Totals.Cost,
			"cacheRate":   res.Totals.CacheRate,
		}
		if res.Totals.CostStatus != "" {
			outData["costStatus"] = res.Totals.CostStatus
		}
	} else if res.Window == "totals" && res.By != "" {
		outData = map[string]any{
			"window": "totals",
			"by":     string(res.By),
			"rows":   res.Rows,
		}
	} else if res.Window == "totals" && res.Period != "" {
		outData = map[string]any{
			"window": "totals",
			"period": string(res.Period),
			"rows":   res.Rows,
		}
	} else {
		outData = map[string]any{
			"window": res.Window,
			"rows":   res.Rows,
		}
	}
	if res.Meta != nil {
		outData["meta"] = res.Meta
	}
	return outData
}
