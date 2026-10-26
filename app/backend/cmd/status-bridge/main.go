package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"copypaste.hk/platform/internal/statusbridge"
)

func main() { os.Exit(run()) }

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	fs := flag.NewFlagSet("status-bridge", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	config := fs.String("config", "", "private JSON configuration file")
	once := fs.Bool("once", false, "one bounded collection/delivery cycle")
	keyFile := fs.String("generate-key", "", "create a new private seed file and print only its public key")
	if fs.Parse(os.Args[1:]) != nil || fs.NArg() != 0 {
		return 2
	}
	if *keyFile != "" {
		if *config != "" || *once {
			return 2
		}
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return 2
		}
		f, err := os.OpenFile(*keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			logger.Error("key creation refused")
			return 2
		}
		_, err = f.WriteString(base64.StdEncoding.EncodeToString(key.Seed()) + "\n")
		syncErr := f.Sync()
		closeErr := f.Close()
		if err != nil || syncErr != nil || closeErr != nil {
			logger.Error("key creation failed")
			return 2
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return 0
	}
	if *config == "" {
		logger.Error("configuration required")
		return 2
	}
	c, err := statusbridge.LoadConfig(*config)
	if err != nil {
		logger.Error("bridge stopped", "code", statusbridge.SafeCode(err))
		return 2
	}
	b, err := statusbridge.Open(c)
	if err != nil {
		logger.Error("bridge stopped", "code", statusbridge.SafeCode(err))
		return 2
	}
	defer b.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		results, err := b.Cycle(ctx)
		pending := false
		for _, r := range results {
			logger.Info("collection result", "source_id", r.SourceID, "code", r.Code, "sequence", r.Sequence, "observation_id", r.ObservationID, "applied", r.Applied, "replayed", r.Replayed)
			if r.Code != "ACKNOWLEDGED" && r.Code != "NO_NEW_SAMPLE" {
				pending = true
			}
		}
		if ctx.Err() != nil {
			return 0
		}
		if err != nil {
			logger.Error("bridge stopped", "code", statusbridge.SafeCode(err))
			return 2
		}
		if *once {
			if pending {
				return 75
			}
			return 0
		}
		timer := time.NewTimer(time.Duration(c.PollSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0
		case <-timer.C:
		}
	}
}
