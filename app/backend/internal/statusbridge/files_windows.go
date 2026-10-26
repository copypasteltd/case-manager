package statusbridge

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockFile(f *os.File) (func() error, error) {
	o := &windows.Overlapped{}
	h := windows.Handle(f.Fd())
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, o); err != nil {
		return nil, err
	}
	return func() error { return windows.UnlockFileEx(h, 0, 1, 0, o) }, nil
}

func replaceFile(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(f, t, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// NTFS file flush persists a newly created journal; replacement additionally
// uses MOVEFILE_WRITE_THROUGH. Directory handles cannot use ordinary File.Sync.
func syncStateDirectory(string) error { return nil }
