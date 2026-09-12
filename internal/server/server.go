package server

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/pi"
	sourcequery "github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

//go:embed webui.html
var webUIContent []byte

const ActiveThreshold = 5 * time.Minute

type Options struct {
	Source   string
	CodexDir string
	DBPath   string
}

type Server struct {
	queryConfig sourcequery.Config
	mux         *http.ServeMux

	// refresh orchestration：GET 只读快照，同步只走 RefreshNow
	//（启动初次同步 + watcher 变更触发），并发触发由 refresh 包串行化。
	// 错误按源记录：pi 查询只看 pi 的同步状态，不被 Codex 失败污染。
	refreshMu      sync.RWMutex
	lastRefreshErr map[string]error
	lastRefreshAt  time.Time
}

func NewServer(dir string, options ...Options) *Server {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Source == "" {
		opts.Source = "pi"
	}
	s := &Server{
		queryConfig: sourcequery.Config{PiDir: dir, Source: opts.Source, CodexDir: opts.CodexDir, DBPath: opts.DBPath},
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /index.html", s.handleIndex)

	s.mux.HandleFunc("GET /api/totals", s.handleApiTotals)
	s.mux.HandleFunc("GET /api/sessions", s.handleApiSessions)
	s.mux.HandleFunc("GET /api/sessions/detail", s.handleApiSessionDetail)
	s.mux.HandleFunc("GET /api/sessions/{id}/detail", s.handleApiSessionDetailPath)
	s.mux.HandleFunc("POST /api/sessions/rename", s.handleApiSessionRename)
	s.mux.HandleFunc("GET /api/requests", s.handleApiRequests)
	s.mux.HandleFunc("GET /api/groups", s.handleApiGroups)
	s.mux.HandleFunc("GET /api/period", s.handleApiPeriod)
	s.mux.HandleFunc("GET /api/meta", s.handleApiMeta)
	s.mux.HandleFunc("GET /api/db/meta", s.handleApiDbMeta)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(webUIContent)
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, errName, detail string) {
	sendJSON(w, status, map[string]string{
		"error":  errName,
		"detail": detail,
	})
}
func sendQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, sourcequery.ErrRequestsUnsupported) || strings.HasPrefix(err.Error(), "未知 source:") {
		sendError(w, http.StatusBadRequest, "Unsupported", err.Error())
		return
	}
	sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
}

func (s *Server) parseFilter(r *http.Request) (sessiondata.Filter, error) {
	q := r.URL.Query()
	f := sessiondata.Filter{
		Model:  q.Get("model"),
		Cwd:    q.Get("cwd"),
		Source: q.Get("source"),
	}
	since := q.Get("since")
	until := q.Get("until")
	if since != "" || until != "" {
		tr, err := timerange.MakeMessageRange(since, until)
		if err != nil {
			return f, err
		}
		f.TimeRange = tr
	}
	return f, nil
}

func (s *Server) handleApiTotals(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := s.query(f, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	if res != nil && res.Totals != nil {
		out := map[string]any{
			"window":      res.Window,
			"totals":      res.Totals,
			"meta":        res.Meta,
			"requests":    res.Totals.Requests,
			"input":       res.Totals.Input,
			"output":      res.Totals.Output,
			"cacheRead":   res.Totals.CacheRead,
			"cacheWrite":  res.Totals.CacheWrite,
			"reasoning":   res.Totals.Reasoning,
			"totalTokens": res.Totals.TotalTokens,
			"cost":        res.Totals.Cost,
			"cacheRate":   res.Totals.CacheRate,
			"costStatus":  res.Totals.CostStatus,
		}
		sendJSON(w, http.StatusOK, out)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleApiSessions(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	sortKey := q.Get("sortKey")
	sortDir := q.Get("sortDir")

	res, err := s.query(f, sessiondata.View{
		Kind:    sessiondata.ViewSessions,
		Page:    page,
		Size:    size,
		SortKey: sortKey,
		SortDir: sortDir,
	})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleApiRequests(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	sortKey := q.Get("sortKey")
	sortDir := q.Get("sortDir")

	res, err := s.query(f, sessiondata.View{
		Kind:    sessiondata.ViewRequests,
		Page:    page,
		Size:    size,
		SortKey: sortKey,
		SortDir: sortDir,
	})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleApiGroups(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	by := domain.GroupBy(r.URL.Query().Get("by"))
	if by != domain.GroupByModel && by != domain.GroupByCwd && by != domain.GroupByModelCwd {
		sendError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("未知分组: %s（支持 model/cwd/model,cwd）", by))
		return
	}

	res, err := s.query(f, sessiondata.View{
		Kind: sessiondata.ViewGroups,
		By:   by,
	})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleApiPeriod(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	p := domain.Period(r.URL.Query().Get("period"))
	if p != domain.PeriodDay && p != domain.PeriodWeek && p != domain.PeriodMonth {
		sendError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("未知周期: %s（支持 day/week/month）", p))
		return
	}

	res, err := s.query(f, sessiondata.View{
		Kind:   sessiondata.ViewPeriod,
		Period: p,
	})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleApiMeta(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := s.query(f, sessiondata.View{Kind: sessiondata.ViewMeta})
	if err != nil {
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res.Meta)
}

func (s *Server) refreshSource(source string) error {
	cfg := s.queryConfig
	err := refresh.Refresh(refresh.Config{PiDir: cfg.PiDir, CodexDir: cfg.CodexDir, DBPath: cfg.DBPath, Source: source})
	s.refreshMu.Lock()
	if s.lastRefreshErr == nil {
		s.lastRefreshErr = make(map[string]error)
	}
	s.lastRefreshErr[source] = err
	s.lastRefreshAt = time.Now()
	s.refreshMu.Unlock()
	return err
}

// RefreshNow 执行统一同步（Pi + Codex 分源覆盖 ?source= 的各种切换），
// 记录结果供 meta 对外暴露。失败保留上一成功 snapshot，只记错不抛快照。
func (s *Server) RefreshNow() error {
	piErr := s.refreshSource("pi")
	codexErr := s.refreshSource("codex")
	return errors.Join(piErr, codexErr)
}

func (s *Server) lastRefresh() (time.Time, map[string]error) {
	s.refreshMu.RLock()
	defer s.refreshMu.RUnlock()
	return s.lastRefreshAt, s.lastRefreshErr
}

// StartWatch 以指纹轮询驱动 change → refresh → query：目录有变才同步，
// 否则所有 GET 都只读快照。返回 stop，调用方（serve）在退出时调用。
func (s *Server) StartWatch(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	done := make(chan struct{})
	watchPi := s.queryConfig.Source == "pi" || s.queryConfig.Source == "all"
	watchCodex := s.queryConfig.Source == "codex" || s.queryConfig.Source == "all"
	piAcknowledged := ""
	codexAcknowledged := ""
	if watchPi {
		piAcknowledged, _ = refresh.PiFingerprint(s.queryConfig.PiDir)
	}
	if watchCodex {
		codexAcknowledged, _ = refresh.CodexFingerprint(s.queryConfig.CodexDir)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if watchPi {
					current, err := refresh.PiFingerprint(s.queryConfig.PiDir)
					if err == nil && current != piAcknowledged {
						if err := s.refreshSource("pi"); err == nil {
							piAcknowledged = current
						}
					}
				}
				if watchCodex {
					current, err := refresh.CodexFingerprint(s.queryConfig.CodexDir)
					if err == nil && current != codexAcknowledged {
						if err := s.refreshSource("codex"); err == nil {
							codexAcknowledged = current
						}
					}
				}
			}
		}
	}()
	return func() { close(done); wg.Wait() }
}

// exposeRefreshError 把与本次查询源相关的同步失败经 meta 警告暴露
// （快照本身不受影响；pi 查询不被 codex 失败污染）。
func (s *Server) exposeRefreshError(res *sessiondata.QueryResult, source string) {
	if res == nil {
		return
	}
	if source == "" {
		source = "pi"
	}
	_, errs := s.lastRefresh()
	var failed []string
	if (source == "pi" || source == "all") && errs["pi"] != nil {
		failed = append(failed, fmt.Sprintf("Pi 同步失败（已保留上次成功快照）: %v", errs["pi"]))
	}
	if (source == "codex" || source == "all") && errs["codex"] != nil {
		failed = append(failed, fmt.Sprintf("Codex 同步失败（已保留上次成功快照）: %v", errs["codex"]))
	}
	if len(failed) == 0 {
		return
	}
	if res.Meta == nil {
		res.Meta = &sessiondata.QueryMeta{Sources: sourcequery.SupportedSources(), Warnings: failed}
		return
	}
	res.Meta.Warnings = append(res.Meta.Warnings, failed...)
}

// query 是统一查询入口：只读已提交 snapshot，不触发同步。
// CLI 走同一 Refresh + Query 映射，相同 Query Request 得到同一 domain 结果。
func (s *Server) query(filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	source := s.queryConfig.Source
	if filter.Source != "" {
		source = filter.Source
	}
	res, err := sourcequery.Query(s.queryConfig, filter, view)
	if err != nil {
		return nil, err
	}
	s.exposeRefreshError(res, source)
	return res, nil
}

func (s *Server) handleApiDbMeta(w http.ResponseWriter, r *http.Request) {
	dbPath := s.queryConfig.DBPath
	if queryPath := r.URL.Query().Get("db"); queryPath != "" {
		dbPath = queryPath
	}
	dbPath = db.ResolveDbPathFromEnv(dbPath)
	// 只读：db/meta 本身是观察口，不能推进游标或建库。
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	defer database.DB.Close()

	var lastSyncAt *int64
	var lastSyncVal sql.NullInt64
	row := database.DB.QueryRow(`SELECT MAX(last_synced_at) FROM session_log_sync`)
	if err := row.Scan(&lastSyncVal); err == nil && lastSyncVal.Valid {
		v := lastSyncVal.Int64
		lastSyncAt = &v
	}

	var rollupWatermark *string
	var rollupVal sql.NullString
	row2 := database.DB.QueryRow(`SELECT MIN(date) FROM usage_daily_rollups`)
	if err := row2.Scan(&rollupVal); err == nil && rollupVal.Valid {
		v := rollupVal.String
		rollupWatermark = &v
	}

	lastRefreshAt, lastRefreshErrs := s.lastRefresh()
	var lastRefreshError *string
	if joined := errors.Join(lastRefreshErrs["pi"], lastRefreshErrs["codex"]); joined != nil {
		msg := joined.Error()
		lastRefreshError = &msg
	}
	var lastRefreshUnix *int64
	if !lastRefreshAt.IsZero() {
		v := lastRefreshAt.Unix()
		lastRefreshUnix = &v
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"dbPath":           dbPath,
		"schemaVersion":    db.SchemaVersion,
		"lastSyncAt":       lastSyncAt,
		"rollupWatermark":  rollupWatermark,
		"lastRefreshAt":    lastRefreshUnix,
		"lastRefreshError": lastRefreshError,
	})
}

// effectiveSource 解析本次请求的数据源：请求参数优先，其次 serve 启动时的 --source，最后默认 pi。
func (s *Server) effectiveSource(query url.Values) string {
	if v := strings.TrimSpace(query.Get("source")); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.queryConfig.Source); v != "" {
		return v
	}
	return "pi"
}

// rejectNonPiSource 对仅覆盖 Pi 会话的能力（会话详情、重命名）给出明确 unsupported，
// 而不是回落到 Pi 目录去猜、也不会写错目录。
func (s *Server) rejectNonPiSource(w http.ResponseWriter, query url.Values, feature string) bool {
	source := s.effectiveSource(query)
	if source == "pi" {
		return false
	}
	sendError(w, http.StatusBadRequest, "Unsupported", fmt.Sprintf("%s 数据源不支持%s：该能力仅覆盖 Pi 会话", source, feature))
	return true
}

func (s *Server) handleApiSessionDetailPath(w http.ResponseWriter, r *http.Request) {
	if s.rejectNonPiSource(w, r.URL.Query(), "会话详情") {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		sendError(w, http.StatusBadRequest, "Bad Request", "缺少 sessionId")
		return
	}
	s.queryAndSendDetail(w, id)
}

func (s *Server) handleApiSessionDetail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if s.rejectNonPiSource(w, q, "会话详情") {
		return
	}
	sessionId := q.Get("sessionId")
	if sessionId == "" {
		sessionId = q.Get("sessionID")
	}
	if sessionId == "" {
		sessionId = q.Get("id")
	}
	if sessionId == "" {
		sendError(w, http.StatusBadRequest, "Bad Request", "缺少 sessionId")
		return
	}
	s.queryAndSendDetail(w, sessionId)
}

func (s *Server) queryAndSendDetail(w http.ResponseWriter, sessionId string) {
	detail, err := sourcequery.QueryDetail(s.queryConfig, sessionId)
	if err != nil {
		if errors.Is(err, sessiondata.ErrSessionNotFound) {
			sendError(w, http.StatusNotFound, "Not Found", err.Error())
			return
		}
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	sendJSON(w, http.StatusOK, detail)
}

type renameRequest struct {
	SessionId string `json:"sessionId"`
	Name      string `json:"name"`
}

func sanitizeFilename(name string) string {
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	res := name
	for _, c := range invalidChars {
		res = strings.ReplaceAll(res, c, "")
	}
	return strings.TrimSpace(res)
}

func (s *Server) handleApiSessionRename(w http.ResponseWriter, r *http.Request) {
	if s.rejectNonPiSource(w, r.URL.Query(), "重命名") {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", "读取请求体失败")
		return
	}
	var req renameRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", "请求体不是合法 JSON")
		return
	}
	if strings.TrimSpace(req.SessionId) == "" {
		sendError(w, http.StatusBadRequest, "Bad Request", "缺少 sessionId")
		return
	}
	sanitized := sanitizeFilename(req.Name)
	if sanitized == "" {
		sendError(w, http.StatusBadRequest, "Bad Request", "显示名非法（去除非法字符后为空）")
		return
	}

	resolved, err := pi.ResolvePiSessionRoot(
		os.Getenv("PI_CODING_AGENT_SESSION_DIR"),
		s.queryConfig.PiDir,
		pi.GetPiNativeSessionDir(),
	)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	matchedFile, err := pi.FindSessionFileByHeaderID(resolved.Root, resolved.Layout, req.SessionId)
	if err != nil {
		sendError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("会话不存在: %s", req.SessionId))
		return
	}

	base := strings.TrimSuffix(filepath.Base(matchedFile), ".jsonl")
	idx := strings.LastIndex(base, "_")
	if idx <= 0 || idx == len(base)-1 {
		sendError(w, http.StatusBadRequest, "Bad Request", "无法识别会话 UUID（文件名缺少 _<UUID> 尾缀）")
		return
	}
	tail := base[idx+1:]
	if tail != req.SessionId {
		sendError(w, http.StatusBadRequest, "Bad Request", "会话 UUID 与 header id 不一致")
		return
	}

	fi, err := os.Stat(matchedFile)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	if time.Since(fi.ModTime()) <= ActiveThreshold {
		sendError(w, http.StatusConflict, "Conflict", "会话活跃中，稍后再试")
		return
	}

	target := filepath.Join(filepath.Dir(matchedFile), fmt.Sprintf("%s_%s.jsonl", sanitized, tail))
	if target != matchedFile {
		if _, err := os.Stat(target); err == nil {
			sendError(w, http.StatusConflict, "Conflict", "同名文件已存在")
			return
		}
		if err := os.Rename(matchedFile, target); err != nil {
			sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		if err := refresh.Refresh(refresh.Config{PiDir: s.queryConfig.PiDir, DBPath: s.queryConfig.DBPath, Source: "pi"}); err != nil {
			sendError(w, http.StatusServiceUnavailable, "Service Unavailable", fmt.Sprintf("文件已改名但 snapshot refresh failed: %v", err))
			return
		}
		if _, err := sourcequery.QueryDetail(s.queryConfig, req.SessionId); err != nil {
			sendError(w, http.StatusServiceUnavailable, "Service Unavailable", fmt.Sprintf("文件已改名但 snapshot refresh failed: snapshot verification: %v", err))
			return
		}
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"fileName": filepath.Base(target),
	})
}
