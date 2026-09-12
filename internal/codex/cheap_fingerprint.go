package codex

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"sort"
)

const cheapFingerprintTailBytes = 4096

// CheapFingerprint inspects discovered rollout metadata and a small physical
// tail without decoding rollout contents. Full revision/parse work remains in
// SyncRollouts after this fingerprint reports a change.
func CheapFingerprint(home string) (string, error) {
	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte("codex-cheap-source-revision-v1"))
	h.Write([]byte{0})
	h.Write([]byte(home))
	h.Write([]byte{0})
	var number [8]byte
	for _, file := range files {
		stat, err := os.Stat(file.Path)
		if err != nil {
			return "", err
		}
		tail, err := readPhysicalTail(file.Path, stat.Size())
		if err != nil {
			return "", err
		}
		tailHash := sha256.Sum256(tail)
		h.Write([]byte(file.Path))
		h.Write([]byte{0})
		binary.BigEndian.PutUint64(number[:], uint64(stat.Size()))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(stat.ModTime().UnixMilli()))
		h.Write(number[:])
		h.Write(tailHash[:])
	}
	warnings := append([]string(nil), diagnostics.Warnings...)
	sort.Strings(warnings)
	for _, warning := range warnings {
		h.Write([]byte(warning))
		h.Write([]byte{0})
	}
	binary.BigEndian.PutUint64(number[:], uint64(diagnostics.Skipped))
	h.Write(number[:])
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readPhysicalTail(path string, size int64) ([]byte, error) {
	if size <= 0 {
		return nil, nil
	}
	length := size
	if length > cheapFingerprintTailBytes {
		length = cheapFingerprintTailBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buffer := make([]byte, length)
	n, err := file.ReadAt(buffer, size-length)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buffer[:n], nil
}
