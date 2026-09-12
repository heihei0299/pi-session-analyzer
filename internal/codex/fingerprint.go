package codex

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Fingerprint returns the revision of discovered Codex rollouts. It shares the
// rollout revision used by incremental sync instead of relying on size/mtime
// alone; discovery diagnostics are included so newly visible artifacts refresh.
func Fingerprint(home string) (string, error) {
	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte("codex-source-revision-v1"))
	h.Write([]byte{0})
	h.Write([]byte(home))
	h.Write([]byte{0})

	var number [8]byte
	for _, file := range files {
		revision, err := rolloutRevision(file.Path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(file.Path))
		h.Write([]byte{0})
		binary.BigEndian.PutUint64(number[:], uint64(revision.logicalSize))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.modifiedMs))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.tail))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.completeBytes))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.completeLines))
		h.Write(number[:])
	}
	for _, warning := range diagnostics.Warnings {
		h.Write([]byte(warning))
		h.Write([]byte{0})
	}
	binary.BigEndian.PutUint64(number[:], uint64(diagnostics.Skipped))
	h.Write(number[:])
	return hex.EncodeToString(h.Sum(nil)), nil
}
