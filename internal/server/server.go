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
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/domain"
	sourcequery "github.com/heihei0299/pi-session-anylize/internal/query"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
	"github.com/heihei0299/pi-session-anylize/internal/timerange"
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
	dir         string
	sessionData *sessiondata.SessionData
	queryConfig sourcequery.Config
	mux         *http.ServeMux
}

func NewServer(dir string, sd *sessiondata.SessionData, options ...Options) *Server {
	if sd == nil {
		sd = sessiondata.DefaultSessionData
	}
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Source == "" {
		opts.Source = "pi"
	}
	s := &Server{
		dir:         dir,
		sessionData: sd,
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

func (s *Server) query(filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	return sourcequery.Query(s.sessionData, s.queryConfig, filter, view)
}

func (s *Server) handleApiDbMeta(w http.ResponseWriter, r *http.Request) {
	dbPath := db.ResolveDbPathFromEnv(r.URL.Query().Get("db"))
	database, err := db.Open(dbPath)
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

	sendJSON(w, http.StatusOK, map[string]any{
		"dbPath":          dbPath,
		"schemaVersion":   db.SchemaVersion,
		"lastSyncAt":      lastSyncAt,
		"rollupWatermark": rollupWatermark,
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
	detail, err := s.sessionData.QueryDetail(s.dir, sessionId)
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

	// 查找会话文件（走 SessionData 快照缓存）
	matchedFile, err := s.sessionData.FindSessionFile(s.dir, req.SessionId)
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
		s.sessionData.InvalidateCache(s.dir)
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"fileName": filepath.Base(target),
	})
}
