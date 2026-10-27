package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func eventPosition(r *http.Request) (int64, int, error) {
	after, limit := int64(0), 100
	raw := r.URL.Query().Get("after_seq")
	header := r.Header.Get("Last-Event-ID")
	if raw != "" && header != "" && raw != header {
		return 0, 0, core.Invalid("after_seq", "查询和恢复事件序号不一致")
	}
	if raw == "" {
		raw = header
	}
	if raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v < 0 || v > 9007199254740991 {
			return 0, 0, core.Invalid("after_seq", "事件序号无效")
		}
		after = v
	}
	if raw = r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			return 0, 0, core.Invalid("limit", "每次读取1至100项事件")
		}
		limit = v
	}
	return after, limit, nil
}
func (s *Server) agentRunRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	s.agentBudgetRoutes(mux, service)
	mux.HandleFunc("GET /v1/conversations/{id}/runtime-options", read(s, "RuntimeOptionsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.RuntimeOptions(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/conversations/{id}/messages", read(s, "MessageListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Messages(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/runs", command(s, "RunCreate", "RunResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.RunCreate) (core.Result, error) {
		return service.CreateRun(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/runs/{id}", read(s, "RunResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetRun(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", command(s, "Transition", "RunResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.RunCancel) (core.Result, error) {
		return service.CancelRun(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/runs/{id}/event-log", read(s, "RunEventPageResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		after, limit, e := eventPosition(r)
		if e != nil {
			return nil, e
		}
		return service.RunEvents(ctx, a, r.PathValue("id"), after, limit)
	}))
	mux.HandleFunc("GET /v1/runs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		a, e := s.actor(r, false)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		after, limit, e := eventPosition(r)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		batch, e := service.RunEvents(r.Context(), a, r.PathValue("id"), after, limit)
		if e != nil {
			s.denyAudit(r, a, e)
			s.fail(w, r, e)
			return
		}
		// A bounded batch closes its database snapshot before any network writes.
		// EventSource reconnects with Last-Event-ID; mini-programs use event-log.
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("X-Accel-Buffering", "no")
		_, _ = fmt.Fprint(w, "retry: 2000\n\n")
		for _, event := range batch.Items {
			body, err := json.Marshal(event)
			if err != nil {
				return
			}
			_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, body)
			if err != nil {
				return
			}
		}
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
	})
}
