package sessiondata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/timerange"
)

var isoDatePrefixRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}`)

type MessageItem struct {
	Timestamp string       `json:"timestamp"`
	Model     string       `json:"model"`
	Usage     domain.Usage `json:"usage"`
}

type SessionFileData struct {
	SessionId       string        `json:"sessionId"`
	Timestamp       string        `json:"timestamp"`
	Cwd             string        `json:"cwd"`
	FilePath        string        `json:"filePath,omitempty"`
	FileName        string        `json:"fileName"`
	IsTask          bool          `json:"isTask"`
	ParentSessionId string        `json:"parentSessionId,omitempty"`
	FirstUserText   string        `json:"firstUserText,omitempty"`
	Source          string        `json:"source,omitempty"`
	Items           []MessageItem `json:"items"`
}

type Filter struct {
	Model     string               `json:"model,omitempty"`
	Source    string               `json:"source,omitempty"`
	Cwd       string               `json:"cwd,omitempty"`
	TimeRange *timerange.TimeRange `json:"timeRange,omitempty"`
}

type ViewKind string

const (
	ViewTotals   ViewKind = "totals"
	ViewSessions ViewKind = "sessions"
	ViewRequests ViewKind = "requests"
	ViewGroups   ViewKind = "groups"
	ViewPeriod   ViewKind = "period"
	ViewMeta     ViewKind = "meta"
)

type View struct {
	Kind    ViewKind       `json:"kind"`
	By      domain.GroupBy `json:"by,omitempty"`
	Period  domain.Period  `json:"period,omitempty"`
	Page    int            `json:"page,omitempty"`
	Size    int            `json:"size,omitempty"`
	SortKey string         `json:"sortKey,omitempty"`
	SortDir string         `json:"sortDir,omitempty"` // "asc" | "desc"
}

type FileCacheEntry struct {
	MtimeMs int64
	Size    int64
	Data    *SessionFileData
}

type DirCache struct {
	ByFile map[string]FileCacheEntry
	Data   []*SessionFileData
}

type flightCall struct {
	wg  sync.WaitGroup
	val []*SessionFileData
	err error
}

type SessionData struct {
	mu       sync.RWMutex
	dirCache map[string]*DirCache
	flightMu sync.Mutex
	flights  map[string]*flightCall
}

func NewSessionData() *SessionData {
	return &SessionData{
		dirCache: make(map[string]*DirCache),
		flights:  make(map[string]*flightCall),
	}
}

var DefaultSessionData = NewSessionData()

func (s *SessionData) InvalidateCache(dir string) {
	s.mu.Lock()
	delete(s.dirCache, dir)
	s.mu.Unlock()
}

func (s *SessionData) FindSessionFile(dir, sessionId string) (string, error) {
	files, err := s.ReadSessionFilesCached(dir)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.SessionId == sessionId {
			return f.FilePath, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrSessionNotFound, sessionId)
}

func (s *SessionData) CollectJsonlFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func (s *SessionData) NormalizeCwd(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	abs = strings.TrimRight(abs, "/\\")
	if abs == "" {
		abs = "/"
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return strings.TrimRight(eval, "/\\")
	}
	return abs
}

func (s *SessionData) DisplayNameOf(fileName, firstUserText string) string {
	base := strings.TrimSuffix(fileName, ".jsonl")
	idx := strings.LastIndex(base, "_")
	prefix := fileName
	if idx > 0 && idx < len(base)-1 {
		prefix = base[:idx]
	}
	if isoDatePrefixRegex.MatchString(prefix) {
		if strings.TrimSpace(firstUserText) != "" {
			return strings.TrimSpace(firstUserText)
		}
		return prefix
	}
	return prefix
}

type jsonHeaderEntry struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession"`
}

func IsSessionFile(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	lineBytes, err := reader.ReadBytes('\n')
	if len(lineBytes) == 0 {
		return false
	}
	var h jsonHeaderEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(lineBytes))), &h); err != nil {
		return false
	}
	return h.Type == "session"
}

func ParseForkTs(file string) *int64 {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	lineBytes, err := reader.ReadBytes('\n')
	if len(lineBytes) == 0 {
		return nil
	}
	var h jsonHeaderEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(lineBytes))), &h); err != nil || h.Type != "session" {
		return nil
	}
	if h.ParentSession == "" || (!strings.Contains(h.ParentSession, "/") && !strings.Contains(h.ParentSession, "\\")) {
		return nil
	}
	ts, err := timerange.ParseUtcTimestamp(h.Timestamp)
	if err != nil {
		return nil
	}
	return &ts
}

type jsonMessageEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   *rawMessageBody `json:"message"`
}

type rawMessageBody struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Usage   *domain.Usage   `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnalyzeFile 解析单会话 JSONL 文件（内聚 ADR-0001 Fork 会话历史去重）
func (s *SessionData) AnalyzeFile(file string) (*SessionFileData, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 256*1024)
	firstLine := true

	var header *jsonHeaderEntry
	var items []MessageItem
	var firstUserText string
	var forkTs int64 = -1
	var parentSessionId string

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) > 0 {
			line := strings.TrimSpace(string(lineBytes))
			if firstLine {
				firstLine = false
				if line == "" {
					return nil, nil
				}
				var h jsonHeaderEntry
				if jsonErr := json.Unmarshal([]byte(line), &h); jsonErr != nil || h.Type != "session" {
					return nil, nil // 非合法 session 文件直接跳过（残留文件）
				}
				header = &h
				if h.ParentSession != "" {
					parentSessionId = h.ParentSession
					// 仅当 parentSession 为文件路径形态时触发 fork 去重（ADR-0001）
					if strings.Contains(h.ParentSession, "/") || strings.Contains(h.ParentSession, "\\") {
						ts, tsErr := timerange.ParseUtcTimestamp(h.Timestamp)
						if tsErr == nil {
							forkTs = ts
						}
					}
				}
				continue
			}

			if line == "" {
				continue
			}

			var me jsonMessageEntry
			if jsonErr := json.Unmarshal([]byte(line), &me); jsonErr == nil && me.Type == "message" && me.Message != nil {
				msg := me.Message

				// ADR-0001 Fork 去重：跳过 fork 创建之前的复制历史消息
				if forkTs >= 0 {
					itTs, itErr := timerange.ParseUtcTimestamp(me.Timestamp)
					if itErr == nil && itTs < forkTs {
						continue
					}
				}

				// 提取首条 user 文本作为候选显示名
				if firstUserText == "" && msg.Role == "user" && len(msg.Content) > 0 {
					var parts []contentPart
					if json.Unmarshal(msg.Content, &parts) == nil {
						for _, p := range parts {
							if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
								firstUserText = strings.TrimSpace(p.Text)
								break
							}
						}
					}
				}

				// 计入口径 A：仅 role=assistant 且 usage != nil
				if msg.Role == "assistant" && msg.Usage != nil {
					items = append(items, MessageItem{
						Timestamp: me.Timestamp,
						Model:     msg.Model,
						Usage:     *msg.Usage,
					})
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	if header == nil {
		return nil, nil
	}

	isTask := strings.Contains(file, "/tasks/") || strings.Contains(file, "\\tasks\\")
	return &SessionFileData{
		SessionId:       header.ID,
		Timestamp:       header.Timestamp,
		Cwd:             header.Cwd,
		FilePath:        file,
		FileName:        filepath.Base(file),
		IsTask:          isTask,
		ParentSessionId: parentSessionId,
		FirstUserText:   firstUserText,
		Items:           items,
	}, nil
}

// ReadSessionFilesCached 读取目录，带 singleflight 并发保护、mtime/size 快照缓存与并发解析
func (s *SessionData) ReadSessionFilesCached(dir string) ([]*SessionFileData, error) {
	s.flightMu.Lock()
	if c, ok := s.flights[dir]; ok {
		s.flightMu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(flightCall)
	c.wg.Add(1)
	s.flights[dir] = c
	s.flightMu.Unlock()

	defer func() {
		s.flightMu.Lock()
		delete(s.flights, dir)
		s.flightMu.Unlock()
		c.wg.Done()
	}()

	res, err := s.readSessionFilesCachedInternal(dir)
	c.val = res
	c.err = err
	return res, err
}

func (s *SessionData) readSessionFilesCachedInternal(dir string) ([]*SessionFileData, error) {
	files := s.CollectJsonlFiles(dir)

	s.mu.RLock()
	prev := s.dirCache[dir]
	s.mu.RUnlock()

	byFile := make(map[string]FileCacheEntry)
	var changed []string

	if prev != nil {
		for _, f := range files {
			st, err := os.Stat(f)
			if err != nil {
				continue
			}
			mtime := st.ModTime().UnixMilli()
			size := st.Size()
			if old, ok := prev.ByFile[f]; ok && old.MtimeMs == mtime && old.Size == size {
				byFile[f] = old
			} else {
				changed = append(changed, f)
			}
		}
	} else {
		changed = append(changed, files...)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}
	deletedCount := 0
	if prev != nil {
		for f := range prev.ByFile {
			if !fileSet[f] {
				deletedCount++
			}
		}
	}

	if len(changed) == 0 && deletedCount == 0 && prev != nil {
		return prev.Data, nil
	}

	// 并发解析变动文件 (Goroutine Pool)
	type parseResult struct {
		file  string
		entry FileCacheEntry
	}
	resChan := make(chan parseResult, len(changed))
	var wg sync.WaitGroup
	workers := 8
	if len(changed) < workers {
		workers = len(changed)
	}
	if workers == 0 {
		workers = 1
	}

	jobs := make(chan string, len(changed))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				st, err := os.Stat(f)
				if err != nil {
					continue
				}
				data, _ := s.AnalyzeFile(f)
				resChan <- parseResult{
					file: f,
					entry: FileCacheEntry{
						MtimeMs: st.ModTime().UnixMilli(),
						Size:    st.Size(),
						Data:    data,
					},
				}
			}
		}()
	}

	for _, f := range changed {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	close(resChan)

	nextByFile := make(map[string]FileCacheEntry)
	for f, entry := range byFile {
		nextByFile[f] = entry
	}
	for res := range resChan {
		nextByFile[res.file] = res.entry
	}

	var finalData []*SessionFileData
	for _, f := range files {
		if entry, ok := nextByFile[f]; ok && entry.Data != nil {
			finalData = append(finalData, entry.Data)
		}
	}

	s.mu.Lock()
	s.dirCache[dir] = &DirCache{
		ByFile: nextByFile,
		Data:   finalData,
	}
	s.mu.Unlock()

	return finalData, nil
}

func (s *SessionData) ApplyFilter(files []*SessionFileData, f Filter) []*SessionFileData {
	var normCwd *string
	if f.Cwd != "" {
		n := s.NormalizeCwd(f.Cwd)
		normCwd = &n
	}

	cwdCache := make(map[string]string)
	normOf := func(cwd string) string {
		if v, ok := cwdCache[cwd]; ok {
			return v
		}
		v := s.NormalizeCwd(cwd)
		cwdCache[cwd] = v
		return v
	}

	out := files
	if f.Source != "" && f.Source != "all" {
		var filtered []*SessionFileData
		for _, file := range out {
			source := file.Source
			if source == "" {
				source = "pi"
			}
			if source == f.Source {
				filtered = append(filtered, file)
			}
		}
		out = filtered
	}
	if normCwd != nil {
		var filtered []*SessionFileData
		for _, file := range out {
			if normOf(file.Cwd) == *normCwd {
				filtered = append(filtered, file)
			}
		}
		out = filtered
	}

	// 时间过滤
	if f.TimeRange != nil {
		tr := f.TimeRange
		if tr.Kind == timerange.KindSession {
			var filtered []*SessionFileData
			for _, file := range out {
				ts, err := timerange.ParseUtcTimestamp(file.Timestamp)
				if err != nil {
					filtered = append(filtered, file)
					continue
				}
				if tr.SinceMs != nil && ts < *tr.SinceMs {
					continue
				}
				if tr.UntilMs != nil && ts > *tr.UntilMs {
					continue
				}
				filtered = append(filtered, file)
			}
			out = filtered
		} else if tr.Kind == timerange.KindMessage {
			var filtered []*SessionFileData
			for _, file := range out {
				var keptItems []MessageItem
				for _, it := range file.Items {
					ts, err := timerange.ParseUtcTimestamp(it.Timestamp)
					if err != nil {
						keptItems = append(keptItems, it)
						continue
					}
					if tr.SinceMs != nil && ts < *tr.SinceMs {
						continue
					}
					if tr.UntilMs != nil && ts > *tr.UntilMs {
						continue
					}
					keptItems = append(keptItems, it)
				}
				if len(keptItems) > 0 {
					cpy := *file
					cpy.Items = keptItems
					filtered = append(filtered, &cpy)
				}
			}
			out = filtered
		}
	}

	// 模型过滤
	if f.Model != "" {
		var filtered []*SessionFileData
		for _, file := range out {
			var keptItems []MessageItem
			for _, it := range file.Items {
				if it.Model == f.Model {
					keptItems = append(keptItems, it)
				}
			}
			if len(keptItems) > 0 {
				cpy := *file
				cpy.Items = keptItems
				filtered = append(filtered, &cpy)
			}
		}
		out = filtered
	}

	return out
}

func (s *SessionData) TotalsFromFiles(files []*SessionFileData) domain.Totals {
	tot := domain.EmptyTotals()
	for _, file := range files {
		if file.Source == "codex" {
			tot.CostStatus = "unpriced"
		}
		for _, item := range file.Items {
			domain.AddUsage(&tot, item.Usage)
		}
	}
	domain.FinalizeTotals(&tot)
	return tot
}

func (s *SessionData) SessionRowsFromFiles(files []*SessionFileData) []domain.SessionRow {
	var rows []domain.SessionRow
	for _, file := range files {
		tot := domain.EmptyTotals()
		modelSet := make(map[string]bool)
		for _, item := range file.Items {
			domain.AddUsage(&tot, item.Usage)
			modelSet[item.Model] = true
		}
		domain.FinalizeTotals(&tot)

		modelStr := "-"
		if len(modelSet) == 1 {
			for m := range modelSet {
				modelStr = m
			}
		} else if len(modelSet) > 1 {
			modelStr = "mixed"
		}

		if file.Source == "codex" {
			tot.CostStatus = "unpriced"
		}
		rows = append(rows, domain.SessionRow{
			Totals:          tot,
			SessionId:       file.SessionId,
			Timestamp:       file.Timestamp,
			Cwd:             file.Cwd,
			Model:           modelStr,
			FileName:        file.FileName,
			DisplayName:     s.DisplayNameOf(file.FileName, file.FirstUserText),
			CwdNorm:         s.NormalizeCwd(file.Cwd),
			IsTask:          file.IsTask,
			ParentSessionId: file.ParentSessionId,
			Source:          file.Source,
		})
	}
	return rows
}

func (s *SessionData) RequestRowsFromFiles(files []*SessionFileData) []domain.RequestRow {
	var rows []domain.RequestRow
	nameBySession := make(map[string]string)
	for _, file := range files {
		nameBySession[file.SessionId] = s.DisplayNameOf(file.FileName, file.FirstUserText)
	}

	for _, file := range files {
		for _, item := range file.Items {
			tot := domain.EmptyTotals()
			domain.AddUsage(&tot, item.Usage)
			domain.FinalizeTotals(&tot)

			rows = append(rows, domain.RequestRow{
				Totals:      tot,
				SessionId:   file.SessionId,
				Timestamp:   item.Timestamp,
				Model:       item.Model,
				DisplayName: nameBySession[file.SessionId],
			})
		}
	}
	return rows
}

func (s *SessionData) GroupRowsFromFiles(files []*SessionFileData, by domain.GroupBy) []domain.GroupRow {
	byModel := by == domain.GroupByModel || by == domain.GroupByModelCwd
	byCwd := by == domain.GroupByCwd || by == domain.GroupByModelCwd

	cwdCache := make(map[string]string)
	normOf := func(cwd string) string {
		if v, ok := cwdCache[cwd]; ok {
			return v
		}
		v := s.NormalizeCwd(cwd)
		cwdCache[cwd] = v
		return v
	}

	groupMap := make(map[string]*domain.GroupRow)
	var keysOrder []string

	for _, file := range files {
		for _, item := range file.Items {
			var parts []string
			row := &domain.GroupRow{Totals: domain.EmptyTotals()}
			if byModel {
				row.Model = item.Model
				parts = append(parts, "m:"+item.Model)
			}
			if byCwd {
				norm := normOf(file.Cwd)
				row.Cwd = norm
				parts = append(parts, "c:"+norm)
			}
			k := strings.Join(parts, "|")
			g, ok := groupMap[k]
			if !ok {
				g = row
				groupMap[k] = g
				keysOrder = append(keysOrder, k)
			}
			if file.Source == "codex" {
				g.CostStatus = "unpriced"
			}
			domain.AddUsage(&g.Totals, item.Usage)
		}
	}

	var rows []domain.GroupRow
	for _, k := range keysOrder {
		g := groupMap[k]
		domain.FinalizeTotals(&g.Totals)
		rows = append(rows, *g)
	}
	return rows
}

func (s *SessionData) PeriodKey(timestamp string, p domain.Period) (string, error) {
	ts, err := timerange.ParseUtcTimestamp(timestamp)
	if err != nil {
		return "", err
	}
	t := time.UnixMilli(ts).UTC()
	y := t.Year()
	m := int(t.Month())
	d := t.Day()

	switch p {
	case domain.PeriodDay:
		return fmt.Sprintf("%04d-%02d-%02d", y, m, d), nil
	case domain.PeriodMonth:
		return fmt.Sprintf("%04d-%02d-01", y, m), nil
	case domain.PeriodWeek:
		// 对齐周一
		weekday := int(t.Weekday())
		dow := (weekday + 6) % 7 // Monday = 0, Sunday = 6
		monday := t.AddDate(0, 0, -dow)
		return fmt.Sprintf("%04d-%02d-%02d", monday.Year(), int(monday.Month()), monday.Day()), nil
	}
	return "", fmt.Errorf("unknown period: %s", p)
}

func (s *SessionData) PeriodRowsFromFiles(files []*SessionFileData, p domain.Period) []domain.PeriodRow {
	periodMap := make(map[string]*domain.PeriodRow)
	var keysOrder []string

	// period 按每条 usage event 的消息 timestamp 归属，而不是整个 rollout/file 的 header timestamp。
	// 跨天长 rollout 因此会按实际使用日拆分；header timestamp 只用于会话级归属。
	for _, file := range files {
		for _, item := range file.Items {
			k, err := s.PeriodKey(item.Timestamp, p)
			if err != nil {
				continue
			}
			g, ok := periodMap[k]
			if !ok {
				g = &domain.PeriodRow{
					Period: k,
					Totals: domain.EmptyTotals(),
				}
				periodMap[k] = g
				keysOrder = append(keysOrder, k)
			}
			if file.Source == "codex" {
				g.CostStatus = "unpriced"
			}
			domain.AddUsage(&g.Totals, item.Usage)
		}
	}

	sort.Strings(keysOrder)
	var rows []domain.PeriodRow
	for _, k := range keysOrder {
		g := periodMap[k]
		domain.FinalizeTotals(&g.Totals)
		rows = append(rows, *g)
	}
	return rows
}

type QueryResult struct {
	Window string         `json:"window"`
	Totals *domain.Totals `json:"totals,omitempty"`
	By     domain.GroupBy `json:"by,omitempty"`
	Period domain.Period  `json:"period,omitempty"`
	Rows   any            `json:"rows,omitempty"`
	Total  int            `json:"total,omitempty"`
	Page   int            `json:"page,omitempty"`
	Size   int            `json:"size,omitempty"`
	Meta   *QueryMeta     `json:"meta,omitempty"`
}

type QueryMeta struct {
	Dir          string         `json:"dir"`
	SessionCount int            `json:"sessionCount"`
	DataRange    DataRangeValue `json:"dataRange"`
	Sources      []string       `json:"sources,omitempty"`
	Warnings     []string       `json:"warnings,omitempty"`
	// UncountedSnapshots 是「只有累计快照、没有 durable usage record」的 Codex 快照条数：
	// 结构化暴露覆盖率缺口，供 UI/对账直接读取，而不必解析告警文案。
	UncountedSnapshots int `json:"uncountedSnapshots,omitempty"`
}

type DataRangeValue struct {
	Since *string `json:"since"`
	Until *string `json:"until"`
}

func (s *SessionData) Query(dir string, f Filter, v View) (*QueryResult, error) {
	files, err := s.ReadSessionFilesCached(dir)
	if err != nil {
		return nil, err
	}
	return s.QueryFiles(dir, files, f, v)
}

func (s *SessionData) QueryFiles(dir string, files []*SessionFileData, f Filter, v View) (*QueryResult, error) {
	if v.Kind == ViewMeta {
		var timestamps []string
		for _, file := range files {
			if _, err := timerange.ParseUtcTimestamp(file.Timestamp); err == nil {
				timestamps = append(timestamps, file.Timestamp)
			}
		}
		var minTs, maxTs *string
		if len(timestamps) > 0 {
			minVal := timestamps[0]
			maxVal := timestamps[0]
			for _, ts := range timestamps[1:] {
				if ts < minVal {
					minVal = ts
				}
				if ts > maxVal {
					maxVal = ts
				}
			}
			minTs = &minVal
			maxTs = &maxVal
		}
		meta := &QueryMeta{Dir: dir, SessionCount: len(files), DataRange: DataRangeValue{Since: minTs, Until: maxTs}, Sources: sourcesFromFiles(files)}
		return &QueryResult{Window: "meta", Meta: meta}, nil
	}

	filtered := s.ApplyFilter(files, f)
	switch v.Kind {
	case ViewTotals:
		tot := s.TotalsFromFiles(filtered)
		return &QueryResult{Window: "totals", Totals: &tot}, nil
	case ViewGroups:
		rows := s.GroupRowsFromFiles(filtered, v.By)
		return &QueryResult{Window: "totals", By: v.By, Rows: rows}, nil
	case ViewPeriod:
		rows := s.PeriodRowsFromFiles(filtered, v.Period)
		return &QueryResult{Window: "totals", Period: v.Period, Rows: rows}, nil
	case ViewSessions:
		rows := s.SessionRowsFromFiles(filtered)
		tot := s.TotalsFromFiles(filtered)
		total := len(rows)
		if v.SortKey != "" {
			sortSessions(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := rows
		if v.Page > 0 && v.Size > 0 {
			start := (v.Page - 1) * v.Size
			if start > total {
				start = total
			}
			end := start + v.Size
			if end > total {
				end = total
			}
			pageRows = rows[start:end]
		}
		return &QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}, nil
	case ViewRequests:
		rows := s.RequestRowsFromFiles(filtered)
		total := len(rows)
		if v.SortKey != "" {
			sortRequests(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := rows
		if v.Page > 0 && v.Size > 0 {
			start := (v.Page - 1) * v.Size
			if start > total {
				start = total
			}
			end := start + v.Size
			if end > total {
				end = total
			}
			pageRows = rows[start:end]
		}
		return &QueryResult{Window: "requests", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size}, nil
	}
	return nil, fmt.Errorf("unknown view kind: %s", v.Kind)
}

func sourcesFromFiles(files []*SessionFileData) []string {
	seen := map[string]bool{}
	for _, file := range files {
		source := file.Source
		if source == "" {
			continue
		}
		seen[source] = true
	}
	var sources []string
	for source := range seen {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func compareTotalsMetric(a, b domain.Totals, key string) (bool, bool) {
	switch key {
	case "requests":
		return a.Requests < b.Requests, true
	case "input":
		return a.Input < b.Input, true
	case "output":
		return a.Output < b.Output, true
	case "cache":
		return (a.CacheRead + a.CacheWrite) < (b.CacheRead + b.CacheWrite), true
	case "cacheRead":
		return a.CacheRead < b.CacheRead, true
	case "cacheWrite":
		return a.CacheWrite < b.CacheWrite, true
	case "reasoning":
		return a.Reasoning < b.Reasoning, true
	case "totalTokens":
		return a.TotalTokens < b.TotalTokens, true
	case "cost":
		return a.Cost < b.Cost, true
	case "cacheRate":
		return a.CacheRate < b.CacheRate, true
	default:
		return false, false
	}
}

func sortSessions(rows []domain.SessionRow, key string, desc bool) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]
		if less, ok := compareTotalsMetric(a.Totals, b.Totals, key); ok {
			if desc {
				return !less
			}
			return less
		}
		var less bool
		switch key {
		case "sessionId":
			less = a.SessionId < b.SessionId
		case "displayName":
			less = a.DisplayName < b.DisplayName
		case "cwd":
			less = a.Cwd < b.Cwd
		case "model":
			less = a.Model < b.Model
		case "timestamp":
			less = a.Timestamp < b.Timestamp
		default:
			less = a.Timestamp < b.Timestamp
		}
		if desc {
			return !less
		}
		return less
	})
}

func sortRequests(rows []domain.RequestRow, key string, desc bool) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]
		if less, ok := compareTotalsMetric(a.Totals, b.Totals, key); ok {
			if desc {
				return !less
			}
			return less
		}
		var less bool
		switch key {
		case "sessionId":
			less = a.SessionId < b.SessionId
		case "displayName":
			less = a.DisplayName < b.DisplayName
		case "model":
			less = a.Model < b.Model
		case "timestamp":
			less = a.Timestamp < b.Timestamp
		default:
			less = a.Timestamp < b.Timestamp
		}
		if desc {
			return !less
		}
		return less
	})
}

type SessionDetailTotals struct {
	Main          domain.Totals `json:"main"`
	Merged        domain.Totals `json:"merged"`
	ChildrenCount int           `json:"childrenCount"`
}

type SessionDetailMeta struct {
	HasChildren bool `json:"hasChildren"`
}

type SessionDetailResult struct {
	Session  domain.SessionRow   `json:"session"`
	Children []domain.SessionRow `json:"children"`
	Totals   SessionDetailTotals `json:"totals"`
	Requests []domain.RequestRow `json:"requests"`
	Meta     SessionDetailMeta   `json:"meta"`
}

var ErrSessionNotFound = fmt.Errorf("session not found")

func (s *SessionData) QueryDetail(dir, sessionId string) (*SessionDetailResult, error) {
	files, err := s.ReadSessionFilesCached(dir)
	if err != nil {
		return nil, err
	}

	var parent *SessionFileData
	var children []*SessionFileData

	for _, f := range files {
		if f.SessionId == sessionId {
			parent = f
		} else if f.ParentSessionId == sessionId {
			children = append(children, f)
		}
	}

	if parent == nil {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionId)
	}

	mainTotals := s.TotalsFromFiles([]*SessionFileData{parent})
	mergedTotals := s.TotalsFromFiles(append([]*SessionFileData{parent}, children...))

	parentRow := s.SessionRowsFromFiles([]*SessionFileData{parent})[0]
	childRows := s.SessionRowsFromFiles(children)

	nameMap := make(map[string]string)
	nameMap[parent.SessionId] = parentRow.DisplayName
	for _, ch := range childRows {
		nameMap[ch.SessionId] = ch.DisplayName
	}

	var requests []domain.RequestRow
	for _, it := range parent.Items {
		tot := domain.EmptyTotals()
		domain.AddUsage(&tot, it.Usage)
		domain.FinalizeTotals(&tot)
		requests = append(requests, domain.RequestRow{
			Totals:          tot,
			SessionId:       parent.SessionId,
			Timestamp:       it.Timestamp,
			Model:           it.Model,
			DisplayName:     nameMap[parent.SessionId],
			Source:          "main",
			SourceSessionId: parent.SessionId,
		})
	}

	for _, ch := range children {
		for _, it := range ch.Items {
			tot := domain.EmptyTotals()
			domain.AddUsage(&tot, it.Usage)
			domain.FinalizeTotals(&tot)
			requests = append(requests, domain.RequestRow{
				Totals:          tot,
				SessionId:       ch.SessionId,
				Timestamp:       it.Timestamp,
				Model:           it.Model,
				DisplayName:     nameMap[ch.SessionId],
				Source:          "child",
				SourceSessionId: ch.SessionId,
			})
		}
	}

	sort.Slice(requests, func(i, j int) bool {
		return requests[i].Timestamp < requests[j].Timestamp
	})

	return &SessionDetailResult{
		Session:  parentRow,
		Children: childRows,
		Totals: SessionDetailTotals{
			Main:          mainTotals,
			Merged:        mergedTotals,
			ChildrenCount: len(children),
		},
		Requests: requests,
		Meta: SessionDetailMeta{
			HasChildren: len(children) > 0,
		},
	}, nil
}
