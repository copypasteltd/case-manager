package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
)

func (s *Server) uploadRoutes(mux *http.ServeMux, metadata *files.Service) {
	service := &files.UploadService{Files: metadata, Queue: jobqueue.Queue{DB: s.DB}, Backend: s.Storage}
	s.fileImportRoutes(mux, service)
	s.historyRestoreRoutes(mux, service)
	mux.HandleFunc("GET /v1/storage/upload-capabilities", read(s, "UploadCapabilitiesResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.UploadCapabilities(ctx, a)
	}))
	mux.HandleFunc("GET /v1/uploads", read(s, "UploadListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.List(ctx, a, f.State, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/storage/scan-waiver-policies", read(s, "ScanWaiverPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.WaiverPolicies(ctx, a)
	}))
	mux.HandleFunc("POST /v1/storage/scan-waiver-policies", command(s, "ScanWaiverPolicyInput", "ScanWaiverPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.WaiverPolicyInput) (core.Result, error) {
		return service.PublishWaiverPolicy(ctx, c, in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/waivers", command(s, "ScanWaiverCreate", "ScanWaiverResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.WaiverCreate) (core.Result, error) {
		return service.RequestWaiver(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/scan-waivers", read(s, "ScanWaiverListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Waivers(ctx, a, f.State, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/scan-waivers/{id}", read(s, "ScanWaiverResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Waiver(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/scan-waivers/{id}/scan-report", read(s, "UploadScanReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.WaiverReport(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/scan-waivers/{id}/decisions", command(s, "ScanWaiverDecision", "ScanWaiverResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.WaiverDecision) (core.Result, error) {
		return service.DecideWaiver(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/scan-waivers/{id}/cancel", command(s, "Transition", "ScanWaiverResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.Transition) (core.Result, error) {
		return service.CancelWaiver(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads", command(s, "UploadCreate", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadCreate) (core.Result, error) {
		return service.Create(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/uploads/{id}", read(s, "UploadResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/uploads/{id}/scan-report", read(s, "UploadScanReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ScanReport(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/sign-parts", command(s, "SignParts", "SignedPartsResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.SignParts) (core.Result, error) {
		return service.Sign(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/ack-parts", command(s, "AckParts", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.AckParts) (core.Result, error) {
		return service.Ack(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/resume", command(s, "ResumeUpload", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.ResumeUpload) (core.Result, error) {
		return service.Resume(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/finalize", command(s, "UploadFinalize", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadExpected) (core.Result, error) {
		return service.Finalize(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/abort", command(s, "Transition", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.Transition) (core.Result, error) {
		return service.Abort(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/rescan", command(s, "UploadFinalize", "UploadResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadExpected) (core.Result, error) {
		return service.Rescan(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/commit", command(s, "UploadCommit", "FileCommitResultResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadCommit) (core.Result, error) {
		return service.Commit(ctx, c, r.PathValue("id"), in)
	}))
}
