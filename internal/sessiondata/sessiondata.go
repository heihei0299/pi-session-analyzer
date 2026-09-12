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

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/timerange"
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

type QueryResult struct {
	Window string         `json:"window"`
	Totals *domain.Totals `json:"totals,omitempty"`
	By     domain.GroupBy `json:"by,omitempty"`
	Period domain.Period  `json:"period,omitempty"`
	Rows   any            `json:"rows"`
	Total  int            `json:"total"`
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

// SortSessionRows 复用生产排序语义，供 ledger-backed 查询在内存行上排序。
func SortSessionRows(rows []domain.SessionRow, key string, desc bool) {
	sortSessions(rows, key, desc)
}

// SortRequestRows 复用生产排序语义，供 ledger-backed 查询在内存行上排序。
func SortRequestRows(rows []domain.RequestRow, key string, desc bool) {
	sortRequests(rows, key, desc)
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

