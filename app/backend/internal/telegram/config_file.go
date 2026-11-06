package telegram

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// The file source is for native service credentials. Errors never disclose a
// filename, JSON fragment, token or OS diagnostic. Existing inline consumers
// remain supported outside the fixed production release catalog.
func botConfiguration(key string) (string, error) {
	raw, inline := os.LookupEnv(key)
	path, file := os.LookupEnv(key + "_FILE")
	failure := errors.New("Telegram configuration requires one bounded private regular source")
	if !file {
		return raw, nil
	}
	if inline || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", failure
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 8192 ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return "", failure
	}
	handle, err := os.Open(path)
	if err != nil {
		return "", failure
	}
	defer handle.Close()
	opened, err := handle.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) ||
		(runtime.GOOS != "windows" && opened.Mode().Perm()&0077 != 0) {
		return "", failure
	}
	content, err := io.ReadAll(io.LimitReader(handle, 8193))
	if err != nil || len(content) == 0 || len(content) > 8192 {
		return "", failure
	}
	return string(content), nil
}
