package server

import (
	"errors"
	"fmt"
	"sync"
	"time"

	sourcequery "github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

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
	var errs map[string]error
	if s.lastRefreshErr != nil {
		errs = make(map[string]error, len(s.lastRefreshErr))
		for source, err := range s.lastRefreshErr {
			errs[source] = err
		}
	}
	return s.lastRefreshAt, errs
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
