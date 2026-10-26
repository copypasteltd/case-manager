package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

func (s *Server) historyRestoreRoutes(mux *http.ServeMux, service *files.UploadService) {
	mux.HandleFunc("POST /v1/files/{id}/history-restores", command(s, "HistoryRestoreCreate", "HistoryRestoreResponse",
		func(ctx context.Context, c core.Command, r *http.Request, in files.HistoryRestoreCreate) (core.Result, error) {
			return service.CreateHistoryRestore(ctx, c, r.PathValue("id"), in)
		}))
	mux.HandleFunc("GET /v1/files/{id}/history-restores", read(s, "HistoryRestoreListResponse",
		func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			f, err := filter(r)
			if err != nil {
				return nil, err
			}
			return service.HistoryRestores(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
		}))
	mux.HandleFunc("GET /v1/history-restores/{id}", read(s, "HistoryRestoreResponse",
		func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			return service.HistoryRestore(ctx, a, r.PathValue("id"))
		}))
	mux.HandleFunc("POST /v1/history-restores/{id}/commit", command(s, "HistoryRestoreCommit", "FileCommitResultResponse",
		func(ctx context.Context, c core.Command, r *http.Request, in files.HistoryRestoreCommit) (core.Result, error) {
			return service.CommitHistoryRestore(ctx, c, r.PathValue("id"), in)
		}))
	mux.HandleFunc("POST /v1/history-restores/{id}/cancel", command(s, "HistoryRestoreCancel", "HistoryRestoreResponse",
		func(ctx context.Context, c core.Command, r *http.Request, in files.HistoryRestoreCancel) (core.Result, error) {
			return service.CancelHistoryRestore(ctx, c, r.PathValue("id"), in)
		}))
}
