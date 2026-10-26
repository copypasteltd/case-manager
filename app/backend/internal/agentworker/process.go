package agentworker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
)

type Executor interface {
	Execute(context.Context, agent.Execution) error
}
type NativeSettings struct {
	Enabled          bool   `json:"enabled"`
	Model            string `json:"model"`
	WorkingDirectory string `json:"working_directory"`
	ProfileID        string `json:"profile_id"`
	MCPEndpoint      string `json:"mcp_endpoint"`
	APIKey           string `json:"api_key,omitempty"`
	ExecutablePath   string `json:"executable_path,omitempty"`
}
type ProcessExecutor struct {
	Launcher, Node, Entry, Repository string
	Settings                          NativeSettings
}

func processEnvironment() []string {
	allowed := map[string]bool{"path": true, "systemroot": true, "windir": true, "comspec": true, "pathext": true, "lang": true, "lc_all": true, "tmp": true, "temp": true}
	out := []string{}
	for _, v := range os.Environ() {
		key, _, ok := strings.Cut(v, "=")
		if ok && allowed[strings.ToLower(key)] {
			out = append(out, v)
		}
	}
	return out
}

// The configured launcher is an operator-owned native isolation boundary, not a shell.
// Its contract is --node <absolute executable> --entry <absolute module>, JSON stdin.
// It must forward termination to every descendant; signed native verification is still
// mandatory before the adapter can instantiate Codex. No DB/session/storage env is inherited.
func (p ProcessExecutor) Execute(ctx context.Context, run agent.Execution) error {
	settings := p.Settings
	settings.Enabled = true
	settings.ProfileID = run.Profile.ID
	body, err := json.Marshal(map[string]any{"repository": p.Repository, "run": run, "settings": settings})
	if err != nil {
		return err
	}
	cmd := exec.Command(p.Launcher, "--node", p.Node, "--entry", p.Entry)
	cmd.Env = processEnvironment()
	cmd.Dir = p.Settings.WorkingDirectory
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = runOwnedProcess(ctx, cmd); err != nil {
		return core.Fail(409, "RUNTIME_EXECUTOR_EXITED", "原生执行进程已停止")
	}
	return nil
}
func runOwnedProcess(ctx context.Context, cmd *exec.Cmd) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	prepareProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		select {
		case err := <-done:
			return err
		default:
		}
		stopProcess(cmd)
		<-done
		return ctx.Err()
	}
}
