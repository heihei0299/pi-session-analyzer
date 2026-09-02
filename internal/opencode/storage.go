package opencode

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type HistoryFile struct {
	Records        []UsageRecord `json:"records"`
	LastSyncedTime *string       `json:"lastSyncedTime"`
	UpdatedAt      string        `json:"updatedAt"`
}

type Storage struct {
	dataDir string
}

func NewStorage(dataDir string) *Storage {
	if dataDir == "" {
		dataDir = "data/opencode"
	}
	return &Storage{dataDir: dataDir}
}

func (s *Storage) EnsureDataDir() error {
	return os.MkdirAll(s.dataDir, 0755)
}

func (s *Storage) costsPath() string {
	return filepath.Join(s.dataDir, "costs.json")
}

func (s *Storage) historyPath() string {
	return filepath.Join(s.dataDir, "history.json")
}

func (s *Storage) csvPath() string {
	return filepath.Join(s.dataDir, "history.csv")
}

func (s *Storage) lockPath() string {
	return filepath.Join(s.dataDir, ".lock")
}

// Lock 获取跨进程 POSIX flock 独占锁，返回 unlock 回调函数
func (s *Storage) Lock() (func(), error) {
	if err := s.EnsureDataDir(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	// 非阻塞排他锁：如果有另一个进程在执行同步，立即返回冲突
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("同步进行中，请稍后再试 (lock busy)")
	}

	unlock := func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	return unlock, nil
}

func (s *Storage) GetCosts(year, month int) (*CostsResult, error) {
	data, err := os.ReadFile(s.costsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var fileMap map[string]CostsResult
	if err := json.Unmarshal(data, &fileMap); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%04d-%02d", year, month)
	if res, ok := fileMap[key]; ok {
		return &res, nil
	}
	return nil, nil
}

func (s *Storage) SaveCosts(year, month int, res CostsResult) error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	fileMap := make(map[string]CostsResult)
	data, err := os.ReadFile(s.costsPath())
	if err == nil {
		_ = json.Unmarshal(data, &fileMap)
	}
	key := fmt.Sprintf("%04d-%02d", year, month)
	fileMap[key] = res

	bytes, err := json.MarshalIndent(fileMap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.costsPath(), bytes, 0644)
}

func (s *Storage) LoadHistory() (*HistoryFile, error) {
	data, err := os.ReadFile(s.historyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &HistoryFile{Records: []UsageRecord{}}, nil
		}
		return nil, err
	}
	var hf HistoryFile
	if err := json.Unmarshal(data, &hf); err != nil {
		return nil, err
	}
	return &hf, nil
}

func (s *Storage) SaveHistory(records []UsageRecord, lastSyncedTime *string) error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}

	// 降序排序按 timeCreated
	sort.Slice(records, func(i, j int) bool {
		return records[i].TimeCreated > records[j].TimeCreated
	})

	hf := HistoryFile{
		Records:        records,
		LastSyncedTime: lastSyncedTime,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	bytes, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.historyPath(), bytes, 0644); err != nil {
		return err
	}
	return s.ExportCSV(records)
}

func (s *Storage) ExportCSV(records []UsageRecord) error {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	headers := []string{
		"id", "workspaceID", "timeCreated", "timeUpdated", "timeDeleted",
		"model", "provider", "inputTokens", "outputTokens", "reasoningTokens",
		"cacheReadTokens", "cacheWrite5mTokens", "cacheWrite1hTokens", "cost",
		"keyID", "sessionID",
	}
	if err := w.Write(headers); err != nil {
		return err
	}

	for _, r := range records {
		var delStr, sessStr string
		if r.TimeDeleted != nil {
			delStr = *r.TimeDeleted
		}
		if r.SessionID != nil {
			sessStr = *r.SessionID
		}
		line := []string{
			r.ID,
			r.WorkspaceID,
			r.TimeCreated,
			r.TimeUpdated,
			delStr,
			r.Model,
			r.Provider,
			fmt.Sprintf("%.0f", r.InputTokens),
			fmt.Sprintf("%.0f", r.OutputTokens),
			fmt.Sprintf("%.0f", r.ReasoningTokens),
			fmt.Sprintf("%.0f", r.CacheReadTokens),
			fmt.Sprintf("%.0f", r.CacheWrite5mTokens),
			fmt.Sprintf("%.0f", r.CacheWrite1hTokens),
			fmt.Sprintf("%.6f", r.Cost),
			r.KeyID,
			sessStr,
		}
		if err := w.Write(line); err != nil {
			return err
		}
	}
	w.Flush()
	return os.WriteFile(s.csvPath(), buf.Bytes(), 0644)
}

func (s *Storage) GetHistory(filter HistoryFilter, page, size int) ([]UsageRecord, int, error) {
	hf, err := s.LoadHistory()
	if err != nil {
		return nil, 0, err
	}

	var filtered []UsageRecord
	for _, r := range hf.Records {
		if filter.Model != "" && r.Model != filter.Model {
			continue
		}
		if filter.SessionID != "" {
			if r.SessionID == nil || *r.SessionID != filter.SessionID {
				continue
			}
		}
		if filter.Since != "" && r.TimeCreated < filter.Since {
			continue
		}
		if filter.Until != "" && r.TimeCreated > filter.Until {
			continue
		}
		filtered = append(filtered, r)
	}

	total := len(filtered)
	if page > 0 && size > 0 {
		start := (page - 1) * size
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}
		return filtered[start:end], total, nil
	}
	return filtered, total, nil
}

func CsvEscape(v string) string {
	if strings.Contains(v, "\"") || strings.Contains(v, ",") || strings.Contains(v, "\n") {
		return "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
	}
	return v
}

func ToFloatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
