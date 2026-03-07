//go:build windows

package metrics

import "os"

func fileInode(info os.FileInfo) uint64 {
	// Windows doesn't have inodes, use modification time as a proxy
	return uint64(info.ModTime().UnixNano())
}
