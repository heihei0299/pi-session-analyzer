package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/pi"
	sourcequery "github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

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
	source, err := sessiondata.NormalizeSource(s.effectiveSource(query))
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return true
	}
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
	s.queryAndSendDetail(w, id, s.effectiveSource(r.URL.Query()))
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
	s.queryAndSendDetail(w, sessionId, s.effectiveSource(q))
}

func (s *Server) queryAndSendDetail(w http.ResponseWriter, sessionID, source string) {
	cfg := s.queryConfig
	cfg.Source = source
	detail, err := sourcequery.QueryDetail(cfg, sessionID)
	if err != nil {
		if errors.Is(err, sessiondata.ErrSessionNotFound) {
			sendError(w, http.StatusNotFound, "Not Found", err.Error())
			return
		}
		sendQueryError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, detail)
}

type renameRequest struct {
	SessionId string `json:"sessionId"`
	Name      string `json:"name"`
}

func sanitizeFilename(name string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\\:*?"<>|`, r) {
			return -1
		}
		return r
	}, name))
}

func (s *Server) handleApiSessionRename(w http.ResponseWriter, r *http.Request) {
	if s.rejectNonPiSource(w, r.URL.Query(), "重命名") {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRenameBodyBytes+1))
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", "读取请求体失败")
		return
	}
	if int64(len(body)) > maxRenameBodyBytes {
		sendError(w, http.StatusBadRequest, "Bad Request", "请求体过大")
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
	if resolved.Root != "" {
		resolved.Root, err = db.CanonicalSourceRoot(resolved.Root)
		if err != nil {
			sendQueryError(w, err)
			return
		}
	}
	ledger, err := db.OpenReadOnly(db.ResolveDbPathFromEnv(s.queryConfig.DBPath))
	if err != nil {
		sendQueryError(w, err)
		return
	}
	defer ledger.Close()
	if err := db.CheckPinnedSourceRoot(ledger, "pi", resolved.Root); err != nil {
		sendQueryError(w, err)
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

	if err := pi.VerifyPinnedSessionFile(resolved.Root, matchedFile); err != nil {
		sendQueryError(w, err)
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
	if err := pi.VerifyPinnedTarget(resolved.Root, target); err != nil {
		sendQueryError(w, err)
		return
	}
	if target != matchedFile {
		if _, err := os.Stat(target); err == nil {
			sendError(w, http.StatusConflict, "Conflict", "同名文件已存在")
			return
		}
		if err := pi.VerifyPinnedSessionFile(resolved.Root, matchedFile); err != nil {
			sendQueryError(w, err)
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
