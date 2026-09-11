package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedWebUIIsSynchronizedWithCanonicalSource(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "src", "webui.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, webUIContent) {
		t.Fatal("internal/server/webui.html is stale; run 'make sync-webui'")
	}
}
