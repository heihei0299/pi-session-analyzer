package pi

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
)

type PiFileRevision struct {
	FileSize        int64
	TailFingerprint uint32
	Complete        bool
	ModifiedMs      int64
}

func tailFingerprintGo(buf []byte) uint32 {
	h := sha256.New()
	label := []byte("pi-session-tail-v1")
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(label)))
	h.Write(lenBuf[:])
	h.Write(label)
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(buf)))
	h.Write(lenBuf[:])
	h.Write(buf)
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

func PiFileRevisionOf(path string) (PiFileRevision, error) {
	st, err := os.Stat(path)
	if err != nil {
		return PiFileRevision{}, err
	}
	fileSize := st.Size()
	modifiedMs := st.ModTime().UnixMilli()
	tailLen := fileSize
	if tailLen > 4096 {
		tailLen = 4096
	}
	var tail []byte
	complete := true
	if tailLen > 0 {
		f, err := os.Open(path)
		if err != nil {
			return PiFileRevision{}, err
		}
		defer f.Close()
		buf := make([]byte, tailLen)
		_, err = f.ReadAt(buf, fileSize-tailLen)
		if err != nil {
			return PiFileRevision{}, err
		}
		tail = buf
		complete = buf[len(buf)-1] == '\n'
	}
	return PiFileRevision{
		FileSize:        fileSize,
		TailFingerprint: tailFingerprintGo(tail),
		Complete:        complete,
		ModifiedMs:      modifiedMs,
	}, nil
}

func tailFingerprintAtGo(path string, offset int64) (uint32, error) {
	l := offset
	if l > 4096 {
		l = 4096
	}
	if l <= 0 {
		return tailFingerprintGo([]byte{}), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, l)
	_, err = f.ReadAt(buf, offset-l)
	if err != nil {
		return 0, err
	}
	return tailFingerprintGo(buf), nil
}
