package codex

import (
	"database/sql"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

func LoadSessionFiles(database *db.Database, physicalIDs map[string]bool) ([]*sessiondata.SessionFileData, error) {
	metas, err := loadSourceSessions(database.DB)
	if err != nil {
		return nil, err
	}
	rows, err := database.DB.Query(`SELECT model, session_id, cwd, timestamp_text, physical_rollout_id, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens FROM proxy_request_logs WHERE data_source = 'codex' ORDER BY created_at, request_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make(map[string]*sessiondata.SessionFileData)
	order := make([]string, 0)
	for rows.Next() {
		var model, sessionID, cwd, timestamp, physicalID string
		var input, output, cacheRead, cacheWrite, reasoning int64
		if err := rows.Scan(&model, &sessionID, &cwd, &timestamp, &physicalID, &input, &output, &cacheRead, &cacheWrite, &reasoning); err != nil {
			return nil, err
		}
		key := physicalID
		if key == "" {
			key = sessionID
		}
		if !physicalIDs[key] {
			continue
		}
		file, ok := files[key]
		if !ok {
			meta := metas[key]
			id := meta.threadID
			if id == "" {
				id = sessionID
			}
			fileTimestamp := meta.timestamp
			if fileTimestamp == "" {
				fileTimestamp = timestamp
			}
			fileCwd := meta.cwd
			if fileCwd == "" {
				fileCwd = cwd
			}
			file = &sessiondata.SessionFileData{
				SessionId:       id,
				Timestamp:       fileTimestamp,
				Cwd:             fileCwd,
				FilePath:        meta.filePath,
				FileName:        meta.fileName,
				ParentSessionId: meta.parentThreadID,
				Source:          "codex",
			}
			files[key] = file
			order = append(order, key)
		}
		file.Items = append(file.Items, sessiondata.MessageItem{
			Timestamp: timestamp,
			Model:     model,
			Usage: domain.Usage{
				Input:       float64(input),
				Output:      float64(output),
				CacheRead:   float64(cacheRead),
				CacheWrite:  float64(cacheWrite),
				Reasoning:   float64(reasoning),
				TotalTokens: float64(input + cacheRead + output),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*sessiondata.SessionFileData, 0, len(order))
	for _, key := range order {
		out = append(out, files[key])
	}
	return out, nil
}

type sourceSession struct {
	physicalID     string
	sessionID      string
	threadID       string
	timestamp      string
	cwd            string
	filePath       string
	fileName       string
	parentThreadID string
}

func loadSourceSessions(database *sql.DB) (map[string]sourceSession, error) {
	rows, err := database.Query(`SELECT physical_id, session_id, thread_id, timestamp_text, cwd, file_path, file_name, parent_thread_id FROM source_sessions WHERE data_source = 'codex'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]sourceSession)
	for rows.Next() {
		var value sourceSession
		if err := rows.Scan(&value.physicalID, &value.sessionID, &value.threadID, &value.timestamp, &value.cwd, &value.filePath, &value.fileName, &value.parentThreadID); err != nil {
			return nil, err
		}
		out[value.physicalID] = value
	}
	return out, rows.Err()
}
