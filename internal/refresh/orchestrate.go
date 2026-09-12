package refresh

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
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
	return pi.Fingerprint(resolved.Root, resolved.Layout)
}

// CodexFingerprint 只覆盖 Codex home。
func CodexFingerprint(codexDir string) (string, error) {
	return codex.CheapFingerprint(codex.ResolveHome(codexDir))
}

// Fingerprint 给定 Pi/Codex 根的 source revision 哈希：包含 adapter 的文件内容尾指纹、
// 完整性、大小与 mtime。Watch 与 server watcher 只凭它判断“变了”，解析规则仍只在 adapter 内。
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
