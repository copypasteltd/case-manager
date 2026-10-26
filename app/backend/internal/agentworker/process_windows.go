//go:build windows

package agentworker

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200}
	cmd.WaitDelay = 3 * time.Second
}
func stopProcess(cmd *exec.Cmd) {
	// The launcher owns its descendant process boundary; this additionally terminates the
	// known child tree on Windows. No visible console, shell interpolation or VM is used.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	kill := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"), "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	kill.Stdout = io.Discard
	kill.Stderr = io.Discard
	_ = kill.Run()
	_ = cmd.Process.Kill()
}
