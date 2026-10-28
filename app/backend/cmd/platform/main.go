package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"copypaste.hk/platform/internal/contract"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/httpapi"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/storageconfig"
	"copypaste.hk/platform/internal/telegram"
	"copypaste.hk/platform/internal/uploadworker"
	"copypaste.hk/platform/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		// Driver errors can contain connection details or submitted values.
		// Print only our safe startup diagnosis; richer diagnostics belong in
		// a restricted operator tool rather than an HTTP/application log.
		logger.Error("platform stopped", "diagnosis", safeStartupError(err))
		os.Exit(1)
	}
}
func safeStartupError(err error) string {
	var diagnosis *startupError
	if errors.As(err, &diagnosis) {
		return diagnosis.message
	}
	return "command failed; inspect configuration and restricted migration diagnostics"
}

type startupError struct{ message string }

func (e *startupError) Error() string { return e.message }
func run(logger *slog.Logger) error {
	command := "api"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "worker-multipart-cleanup" {
		return runMultipartCleanupWorker(ctx, logger)
	}
	if command == "worker-file-text" {
		return runTextWorker(ctx, logger)
	}
	if command == "worker-report-export" {
		return runReportExportWorker(ctx, logger)
	}
	if command == "worker-subscription" {
		return runSubscriptionWorker(ctx, logger)
	}
	if command == "worker-notification" {
		return runNotificationWorker(ctx, logger)
	}
	if command == "worker-telegram" {
		return runTelegramWorker(ctx, logger)
	}
	if command == "worker-telegram-delivery" {
		return runTelegramDeliveryWorker(ctx, logger)
	}
	if command == "worker-agent" || command == "worker-agent-reconcile" {
		return runAgentWorker(ctx, logger, command == "worker-agent-reconcile")
	}
	if command == "migrate" {
		connection := os.Getenv("CP_MIGRATION_DATABASE_URL")
		if connection == "" {
			return &startupError{"CP_MIGRATION_DATABASE_URL is required"}
		}
		if err := migrations.Apply(ctx, connection); err != nil {
			// Explicit migration command is an operator surface. Error text may
			// name schema objects, never include a URL or SQL parameters.
			fmt.Fprintln(os.Stderr, redactMigrationError(err, connection))
			return err
		}
		logger.Info("migrations applied")
		return nil
	}
	if command == "dev-seed" {
		if os.Getenv("CP_ENV") != "development" {
			return &startupError{"dev-seed is restricted to development"}
		}
		connection := os.Getenv("CP_MIGRATION_DATABASE_URL")
		parsed, err := url.Parse(connection)
		if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Path != "/copypaste" {
			return &startupError{"dev-seed requires the owned local development database"}
		}
		var input identity.DevelopmentSeed
		decoder := json.NewDecoder(http.MaxBytesReader(nil, os.Stdin, 20<<10))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err != nil {
			return &startupError{"invalid development seed input"}
		}
		db, err := platform.OpenDatabase(ctx, connection)
		if err != nil {
			return err
		}
		defer db.Close()
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		ids, err := identity.SeedDevelopment(ctx, tx, input)
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"development_user_ids": ids, "credentials_printed": false})
	}
	if command == "bootstrap" {
		options := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
		org := options.String("organization", "CopyPaste", "organization name")
		login := options.String("login", "", "initial administrator login")
		name := options.String("name", "", "initial administrator display name")
		if err := options.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *login == "" || *name == "" {
			return &startupError{"bootstrap requires --login and --name; password is read from standard input"}
		}
		connection := os.Getenv("CP_MIGRATION_DATABASE_URL")
		if connection == "" {
			return &startupError{"migration identity is required for bootstrap"}
		}
		password, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && len(password) == 0 {
			return &startupError{"a password must be supplied on standard input"}
		}
		db, err := platform.OpenDatabase(ctx, connection)
		if err != nil {
			return err
		}
		defer db.Close()
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		result, err := identity.Bootstrap(ctx, tx, identity.BootstrapInput{Organization: *org, Login: *login, DisplayName: *name, Password: strings.TrimRight(password, "\r\n")})
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if command == "worker-upload" || command == "worker-verify-small" || command == "worker-verify-large" || command == "worker-promote-small" || command == "worker-promote-large" || command == "worker-replica-small" || command == "worker-replica-large" || command == "worker-restore-small" || command == "worker-restore-large" {
		c, err := platform.LoadWorkerConfig()
		if err != nil {
			return &startupError{err.Error()}
		}
		var backend *files.UploadBackend
		var replicaTarget *files.UploadBackend
		var store, target objectstore.Store
		mode := strings.TrimPrefix(command, "worker-")
		var scanner filescan.Scanner
		switch {
		case command == "worker-upload":
			mode = "control"
			backend, store, err = storageconfig.LoadUploadWorker()
		case strings.HasPrefix(command, "worker-restore-"):
			backend, store, target, err = storageconfig.LoadRestoreWorker()
		case strings.HasPrefix(command, "worker-promote-"):
			backend, store, target, err = storageconfig.LoadPromotionWorker()
		case strings.HasPrefix(command, "worker-replica-"):
			backend, replicaTarget, store, target, err = storageconfig.LoadReplicaWorker()
		default:
			backend, store, err = storageconfig.LoadVerificationWorker()
			if err == nil {
				scanner, err = filescan.Load()
			}
		}
		if err != nil {
			return &startupError{err.Error()}
		}
		db, err := platform.OpenDatabase(ctx, c.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		service := &files.UploadService{Files: &files.Service{DB: db, Commands: platform.Commands{DB: db}}, Queue: jobqueue.Queue{DB: db}, Backend: backend, ReplicaTarget: replicaTarget}
		if err = migrations.CheckCompiled(ctx, db); err != nil {
			return &startupError{"database schema does not match this binary; run controlled migrations"}
		}
		runner := &uploadworker.Runner{Service: service, Store: store, TargetStore: target, Owner: c.Owner, Logger: logger, Mode: mode, Scanner: scanner}
		logger.Info("upload worker started", "mode", mode)
		return runner.Run(ctx)
	}
	if command != "api" {
		return &startupError{"supported commands: api, worker-upload, worker-verify-small/large, worker-promote-small/large, worker-replica-small/large, worker-restore-small/large, migrate, bootstrap, dev-seed (development only)"}
	}
	c, err := platform.LoadConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	storage, err := storageconfig.LoadAPI()
	if err != nil {
		return &startupError{err.Error()}
	}
	cleanupTargets, err := storageconfig.LoadMultipartCleanupTargets()
	if err != nil {
		return &startupError{err.Error()}
	}
	downloads, err := storageconfig.LoadDownloads()
	if err != nil {
		return &startupError{err.Error()}
	}
	agentReaders, err := storageconfig.LoadAgentTextReaders()
	if err != nil {
		return &startupError{err.Error()}
	}
	agentDrafts, err := storageconfig.LoadAgentDraftWriter()
	if err != nil {
		return &startupError{err.Error()}
	}
	telegramBots, err := telegram.LoadBots()
	if err != nil {
		return &startupError{err.Error()}
	}
	db, err := platform.OpenDatabase(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return &startupError{"database schema does not match this binary; run controlled migrations"}
	}
	validator, err := contract.New()
	if err != nil {
		return err
	}
	identities, err := identity.NewService(db, c)
	if err != nil {
		return err
	}
	projects := &project.Service{DB: db, Commands: platform.Commands{DB: db}, Pager: platform.Pager{Key: c.CursorKey}}
	api := &httpapi.Server{DB: db, Config: c, Identity: identities, Projects: projects, Contract: validator, Logger: logger, Storage: storage, MultipartCleanup: cleanupTargets, Downloads: downloads, AgentFileReaders: agentReaders, AgentDraftWriter: agentDrafts}
	api.Telegram = &telegram.Service{DB: db, Config: c, Bots: telegramBots}
	server := &http.Server{Addr: c.HTTPAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	failures := make(chan error, 1)
	go func() { logger.Info("api listening", "address", c.HTTPAddr); failures <- server.ListenAndServe() }()
	select {
	case err = <-failures:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}
func redactMigrationError(err error, connection string) string {
	message := strings.ReplaceAll(err.Error(), connection, "[database]")
	// libpq URLs can be echoed without the exact original formatting.
	if strings.Contains(message, "postgres://") || strings.Contains(message, "postgresql://") {
		return "migration connection failed"
	}
	return message
}
