package pi

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

// Fingerprint returns the revision of the Pi files selected by layout. It uses
// the same tail/size/completeness revision as incremental sync, so a same-size
// rewrite is visible to watchers.
func Fingerprint(root string, layout Layout) (string, error) {
	h := sha256.New()
	h.Write([]byte("pi-source-revision-v1"))
	h.Write([]byte{0})
	h.Write([]byte(root))
	h.Write([]byte{0})
	h.Write([]byte(layout))
	h.Write([]byte{0})

	var number [8]byte
	files := CollectPiJsonlFiles(root, layout)
	sort.Strings(files)
	for _, path := range files {
		revision, err := PiFileRevisionOf(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(path))
		h.Write([]byte{0})
		binary.BigEndian.PutUint64(number[:], uint64(revision.FileSize))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.ModifiedMs))
		h.Write(number[:])
		binary.BigEndian.PutUint64(number[:], uint64(revision.TailFingerprint))
		h.Write(number[:])
		if revision.Complete {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
