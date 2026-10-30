package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"net/http"
)

func (s *Server) officeAuthorRoutes(mux *http.ServeMux, fs *files.Service) {
	service := &files.OfficeService{Files: fs, Backends: s.Downloads}
	mux.HandleFunc("GET /v1/files/{id}/versions/{version}/office", read(s, "OfficeReadResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Read(ctx, a, r.PathValue("id"), r.PathValue("version"))
	}))
	mux.HandleFunc("POST /v1/folders/{id}/office-render", officeRender(s, "OfficeCreate", func(ctx context.Context, a core.Actor, r *http.Request, in files.OfficeCreate) (files.OfficeArtifact, error) {
		return service.Create(ctx, a, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/files/{id}/office-render", officeRender(s, "OfficeEdit", func(ctx context.Context, a core.Actor, r *http.Request, in files.OfficeEdit) (files.OfficeArtifact, error) {
		return service.Edit(ctx, a, r.PathValue("id"), in)
	}))
}

// Rendering produces an ephemeral artifact. The existing audited upload commit
// remains the only operation that publishes its bytes as a formal version.
func officeRender[T any](s *Server, schema string, fn func(context.Context, core.Actor, *http.Request, T) (files.OfficeArtifact, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, true)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		in, _, err := decode[T](s, w, r, schema)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		out, err := fn(r.Context(), a, r, in)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, "OfficeArtifactResponse", out)
	}
}
