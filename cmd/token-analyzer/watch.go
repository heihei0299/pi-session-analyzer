package main

import (
	"fmt"
	"os"
	"time"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func runWatch(piDir, codexDir, dbPath string, interval int, filter sessiondata.Filter) error {
	source := filter.Source
	if source == "" {
		source = "pi"
	}
	if err := query.Validate(query.Config{PiDir: piDir, CodexDir: codexDir, DBPath: dbPath, Source: source}, filter, sessiondata.View{Kind: sessiondata.ViewTotals}); err != nil {
		return err
	}

	fmt.Printf("开始监控数据源: %s (轮询间隔: %dms)...\n", source, interval)
	printWatchTotals := func() error {
		tot, err := queryWatchTotals(piDir, codexDir, dbPath, filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "watch 失败（保留上次快照）: %v\n", err)
			return err
		}
		now := time.Now().Format("15:04:05")
		fmt.Printf("[%s] 请求数: %d | 总 Token: %.0f | 花费: $%.4f\n", now, tot.Requests, tot.TotalTokens, tot.Cost)
		return nil
	}
	refreshSource := func(source string) error {
		return refresh.Refresh(refresh.Config{PiDir: piDir, CodexDir: codexDir, DBPath: dbPath, Source: source})
	}
	lastPi, lastCodex := "", ""
	if err := refreshSource(source); err == nil {
		if source == "pi" || source == "all" {
			lastPi, _ = refresh.PiFingerprint(piDir)
		}
		if source == "codex" || source == "all" {
			lastCodex, _ = refresh.CodexFingerprint(codexDir)
		}
		_ = printWatchTotals()
	} else {
		fmt.Fprintf(os.Stderr, "watch 失败（保留上次快照）: %v\n", err)
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		refreshed := false
		if source == "pi" || source == "all" {
			cur, err := refresh.PiFingerprint(piDir)
			if err == nil && cur != lastPi {
				if err := refreshSource("pi"); err != nil {
					fmt.Fprintf(os.Stderr, "watch pi 失败（保留上次快照）: %v\n", err)
				} else {
					lastPi = cur
					refreshed = true
				}
			}
		}
		if source == "codex" || source == "all" {
			cur, err := refresh.CodexFingerprint(codexDir)
			if err == nil && cur != lastCodex {
				if err := refreshSource("codex"); err != nil {
					fmt.Fprintf(os.Stderr, "watch codex 失败（保留上次快照）: %v\n", err)
				} else {
					lastCodex = cur
					refreshed = true
				}
			}
		}
		if refreshed {
			_ = printWatchTotals()
		}
	}
	return nil
}

// watchTotalsOnce 是 --watch 每次 tick 的全部工作：refresh 后查同一快照。
// 与普通查询走同一 Refresh + Query 映射，故 watch totals 与同一时刻
// 普通 Query totals 天然一致；source 规则只在 adapter 内实现，这里不累加。
func watchTotalsOnce(piDir, codexDir, dbPath string, filter sessiondata.Filter) (*domain.Totals, error) {
	source := filter.Source
	if source == "" {
		source = "pi"
	}
	if err := refresh.Refresh(refresh.Config{PiDir: piDir, CodexDir: codexDir, DBPath: dbPath, Source: source}); err != nil {
		return nil, err
	}
	return queryWatchTotals(piDir, codexDir, dbPath, filter)
}

func queryWatchTotals(piDir, codexDir, dbPath string, filter sessiondata.Filter) (*domain.Totals, error) {
	source := filter.Source
	if source == "" {
		source = "pi"
	}
	res, err := query.Query(query.Config{PiDir: piDir, CodexDir: codexDir, DBPath: dbPath, Source: source}, filter, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		return nil, err
	}
	if res.Totals == nil {
		return nil, fmt.Errorf("watch 查询无 totals")
	}
	return res.Totals, nil
}
