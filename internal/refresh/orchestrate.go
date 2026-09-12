package refresh

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/pi"
)

var lastMu sync.RWMutex
var lastResult Result

// Result 记录最近一次 Refresh 的结果，供 server 通过 meta 对外暴露。
// 失败不清除旧 ledger：sync 按文件事务提交，失败只影响本次增量。
type Result struct {
	At  time.Time
	Err error
}

// LastResult 返回最近一次 Refresh 的结果（零值表示尚未执行过）。
func LastResult() Result {
	lastMu.RLock()
	defer lastMu.RUnlock()
	return lastResult
}

func recordResult(err error) {
	lastMu.Lock()
	lastResult = Result{At: time.Now(), Err: err}
	lastMu.Unlock()
}

// PiFingerprint 只覆盖 Pi 会话根（CLI --watch 限定 pi 源时用，避免
// Codex 目录变化引发无意义的 Pi 刷新输出）。
func PiFingerprint(piDir string) (string, error) {
	resolved, err := pi.ResolvePiSessionRoot(
		os.Getenv("PI_CODING_AGENT_SESSION_DIR"),
		piDir,
		pi.GetPiNativeSessionDir(),
	)
	if err != nil {
		return "", err
	}
	return fingerprintRoots(resolved.Root)
}

// CodexFingerprint 只覆盖 Codex home。
func CodexFingerprint(codexDir string) (string, error) {
	return fingerprintRoots(codex.ResolveHome(codexDir))
}

// Fingerprint 给定 Pi/Codex 根的可变指纹：排序后的（路径、大小、mtime）哈希。
// Watch 与 server watcher 只凭它判断“变了”，解析规则仍只在 adapter 内。
func Fingerprint(piDir, codexDir string) (string, error) {
	piFp, err := PiFingerprint(piDir)
	if err != nil {
		return "", err
	}
	codexFp, err := CodexFingerprint(codexDir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(piFp))
	h.Write([]byte{0})
	h.Write([]byte(codexFp))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fingerprintRoots(roots ...string) (string, error) {
	h := sha256.New()
	feed := func(root string) {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			fi, err := entry.Info()
			if err != nil {
				return nil
			}
			var buf [8]byte
			binary.BigEndian.PutUint64(buf[:], uint64(fi.Size()))
			h.Write([]byte(path))
			h.Write([]byte{0})
			h.Write(buf[:])
			binary.BigEndian.PutUint64(buf[:], uint64(fi.ModTime().UnixMilli()))
			h.Write(buf[:])
			h.Write([]byte{0})
			return nil
		})
	}
	sort.Strings(roots)
	for _, root := range roots {
		h.Write([]byte(root))
		h.Write([]byte{0})
		feed(root)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
