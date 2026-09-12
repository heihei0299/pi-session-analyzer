package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedWebUIMatchesSingleSource(t *testing.T) {
	// WebUI 唯一人工维护源码：internal/server/webui.html，Go embed 直引。
	// 此测试只防 embed 陈旧（改完未重新 build），不再有第二份 copy/sync。
	single, err := os.ReadFile(filepath.Join("webui.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(single, webUIContent) {
		t.Fatal("embedded WebUI is stale; rebuild the Go binary")
	}
}
