package httpapi

import (
	"context"
	"io"
	"mime"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/servicestatus"
)

func (s *Server) hostResourceRoutes(mux *http.ServeMux, service *servicestatus.Service) {
	mux.HandleFunc("GET /v1/services/{id}/resource-monitor/health-events", read(s, "ResourceHealthEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.ResourceHealthHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/services/{id}/resource-monitor", read(s, "ResourceViewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ResourceMonitor(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/services/{id}/resource-monitor/revisions", command(s, "ResourceConfigInput", "ServiceResponse", func(ctx context.Context, c core.Command, r *http.Request, in servicestatus.ResourceConfigInput) (core.Result, error) {
		return service.PublishResourceConfig(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/services/{id}/resource-monitor/revisions", read(s, "ResourceRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.ResourceRevisions(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/services/{id}/resource-monitor/observations", read(s, "ResourceObservationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.ResourceObservations(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/services/{id}/resource-monitor/observations/{observation}/evidence", read(s, "ResourceProofResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ResourceObservationProof(ctx, a, r.PathValue("id"), r.PathValue("observation"))
	}))
	mux.HandleFunc("POST /v1/resource-sources/{source}/observations", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			s.fail(w, r, core.Forbidden())
			return
		}
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" {
			s.fail(w, r, core.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "请发送 application/json 观测"))
			return
		}
		sig := r.Header.Values("X-CP-Status-Signature")
		if len(sig) != 1 || len(sig[0]) != 88 {
			s.fail(w, r, core.Fail(401, "PROBE_AUTHENTICATION_FAILED", "观测来源凭证无效"))
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if e != nil {
			s.fail(w, r, core.Fail(413, "BODY_TOO_LARGE", "资源观测不得超过16384字节"))
			return
		}
		ack, e := service.IngestResources(r.Context(), r.PathValue("source"), sig[0], requestID(r), body)
		if e != nil {
			s.fail(w, r, platform.DatabaseError(e))
			return
		}
		s.respond(w, r, 200, "ObservationAckResponse", ack)
	})
}
