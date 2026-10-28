package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/storageconfig"
	"copypaste.hk/platform/migrations"
)

func agentFile(key string, directory bool) (string, error) {
	value := os.Getenv(key)
	if !filepath.IsAbs(value) {
		return "", &startupError{key + " must be an absolute operator-owned native path"}
	}
	info, err := os.Stat(value)
	if err != nil || info.IsDir() != directory {
		return "", &startupError{key + " is unavailable"}
	}
	return value, nil
}
func configureAgentModel(settings *agentworker.NativeSettings) error {
	settings.Model = os.Getenv("CP_AGENT_MODEL")
	switch os.Getenv("CP_AGENT_MODEL_GATEWAY") {
	case "native":
		if os.Getenv("CP_AGENT_OPENAI_API_KEY") != "" {
			return &startupError{"native model keys belong only to the privileged broker"}
		}
		settings.APIKey = "native-broker-managed"
		settings.APIBaseURL = os.Getenv("CP_AGENT_MODEL_BASE_URL")
		u, e := url.Parse(settings.APIBaseURL)
		if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/v1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return &startupError{"native model relay requires a fixed private loopback v1 endpoint"}
		}
		var err error
		if settings.ExecutablePath, err = agentFile("CP_AGENT_CODEX_EXECUTABLE", false); err != nil {
			return err
		}
	case "":
		settings.APIKey = os.Getenv("CP_AGENT_OPENAI_API_KEY")
	default:
		return &startupError{"CP_AGENT_MODEL_GATEWAY must be native or empty"}
	}
	if settings.Model == "" || settings.APIKey == "" {
		return &startupError{"native agent requires an explicit model and dedicated API credential"}
	}
	return nil
}
func runAgentWorker(ctx context.Context, logger *slog.Logger, reconcileOnly bool) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	var executor agentworker.ProcessExecutor
	var verifier agentworker.SignedProbe
	var address string
	if !reconcileOnly {
		if executor.Launcher, err = agentFile("CP_AGENT_LAUNCHER", false); err != nil {
			return err
		}
		if executor.Node, err = agentFile("CP_AGENT_NODE", false); err != nil {
			return err
		}
		if executor.Entry, err = agentFile("CP_AGENT_ENTRY", false); err != nil {
			return err
		}
		if executor.Repository, err = agentFile("CP_AGENT_REPOSITORY", true); err != nil {
			return err
		}
		if executor.Settings.WorkingDirectory, err = agentFile("CP_AGENT_WORK_DIRECTORY", true); err != nil {
			return err
		}
		if verifier.Command, err = agentFile("CP_AGENT_PROOF_COMMAND", false); err != nil {
			return err
		}
		verifier.PublicKey, err = base64.StdEncoding.DecodeString(os.Getenv("CP_AGENT_PROOF_PUBLIC_KEY"))
		if err != nil || len(verifier.PublicKey) != ed25519.PublicKeySize {
			return &startupError{"CP_AGENT_PROOF_PUBLIC_KEY requires an Ed25519 verification key"}
		}
		if err = configureAgentModel(&executor.Settings); err != nil {
			return err
		}
		address = os.Getenv("CP_AGENT_BRIDGE_ADDRESS")
		if address == "" {
			address = "127.0.0.1:18081"
		}
		host, port, e := net.SplitHostPort(address)
		if e != nil || host != "127.0.0.1" || port == "0" || port == "" {
			return &startupError{"agent bridge must bind a fixed 127.0.0.1 port"}
		}
		executor.Settings.MCPEndpoint = "http://" + address + agentworker.MCPPath
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return &startupError{"agent worker requires matching database migrations"}
	}
	readers, err := storageconfig.LoadAgentTextReaders()
	if err != nil {
		return &startupError{err.Error()}
	}
	drafts, err := storageconfig.LoadAgentDraftWriter()
	if err != nil {
		return &startupError{err.Error()}
	}
	service := &agent.Service{DB: db, FileText: &files.AgentTextService{Backends: readers}, FileDrafts: drafts}
	runner := agentworker.Runner{Service: service, Executor: executor, Owner: "agent." + core.NewID()}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !reconcileOnly {
		listener, e := net.Listen("tcp", address)
		if e != nil {
			return &startupError{"agent bridge address is unavailable"}
		}
		bridge := &agentworker.Bridge{Service: service, Verifier: verifier, Address: address, Model: executor.Settings.Model, WorkingDirectory: executor.Settings.WorkingDirectory}
		server := &http.Server{Handler: bridge, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
		defer server.Close()
		go func() {
			if e := server.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
				cancel()
			}
		}()
	}
	logger.Info("agent worker started", "reconcile_only", reconcileOnly, "owner", runner.Owner)
	for workerCtx.Err() == nil {
		if reconcileOnly {
			_, err = service.ReconcileExecutions(workerCtx)
		} else {
			var handled bool
			handled, err = runner.Once(workerCtx)
			if err == nil && handled {
				continue
			}
		}
		if err != nil && workerCtx.Err() == nil {
			logger.Warn("agent reconciliation or execution failed", "code", core.PublicError(err).Code)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-workerCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
