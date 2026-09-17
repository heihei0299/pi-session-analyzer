package codex

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"strings"
)

type fileRevision struct {
	modifiedMs    int64
	logicalSize   int64
	completeBytes int64
	completeLines int64
	tail          int64
}

func rolloutRevision(path string) (fileRevision, error) {
	st, err := os.Stat(path)
	if err != nil {
		return fileRevision{}, err
	}
	data, err := readRolloutBytes(path)
	if err != nil {
		return fileRevision{}, err
	}
	completeBytes := int64(len(data))
	completeLines := int64(0)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		idx := strings.LastIndexByte(string(data), '\n')
		if idx < 0 {
			completeBytes = 0
		} else {
			completeBytes = int64(idx + 1)
		}
	}
	if completeBytes > 0 {
		completeLines = int64(strings.Count(string(data[:completeBytes]), "\n"))
	}
	return fileRevision{
		modifiedMs:    st.ModTime().UnixMilli(),
		logicalSize:   int64(len(data)),
		completeBytes: completeBytes,
		completeLines: completeLines,
		tail:          fingerprint(data),
	}, nil
}

func readRolloutBytes(path string) ([]byte, error) {
	reader, closeReader, err := openRollout(path)
	if err != nil {
		return nil, err
	}
	defer closeReader()
	return io.ReadAll(reader)
}

func fingerprint(data []byte) int64 {
	if len(data) > 4096 {
		data = data[len(data)-4096:]
	}
	sum := sha256.Sum256(data)
	var result int64
	for _, b := range sum[:8] {
		result = (result << 8) | int64(b)
	}
	return result
}

// cursorState 判定文件 revision 是否与游标一致；一致时一并返回该 revision 的 per-file 诊断摘要。
func cursorState(database *sql.DB, path string, rev fileRevision) (bool, Diagnostics, error) {
	var modified, offset, tail sql.NullInt64
	var summary sql.NullString
	err := database.QueryRow(`SELECT last_modified, last_byte_offset, last_tail_fingerprint, diagnostics_summary FROM session_log_sync WHERE file_path = ?`, path).Scan(&modified, &offset, &tail, &summary)
	if err == sql.ErrNoRows {
		return false, Diagnostics{}, nil
	}
	if err != nil {
		return false, Diagnostics{}, err
	}
	matched := modified.Valid && offset.Valid && tail.Valid && modified.Int64 == rev.modifiedMs && offset.Int64 == rev.completeBytes && tail.Int64 == rev.tail && rev.logicalSize == rev.completeBytes
	if !matched {
		return false, Diagnostics{}, nil
	}
	stored := Diagnostics{}
	if summary.Valid && strings.TrimSpace(summary.String) != "" {
		// 摘要是诊断通道，损坏时降级为「无诊断」，不能因此中断同步。
		_ = json.Unmarshal([]byte(summary.String), &stored)
	}
	return true, stored, nil
}
