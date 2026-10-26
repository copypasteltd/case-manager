package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) filesRoutes(mux *http.ServeMux) {
	service := &files.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	s.uploadRoutes(mux, service)
	s.downloadRoutes(mux, service)
	s.retentionRoutes(mux, service)
	mux.HandleFunc("POST /v1/file-replicas/{id}/retry", command(s, "ReplicaRetry", "FileReplicaResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.ReplicaRetry) (core.Result, error) {
		return service.RetryReplica(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/folders", read(s, "FolderListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Folders(ctx, a, files.Filter{ParentID: r.URL.Query().Get("parent_id"), ProjectID: r.URL.Query().Get("project_id"), Query: f.Query, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/folders/{id}", read(s, "FolderResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Folder(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/folders", command(s, "FolderCreate", "FolderResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.FolderCreate) (core.Result, error) {
		return service.CreateFolder(ctx, c, input)
	}))
	mux.HandleFunc("POST /v1/folders/{id}/move", command(s, "Move", "FolderResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.Move) (core.Result, error) {
		return service.MoveFolder(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/files", read(s, "FileListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		companyOnly, err := queryBoolean(r, "company_only")
		if err != nil {
			return nil, err
		}
		return service.Files(ctx, a, files.Filter{CompanyOnly: companyOnly, ParentID: r.URL.Query().Get("folder_id"), ProjectID: r.URL.Query().Get("project_id"), Query: f.Query, State: f.State, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/files/{id}", read(s, "FileResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.File(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/files/{id}/versions", read(s, "FileVersionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Versions(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/file-versions/{id}", read(s, "FileVersionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.VersionByID(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/files/{id}/versions/{version}", read(s, "FileVersionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Version(ctx, a, r.PathValue("id"), r.PathValue("version"))
	}))
	mux.HandleFunc("POST /v1/files/{id}/move", command(s, "Move", "FileResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.Move) (core.Result, error) {
		return service.MoveFile(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/files/{id}/trash", command(s, "Transition", "FileResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.Transition) (core.Result, error) {
		return service.Trash(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/files/{id}/restore", command(s, "RestoreFile", "FileResponse", func(ctx context.Context, c core.Command, r *http.Request, input files.Restore) (core.Result, error) {
		return service.Restore(ctx, c, r.PathValue("id"), input)
	}))
}
