package utils

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

// NewID generates a short random hex ID, used for task IDs and rclone
// remote name suffixes.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var unsafeFilenameChars = regexp.MustCompile(`[/\\:*?"<>|\x00]`)

// SafeFilename strips characters that are unsafe for both the local
// filesystem and Drive uploads.
func SafeFilename(name string) string {
	cleaned := unsafeFilenameChars.ReplaceAllString(name, "_")
	if cleaned == "" {
		return "file_" + NewID()
	}
	return cleaned
}

// EnsureDir creates dir (and parents) if it doesn't already exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// DiskUsage reports used/total bytes for the filesystem containing path.
// Used by /disk and the automatic cleanup threshold check.
func DiskUsage(path string) (total, used, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bfree * uint64(stat.Bsize)
	used = total - free
	return total, used, free, nil
}

// DirSize walks dir and sums file sizes, used by /clean to report how much
// was freed.
func DirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
