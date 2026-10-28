// file-import operates through the ordinary authorized API and signed uploads.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileimport"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
)

type origins []string

func (v *origins) String() string     { return strings.Join(*v, ",") }
func (v *origins) Set(s string) error { *v = append(*v, s); return nil }

func execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("COMMAND_REQUIRED: inventory | template | plan | run | report | retry")
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(stderr)
	root := f.String("root", "", "Read-only source root; outputs must be outside it")
	output := f.String("out", "", "New private output file (never overwritten)")
	namespace := f.String("namespace", "", "Stable canonical source UUID, reused for future delta plans")
	inventory := f.String("inventory", "", "Saved inventory JSONL")
	mapping := f.String("mapping", "", "Reviewed mapping JSONL in inventory order")
	planPath := f.String("plan", "", "Saved exact plan JSONL")
	reason := f.String("reason", "", "Explicit review/retry reason")
	api := f.String("api", "", "HTTPS API base ending in /v1")
	local := f.Bool("allow-loopback-http", false, "Native isolated tests only; numeric loopback HTTP")
	var uploadOrigins origins
	f.Var(&uploadOrigins, "upload-origin", "Explicit HTTPS object upload origin; repeat for each allowed origin")
	approve := f.String("approve-plan", "", "Exact reviewed plan SHA256")
	reviewed := f.Bool("reviewed-mappings", false, "Confirm every source owner, folder, project, classification and update base was reviewed")
	readonly := f.Bool("sources-read-only", false, "Confirm source read-only window is in force")
	only := f.String("only-source-key", "", "Resume one exact plan row; original whole manifest remains required")
	wait := f.Duration("wait", 24*time.Hour, "Maximum transfer/worker wait per file, separate from API session and upload expiry")
	batch := f.String("batch", "", "Exact durable batch UUID")
	entry := f.String("entry", "", "Exact durable entry UUID for reviewed retry")
	upload := f.String("upload", "", "Old failed/aborted/expired upload UUID for reviewed retry")
	version := f.Int64("version", 0, "Observed old upload version for reviewed retry")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("UNEXPECTED_POSITIONAL_ARGUMENT")
	}
	if *output == "" {
		return fmt.Errorf("NEW_OUTPUT_PATH_REQUIRED")
	}
	// Output publication is exclusive. Preserve partial run/report artifacts with
	// an explicit operation_error row, so interrupted evidence cannot imply success.
	var operationErr error
	err := fileimport.PrivateOutput(*output, *root, func(out *os.File) error {
		open := func(name string) (*os.File, error) {
			if name == "" {
				return nil, fmt.Errorf("INPUT_PATH_REQUIRED")
			}
			f, e := os.Open(name)
			if e != nil {
				return nil, fmt.Errorf("INPUT_UNREADABLE")
			}
			return f, nil
		}
		switch command {
		case "inventory":
			if *root == "" {
				return fmt.Errorf("SOURCE_ROOT_REQUIRED")
			}
			_, e := fileimport.Inventory(ctx, *root, *namespace, out)
			return e
		case "template":
			in, e := open(*inventory)
			if e != nil {
				return e
			}
			defer in.Close()
			return fileimport.Template(in, out)
		case "plan":
			in, e := open(*inventory)
			if e != nil {
				return e
			}
			defer in.Close()
			m, e := open(*mapping)
			if e != nil {
				return e
			}
			defer m.Close()
			spool, e := os.CreateTemp(filepath.Dir(out.Name()), ".copypaste-import-plan-*")
			if e != nil {
				return fmt.Errorf("PLAN_SPOOL_UNAVAILABLE")
			}
			defer os.Remove(spool.Name())
			defer spool.Close()
			_, e = fileimport.BuildPlan(in, m, spool, out, *reason)
			return e
		case "run", "report", "retry":
			client, e := fileimport.NewClient(*api, os.Getenv("CP_FILE_IMPORT_TOKEN"), uploadOrigins, *local)
			if e != nil {
				return e
			}
			defer client.Close()
			runner := fileimport.Runner{Client: client, WaitLimit: *wait, OnlySourceKey: *only, ReviewedMappings: *reviewed, SourcesReadOnly: *readonly}
			switch command {
			case "run":
				if *root == "" || *wait <= 0 {
					return fmt.Errorf("SOURCE_ROOT_AND_POSITIVE_WAIT_REQUIRED")
				}
				in, e := open(*planPath)
				if e != nil {
					return e
				}
				defer in.Close()
				id, e := runner.Run(ctx, *root, *planPath, *approve, out)
				operationErr = e
				if e != nil {
					if e = json.NewEncoder(out).Encode(map[string]any{"type": "operation_error", "error_code": safeCode(operationErr), "batch_id": id}); e != nil {
						return e
					}
				}
				if id != "" && ctx.Err() == nil {
					_, e = runner.Report(ctx, id, out)
					if e != nil {
						operationErr = errors.Join(operationErr, e)
						return json.NewEncoder(out).Encode(map[string]any{"type": "report_error", "error_code": safeCode(e)})
					}
				}
			case "report":
				_, operationErr = runner.Report(ctx, *batch, out)
				if operationErr != nil {
					return json.NewEncoder(out).Encode(map[string]any{"type": "report_error", "error_code": safeCode(operationErr)})
				}
			case "retry":
				if !importspec.CanonicalID(*entry) || !importspec.CanonicalID(*upload) || *version < 1 || !importspec.Hash(*approve) || !*reviewed || !*readonly || core.Name(*reason) != nil {
					return fmt.Errorf("EXPLICIT_RETRY_REVIEW_REQUIRED")
				}
				var current files.ImportEntry
				if e = client.Request(ctx, "GET", "/file-import-entries/"+*entry, "", nil, &current); e != nil {
					return e
				}
				if current.ID != *entry || !importspec.CanonicalID(current.BatchID) || current.Upload == nil || current.Upload.ID != *upload || current.Upload.Version != *version {
					return fmt.Errorf("RETRY_RECEIPT_CHANGED")
				}
				var b files.ImportBatch
				if e = client.Request(ctx, "GET", "/file-imports/"+current.BatchID, "", nil, &b); e != nil {
					return e
				}
				if b.ID != current.BatchID || b.PlanSHA256 != *approve || !b.Approved {
					return fmt.Errorf("RETRY_APPROVED_PLAN_REQUIRED")
				}
				in := files.ImportRetry{UploadID: *upload, ExpectedVersion: *version, Reason: *reason}
				var result files.ImportEntry
				operationErr = client.Request(ctx, "POST", "/file-import-entries/"+*entry+"/retry", core.NewID(), in, &result)
				if operationErr != nil {
					return json.NewEncoder(out).Encode(map[string]any{"type": "retry_error", "error_code": safeCode(operationErr)})
				}
				if result.ID != current.ID || result.Upload == nil || result.Upload.ID == *upload {
					return fmt.Errorf("RETRY_RECEIPT_INCOMPLETE_REREAD_BEFORE_RETRY")
				}
				return json.NewEncoder(out).Encode(map[string]any{"type": "retry_reserved", "entry_id": result.ID, "upload_id": result.Upload.ID, "plan_sha256": *approve, "resume_required": true})
			}
			return nil
		default:
			return fmt.Errorf("UNKNOWN_COMMAND")
		}
	})
	if err != nil {
		return err
	}
	if operationErr != nil {
		return operationErr
	}
	_, err = fmt.Fprintln(stdout, "PRIVATE_OUTPUT_SAVED")
	return err
}

// Do not emit arbitrary filesystem paths, server messages, signed URLs or secrets.
func safeCode(err error) string {
	var api *fileimport.APIError
	if errors.As(err, &api) {
		return api.Code
	}
	text := err.Error()
	if len(text) > 120 {
		return "IMPORT_OPERATION_FAILED"
	}
	for _, r := range text {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "IMPORT_OPERATION_FAILED"
		}
	}
	return text
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := execute(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, safeCode(err))
		os.Exit(1)
	}
}
