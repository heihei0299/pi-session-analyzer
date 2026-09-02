//go:build windows

package watch

import "os"

// Windows 下无 POSIX Inode，基于 0 返回（依赖 size 与 mtime 共同判断文件重写）
func getInode(fi os.FileInfo) uint64 {
	return 0
}
