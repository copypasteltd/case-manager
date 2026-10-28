//go:build !linux

package nativeruntime

import (
	"context"
	"errors"
	"io"
)

func unavailable() error {
	return errors.New("native agent confinement requires a Linux systemd/cgroup-v2 host; no fallback is available")
}
func Check(string) error                   { return unavailable() }
func Serve(context.Context, string) error  { return unavailable() }
func PublicKey(string) (string, error)     { return "", unavailable() }
func ReleaseDigest(string) (string, error) { return "", unavailable() }
func Client(context.Context, string, string, string, string, io.Reader, io.Writer) error {
	return unavailable()
}
func Child(context.Context, string, string, string, string, string, io.Reader) error {
	return unavailable()
}
func GenerateKey(string) error { return unavailable() }
