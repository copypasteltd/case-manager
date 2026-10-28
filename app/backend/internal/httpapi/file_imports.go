package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
)

func (s *Server) fileImportRoutes(mux *http.ServeMux, uploads *files.UploadService) {
	service := &files.ImportService{Uploads: uploads}
	mux.HandleFunc("POST /v1/file-imports", command(s, "FileImportBatchInput", "FileImportBatchResponse", func(ctx context.Context, c core.Command, r *http.Request, in importspec.BatchInput) (core.Result, error) {
		return service.CreateBatch(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/file-imports/{id}", read(s, "FileImportBatchResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetBatch(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/file-imports/{id}/entries", command(s, "FileImportEntryInput", "FileImportEntryResponse", func(ctx context.Context, c core.Command, r *http.Request, in importspec.Entry) (core.Result, error) {
		return service.AddEntry(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/file-imports/{id}/entries", read(s, "FileImportEntryListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Entries(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/file-imports/{id}/approve", command(s, "FileImportApprovalInput", "FileImportBatchResponse", func(ctx context.Context, c core.Command, r *http.Request, in importspec.ApprovalInput) (core.Result, error) {
		return service.Approve(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/file-import-entries/{id}", read(s, "FileImportEntryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetEntry(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/file-import-entries/{id}/uploads", command(s, "EmptyCommand", "FileImportEntryResponse", func(ctx context.Context, c core.Command, r *http.Request, in struct{}) (core.Result, error) {
		return service.Start(ctx, c, r.PathValue("id"), nil)
	}))
	mux.HandleFunc("POST /v1/file-import-entries/{id}/retry", command(s, "FileImportRetryInput", "FileImportEntryResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.ImportRetry) (core.Result, error) {
		return service.Start(ctx, c, r.PathValue("id"), &in)
	}))
}
