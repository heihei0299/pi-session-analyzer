package main

import (
	"errors"
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
	"github.com/heihei0299/token-analyzer/internal/server"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

var Version = "dev"

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
		} else {
			fmt.Fprintf(os.Stderr, "错误: 未知 command: %s\n", cmd)
			os.Exit(2)
		}
	}

	fs := flag.NewFlagSet("token-analyzer", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
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

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "错误: 未知参数或 command: %s\n", strings.Join(fs.Args(), " "))
		os.Exit(2)
	}
	if err := validateCLIOptions(window, *source, *format, *by, *period, *watchMode, *interval, *port, *host); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(2)
	}

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
		if err := runWatch(*dir, *codexDir, *dbPath, *interval, filter); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(2)
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

	if err := query.Validate(query.Config{PiDir: *dir, CodexDir: *codexDir, DBPath: *dbPath, Source: *source}, filter, view); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(2)
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

	if err := printQueryResult(*format, res, view); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func validateCLIOptions(window, source, format, by, period string, watch bool, interval, port int, host string) error {
	if _, err := sessiondata.NormalizeSource(source); err != nil {
		return err
	}
	switch format {
	case "table", "json", "csv":
	default:
		return fmt.Errorf("未知 format: %s（支持 table|json|csv）", format)
	}
	if by != "" {
		if window != "totals" {
			return fmt.Errorf("--by 仅适用于 totals command")
		}
		value := domain.GroupBy(by)
		if value != domain.GroupByModel && value != domain.GroupByCwd && value != domain.GroupByModelCwd {
			return fmt.Errorf("未知 group: %s（支持 model|cwd|model,cwd）", by)
		}
	}
	if period != "" {
		if window != "totals" {
			return fmt.Errorf("--period 仅适用于 totals command")
		}
		value := domain.Period(period)
		if value != domain.PeriodDay && value != domain.PeriodWeek && value != domain.PeriodMonth {
			return fmt.Errorf("未知 period: %s（支持 day|week|month）", period)
		}
	}
	if by != "" && period != "" {
		return fmt.Errorf("--by 与 --period 不能同时使用")
	}
	if watch && interval <= 0 {
		return fmt.Errorf("--interval 必须为正数毫秒")
	}
	if window == "serve" {
		if port <= 0 || port > 65535 {
			return fmt.Errorf("--port 必须在 1 到 65535 之间")
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("--host 不能为空")
		}
	}
	return nil
}
