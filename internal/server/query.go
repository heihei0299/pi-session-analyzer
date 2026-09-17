package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/heihei0299/token-analyzer/internal/db"
	sourcequery "github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

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
