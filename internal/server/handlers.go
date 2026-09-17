package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

func (s *Server) parseFilter(r *http.Request) (sessiondata.Filter, error) {
	q := r.URL.Query()
	f := sessiondata.Filter{
		Model: q.Get("model"),
		Cwd:   q.Get("cwd"),
	}
	if rawSource := q.Get("source"); rawSource != "" {
		source, err := sessiondata.NormalizeSource(rawSource)
		if err != nil {
			return f, err
		}
		f.Source = source
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

func rejectUnsupportedQueryOptions(q url.Values) error {
	for _, option := range []string{"page", "size", "sortKey", "sortDir"} {
		if _, ok := q[option]; ok {
			return fmt.Errorf("unsupported query option: %s", option)
		}
	}
	return nil
}

func (s *Server) handleApiTotals(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if err := rejectUnsupportedQueryOptions(r.URL.Query()); err != nil {
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
	page, size, err := parsePagination(q)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
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
	page, size, err := parsePagination(q)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
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

func parsePagination(q url.Values) (page, size int, err error) {
	parse := func(name string) (int, error) {
		raw := strings.TrimSpace(q.Get(name))
		if raw == "" {
			return 0, nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", name)
		}
		return value, nil
	}
	page, err = parse("page")
	if err != nil {
		return 0, 0, err
	}
	size, err = parse("size")
	if err != nil {
		return 0, 0, err
	}
	if (page == 0) != (size == 0) {
		return 0, 0, fmt.Errorf("page and size must be provided together")
	}
	return page, size, nil
}

func (s *Server) handleApiGroups(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseFilter(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if err := rejectUnsupportedQueryOptions(r.URL.Query()); err != nil {
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
	if err := rejectUnsupportedQueryOptions(r.URL.Query()); err != nil {
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
	if err := rejectUnsupportedQueryOptions(r.URL.Query()); err != nil {
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
