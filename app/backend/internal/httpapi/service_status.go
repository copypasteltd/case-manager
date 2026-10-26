package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/servicestatus"
	"io"
	"mime"
	"net/http"
)

func (s *Server) serviceStatusRoutes(mux *http.ServeMux) {
	service := &servicestatus.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/services", read(s, "ServiceListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		var project *string
		if v := r.URL.Query().Get("project_id"); v != "" {
			project = &v
		}
		return service.List(ctx, a, servicestatus.Filter{Query: f.Query, Lifecycle: r.URL.Query().Get("lifecycle"), Cursor: f.Cursor, Limit: f.Limit, ProjectID: project})
	}))
	mux.HandleFunc("GET /v1/services/{id}", read(s, "ServiceResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/services", command(s, "ServiceCreate", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.Create) (core.Result, error) {
		return service.Create(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/services/{id}", command(s, "ServicePatch", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.Patch) (core.Result, error) {
		return service.Update(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/services/{id}/sources", command(s, "ServiceSourceInput", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.SourceInput) (core.Result, error) {
		return service.PublishSource(ctx, c, r.PathValue("id"), "", in)
	}))
	mux.HandleFunc("POST /v1/services/{id}/sources/{source}/revisions", command(s, "ServiceSourceInput", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.SourceInput) (core.Result, error) {
		return service.PublishSource(ctx, c, r.PathValue("id"), r.PathValue("source"), in)
	}))
	mux.HandleFunc("GET /v1/services/{id}/sources/{source}/revisions", read(s, "ServiceSourceRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.SourceRevisions(ctx, a, r.PathValue("id"), r.PathValue("source"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/services/{id}/observations", read(s, "ServiceObservationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Observations(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/services/{id}/observations/{observation}/evidence", read(s, "ServiceObservationProofResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ObservationProof(ctx, a, r.PathValue("id"), r.PathValue("observation"))
	}))
	mux.HandleFunc("GET /v1/services/{id}/maintenance-windows", read(s, "MaintenanceWindowListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.MaintenanceWindows(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/services/{id}/maintenance-windows", command(s, "MaintenanceInput", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.MaintenanceInput) (core.Result, error) {
		return service.CreateMaintenance(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/services/{id}/maintenance-windows/{window}/cancel", command(s, "MaintenanceCancel", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.MaintenanceCancel) (core.Result, error) {
		return service.CancelMaintenance(ctx, c, r.PathValue("id"), r.PathValue("window"), in)
	}))
	mux.HandleFunc("GET /v1/incidents", read(s, "IncidentListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Incidents(ctx, a, f.State, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/incidents/{id}", read(s, "IncidentResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetIncident(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/incidents", command(s, "IncidentCreate", "IncidentResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.IncidentCreate) (core.Result, error) {
		return service.CreateIncident(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/incidents/{id}", command(s, "IncidentPatch", "IncidentResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.IncidentPatch) (core.Result, error) {
		return service.UpdateIncident(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/incidents/{id}/timeline", read(s, "IncidentEntryListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.IncidentTimeline(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/status-sources/{source}/observations", func(w http.ResponseWriter, r *http.Request) {
		// Dedicated server-to-server signature authentication. Browser sessions,
		// cookies and user Bearer tokens never authorize monitoring observations.
		if r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			s.fail(w, r, core.Forbidden())
			return
		}
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" {
			s.fail(w, r, core.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "请发送 application/json 观测"))
			return
		}
		signatures := r.Header.Values("X-CP-Status-Signature")
		if len(signatures) != 1 || len(signatures[0]) != 88 {
			s.fail(w, r, core.Fail(401, "PROBE_AUTHENTICATION_FAILED", "观测来源凭证无效"))
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if e != nil {
			s.fail(w, r, core.Fail(413, "BODY_TOO_LARGE", "观测请求不得超过4096字节"))
			return
		}
		ack, e := service.Ingest(r.Context(), r.PathValue("source"), signatures[0], requestID(r), body)
		if e != nil {
			s.fail(w, r, platform.DatabaseError(e))
			return
		}
		s.respond(w, r, 200, "ObservationAckResponse", ack)
	})
}
