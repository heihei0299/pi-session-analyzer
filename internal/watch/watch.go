package watch

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
	"github.com/heihei0299/pi-session-anylize/internal/timerange"
)

type FileState struct {
	Offset  int64
	Inode   uint64
	MtimeMs int64
	ForkTs  *int64
}

type Increment struct {
	domain.Totals
	File string `json:"file"`
}

type IncrementalReader struct {
	dir     string
	sd      *sessiondata.SessionData
	states  map[string]FileState
	contrib map[string]domain.Totals
}

func NewIncrementalReader(dir string, sd ...*sessiondata.SessionData) *IncrementalReader {
	var s *sessiondata.SessionData
	if len(sd) > 0 && sd[0] != nil {
		s = sd[0]
	} else {
		s = sessiondata.DefaultSessionData
	}
	return &IncrementalReader{
		dir:     dir,
		sd:      s,
		states:  make(map[string]FileState),
		contrib: make(map[string]domain.Totals),
	}
}

func (r *IncrementalReader) ReadIncrements() ([]Increment, error) {
	var increments []Increment
	allFiles := r.sd.CollectJsonlFiles(r.dir)

	// 1. 合法会话文件收集：已跟踪且 inode 未变的文件跳过首行重验（大幅降低高频轮询开销）
	var validFiles []string
	for _, f := range allFiles {
		known, exists := r.states[f]
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		ino := getInode(fi)
		if !exists {
			if sessiondata.IsSessionFile(f) {
				validFiles = append(validFiles, f)
			}
		} else {
			if ino == known.Inode {
				validFiles = append(validFiles, f)
			} else if sessiondata.IsSessionFile(f) {
				validFiles = append(validFiles, f)
			}
		}
	}

	seen := make(map[string]bool)
	for _, f := range validFiles {
		seen[f] = true
	}

	// 2. 处理已跟踪文件
	for file, state := range r.states {
		if !seen[file] {
			delete(r.states, file)
			delete(r.contrib, file)
			continue
		}

		fi, err := os.Stat(file)
		if err != nil {
			continue
		}
		ino := getInode(fi)
		mtime := fi.ModTime().UnixMilli()
		size := fi.Size()

		// 替换、截断或重写
		if ino != state.Inode || size < state.Offset || (size == state.Offset && mtime != state.MtimeMs) {
			// 扣减历史贡献
			if prev, ok := r.contrib[file]; ok {
				increments = append(increments, negate(prev, file))
			}
			// 从头全量重读，复用初始 forkTs
			added, _, err := readEntriesFrom(file, 0, &increments, state.ForkTs)
			if err == nil {
				r.states[file] = FileState{
					Offset:  size,
					Inode:   ino,
					MtimeMs: mtime,
					ForkTs:  state.ForkTs,
				}
				r.contrib[file] = added
			}
		} else if size > state.Offset {
			// 正常追加：从上次 offset 续读
			added, bytesRead, err := readEntriesFrom(file, state.Offset, &increments, state.ForkTs)
			if err == nil {
				prev := r.contrib[file]
				r.contrib[file] = mergeTotals(prev, added)
				r.states[file] = FileState{
					Offset:  state.Offset + bytesRead,
					Inode:   ino,
					MtimeMs: mtime,
					ForkTs:  state.ForkTs,
				}
			}
		}
	}

	// 3. 处理新加入的文件
	for _, file := range validFiles {
		if _, ok := r.states[file]; ok {
			continue
		}
		fi, err := os.Stat(file)
		if err != nil {
			continue
		}
		ino := getInode(fi)
		mtime := fi.ModTime().UnixMilli()

		// 解析首行获取 forkTs（复用 sessiondata 统一权威口径）
		forkTs := sessiondata.ParseForkTs(file)
		added, bytesRead, err := readEntriesFrom(file, 0, &increments, forkTs)
		if err == nil {
			r.states[file] = FileState{
				Offset:  bytesRead,
				Inode:   ino,
				MtimeMs: mtime,
				ForkTs:  forkTs,
			}
			r.contrib[file] = added
		}
	}

	return increments, nil
}

func readEntriesFrom(file string, offset int64, increments *[]Increment, forkTs *int64) (domain.Totals, int64, error) {
	tot := domain.EmptyTotals()
	f, err := os.Open(file)
	if err != nil {
		return tot, 0, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return tot, 0, err
		}
	}

	reader := bufio.NewReader(f)
	var bytesConsumed int64

	for {
		lineBytes, err := reader.ReadBytes('\n')
		n := int64(len(lineBytes))
		if n == 0 {
			break
		}

		// 保证只处理包含换行符的完整行；若文件末尾是不完整写入的半行，不计入 offset 并在下次续读
		if lineBytes[len(lineBytes)-1] != '\n' {
			break
		}
		bytesConsumed += n

		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			continue
		}

		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   *struct {
				Role  string        `json:"role"`
				Model string        `json:"model"`
				Usage *domain.Usage `json:"usage"`
			} `json:"message"`
		}

		if jsonErr := json.Unmarshal([]byte(line), &entry); jsonErr == nil && entry.Type == "message" && entry.Message != nil {
			msg := entry.Message
			if forkTs != nil {
				ts, err := timerange.ParseUtcTimestamp(entry.Timestamp)
				if err == nil && ts < *forkTs {
					continue
				}
			}
			if msg.Role == "assistant" && msg.Usage != nil {
				domain.AddUsage(&tot, *msg.Usage)
				single := domain.EmptyTotals()
				domain.AddUsage(&single, *msg.Usage)
				domain.FinalizeTotals(&single)
				*increments = append(*increments, Increment{
					Totals: single,
					File:   file,
				})
			}
		}

		if err != nil {
			break
		}
	}

	domain.FinalizeTotals(&tot)
	return tot, bytesConsumed, nil
}

func negate(t domain.Totals, file string) Increment {
	return Increment{
		Totals: domain.Totals{
			Requests:    -t.Requests,
			Input:       -t.Input,
			Output:      -t.Output,
			CacheRead:   -t.CacheRead,
			CacheWrite:  -t.CacheWrite,
			Reasoning:   -t.Reasoning,
			TotalTokens: -t.TotalTokens,
			Cost:        -t.Cost,
		},
		File: file,
	}
}

func mergeTotals(a, b domain.Totals) domain.Totals {
	res := domain.Totals{
		Requests:   a.Requests + b.Requests,
		Input:      a.Input + b.Input,
		Output:     a.Output + b.Output,
		CacheRead:  a.CacheRead + b.CacheRead,
		CacheWrite: a.CacheWrite + b.CacheWrite,
		Reasoning:  a.Reasoning + b.Reasoning,
		Cost:       a.Cost + b.Cost,
	}
	domain.FinalizeTotals(&res)
	return res
}

func ApplyIncrements(totals *domain.Totals, incs []Increment) {
	for _, inc := range incs {
		totals.Requests += inc.Requests
		totals.Input += inc.Input
		totals.Output += inc.Output
		totals.CacheRead += inc.CacheRead
		totals.CacheWrite += inc.CacheWrite
		totals.Reasoning += inc.Reasoning
		totals.Cost += inc.Cost
	}
	domain.FinalizeTotals(totals)
}
