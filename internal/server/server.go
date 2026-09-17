package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	sourcequery "github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

//go:embed webui.html
var webUIContent []byte

const ActiveThreshold = 5 * time.Minute
const maxRenameBodyBytes int64 = 1 << 20

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
	if errors.Is(err, sessiondata.ErrUnknownSource) || errors.Is(err, sourcequery.ErrInvalidQuery) {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if errors.Is(err, sourcequery.ErrRequestsUnsupported) || errors.Is(err, sourcequery.ErrPiOnlyUnsupported) {
		sendError(w, http.StatusBadRequest, "Unsupported", err.Error())
		return
	}
	if errors.Is(err, db.ErrSourceRootMismatch) {
		sendError(w, http.StatusConflict, "Conflict", err.Error())
		return
	}
	sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
}
