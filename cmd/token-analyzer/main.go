package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/render"
	"github.com/heihei0299/token-analyzer/internal/serialize"
	"github.com/heihei0299/token-analyzer/internal/server"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

const Version = "2026.9.3"

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

func printHelp() {
	fmt.Println(`用法:
  token-analyzer [totals|sessions|requests] --dir <path> [选项]
  token-analyzer serve [--port <n>] [--host <h>] [--dir <path>]

选项:
  --dir <path>      Pi 数据目录 (默认 ~/.pi/agent/sessions)
  --source <s>       数据源 pi|codex|all (默认 pi)
  --codex-dir <path> Codex 数据目录 (优先级高于 CODEX_HOME/~/.codex)
  --db <path>        normalized ledger 路径 (默认按 TOKEN_ANALYZER_DB)
  --format <format> 输出格式 table|json|csv (默认 table)
  --model <id>      按模型过滤
  --cwd <path>      按项目路径过滤
  --since <time>    起始时间
  --until <time>    截止时间
  --by <dim>        按维度汇总 model|cwd|model,cwd (仅 totals)
  --period <p>      按周期汇总 day|week|month (仅 totals)
  --watch           实时监控模式
  --interval <ms>   watch 轮询间隔 (默认 1000)
  --port <n>        serve 监听端口 (默认 50080)
  --host <h>        serve 监听地址 (默认 127.0.0.1)
  -v, --version     显示版本
  -h, --help        显示帮助`)
}

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "-v" || os.Args[1] == "--version" {
			fmt.Printf("token-analyzer %s\n", Version)
			return
		}
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			printHelp()
			return
		}
	}

	window := "totals"
	args := os.Args[1:]

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd := args[0]
		if cmd == "serve" || cmd == "totals" || cmd == "sessions" || cmd == "requests" {
			window = cmd
			args = args[1:]
		}
	}

	fs := flag.NewFlagSet("token-analyzer", flag.ExitOnError)
	dir := fs.String("dir", defaultDir(), "Pi 数据目录")
	source := fs.String("source", "pi", "数据源 pi|codex|all")
	codexDir := fs.String("codex-dir", "", "Codex 数据目录")
	dbPath := fs.String("db", "", "normalized ledger 路径")
	format := fs.String("format", "table", "输出格式 table|json|csv")
	model := fs.String("model", "", "按模型过滤")
	cwd := fs.String("cwd", "", "按项目路径过滤")
	since := fs.String("since", "", "起始时间")
	until := fs.String("until", "", "截止时间")
	by := fs.String("by", "", "分组汇总 model|cwd|model,cwd")
	period := fs.String("period", "", "时间周期汇总 day|week|month")
	watchMode := fs.Bool("watch", false, "实时监控模式")
	interval := fs.Int("interval", 1000, "轮询间隔 (ms)")
	port := fs.Int("port", 50080, "serve 监听端口")
	host := fs.String("host", "127.0.0.1", "serve 监听地址")

	_ = fs.Parse(args)

	// Serve 模式：初次 Refresh 完成后才对外提供 snapshot 查询，
	// 之后 watcher 以 change → refresh → query 驱动，GET 只读快照。
	if window == "serve" {
		srv := server.NewServer(*dir, server.Options{Source: *source, CodexDir: *codexDir, DBPath: *dbPath})
		if err := srv.RefreshNow(); err != nil {
			fmt.Fprintf(os.Stderr, "初次同步失败（将提供既有快照并经 meta 暴露）: %v\n", err)
		}
		stopWatch := srv.StartWatch(2 * time.Second)
		defer stopWatch()
		addr := fmt.Sprintf("%s:%d", *host, *port)
		fmt.Printf("Token Analyzer WebUI 已启动: http://%s/\n数据目录: %s\n", addr, *dir)
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
			fmt.Fprintf(os.Stderr, "启动服务器失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 构造时间过滤
	var tr *timerange.TimeRange
	if *since != "" || *until != "" {
		var err error
		tr, err = timerange.MakeSessionRange(*since, *until)
		if err != nil {
			fmt.Fprintf(os.Stderr, "时间范围错误: %v\n", err)
			os.Exit(1)
		}
	}

	filter := sessiondata.Filter{
		Model:     *model,
		Source:    *source,
		Cwd:       *cwd,
		TimeRange: tr,
	}

	// Watch 实时监控模式：change → source refresh → query，与普通查询同一快照源。
	// source 解析/去重/费用等规则只在 adapter 内实现，这里不累加。
	if *watchMode {
		if *source != "pi" && *source != "codex" && *source != "all" {
			fmt.Fprintln(os.Stderr, "错误: --watch 的 --source 只支持 pi|codex|all")
			os.Exit(1)
		}
		fmt.Printf("开始监控数据源: %s (轮询间隔: %dms)...\n", *source, *interval)
		printWatchTotals := func() error {
			tot, err := queryWatchTotals(*dir, *codexDir, *dbPath, filter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "watch 失败（保留上次快照）: %v\n", err)
				return err
			}
			now := time.Now().Format("15:04:05")
			fmt.Printf("[%s] 请求数: %d | 总 Token: %.0f | 花费: $%.4f\n",
				now, tot.Requests, tot.TotalTokens, tot.Cost)
			return nil
		}
		refreshSource := func(source string) error {
			return refresh.Refresh(refresh.Config{PiDir: *dir, CodexDir: *codexDir, DBPath: *dbPath, Source: source})
		}
		lastPi, lastCodex := "", ""
		if err := refreshSource(*source); err == nil {
			if *source == "pi" || *source == "all" {
				lastPi, _ = refresh.PiFingerprint(*dir)
			}
			if *source == "codex" || *source == "all" {
				lastCodex, _ = refresh.CodexFingerprint(*codexDir)
			}
			_ = printWatchTotals()
		} else {
			fmt.Fprintf(os.Stderr, "watch 失败（保留上次快照）: %v\n", err)
		}
		ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			refreshed := false
			if *source == "pi" || *source == "all" {
				cur, err := refresh.PiFingerprint(*dir)
				if err == nil && cur != lastPi {
					if err := refreshSource("pi"); err != nil {
						fmt.Fprintf(os.Stderr, "watch pi 失败（保留上次快照）: %v\n", err)
					} else {
						lastPi = cur
						refreshed = true
					}
				}
			}
			if *source == "codex" || *source == "all" {
				cur, err := refresh.CodexFingerprint(*codexDir)
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
		return
	}

	// 视图判断
	var view sessiondata.View
	if *by != "" {
		view = sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupBy(*by)}
	} else if *period != "" {
		view = sessiondata.View{Kind: sessiondata.ViewPeriod, Period: domain.Period(*period)}
	} else {
		switch window {
		case "sessions":
			view = sessiondata.View{Kind: sessiondata.ViewSessions}
		case "requests":
			view = sessiondata.View{Kind: sessiondata.ViewRequests}
		default:
			view = sessiondata.View{Kind: sessiondata.ViewTotals}
		}
	}

	if err := refresh.Refresh(refresh.Config{PiDir: *dir, CodexDir: *codexDir, DBPath: *dbPath, Source: *source}); err != nil {
		fmt.Fprintf(os.Stderr, "同步失败: %v\n", err)
		os.Exit(1)
	}
	res, err := query.Query(query.Config{PiDir: *dir, CodexDir: *codexDir, DBPath: *dbPath, Source: *source}, filter, view)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		os.Exit(1)
	}

	if res.Meta != nil {
		for _, warning := range res.Meta.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
		}
	}

	// 格式化输出
	switch *format {
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
			fmt.Fprintf(os.Stderr, "CSV 序列化失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(bytes))
	default:
		// Table 格式
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
