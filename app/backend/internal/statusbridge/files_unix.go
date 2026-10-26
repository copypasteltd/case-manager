//go:build unix

package statusbridge

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func lockFile(f *os.File) (func() error, error) {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, err
	}
	return func() error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}

func replaceFile(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return syncStateDirectory(filepath.Dir(to))
}

func syncStateDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
