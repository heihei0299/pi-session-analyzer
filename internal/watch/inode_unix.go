//go:build !windows

package watch

import (
	"os"
	"syscall"
)

func getInode(fi os.FileInfo) uint64 {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
