package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

func storageOperationQuery(r *http.Request, history bool) error {
	for k, v := range r.URL.Query() {
		allowed := k == "limit" || k == "cursor" || !history && (k == "provider" || k == "state" || k == "job_state") || history && k == "replica_id"
		if !allowed || len(v) != 1 {
			return core.Invalid("filters", "查询条件无效或重复")
		}
	}
	return nil
}
func (s *Server) storageOperationsRoutes(mux *http.ServeMux, service *files.Service) {
	mux.HandleFunc("GET /v1/storage/replicas", read(s, "StorageReplicaOperationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if err := storageOperationQuery(r, false); err != nil {
			return nil, err
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.StorageReplicas(ctx, a, files.StorageOperationFilter{Provider: r.URL.Query().Get("provider"), State: f.State, JobState: r.URL.Query().Get("job_state"), Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/storage/replicas/{id}", read(s, "StorageReplicaDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if len(r.URL.Query()) != 0 {
			return nil, core.Invalid("filters", "详情不接受查询条件")
		}
		return service.StorageReplica(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/storage/replicas/{id}/retry", command(s, "StorageReplicaRetry", "StorageReplicaReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.StorageReplicaRetry) (core.Result, error) {
		if len(r.URL.Query()) != 0 {
			return core.Result{}, core.Invalid("filters", "重试不接受查询条件")
		}
		return service.RetryStorageReplica(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/storage/replica-retries", read(s, "StorageReplicaReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if err := storageOperationQuery(r, true); err != nil {
			return nil, err
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.StorageReplicaRetries(ctx, a, r.URL.Query().Get("replica_id"), f.Cursor, f.Limit)
	}))
}
