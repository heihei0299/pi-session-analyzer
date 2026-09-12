package pi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrSessionNotFound = errors.New("Pi session not found")

// FindSessionFileByHeaderID locates a Pi session without parsing its JSONL body.
// Discovery follows the resolved layout and only the first line is read.
func FindSessionFileByHeaderID(root string, layout Layout, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("%w: empty session id", ErrSessionNotFound)
	}
	for _, path := range CollectPiJsonlFiles(root, layout) {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		line, readErr := bufio.NewReader(file).ReadString('\n')
		closeErr := file.Close()
		if readErr != nil && readErr != io.EOF {
			continue
		}
		if closeErr != nil || strings.TrimSpace(line) == "" {
			continue
		}
		var header struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			continue
		}
		if header.Type == "session" && header.ID == sessionID {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
}
