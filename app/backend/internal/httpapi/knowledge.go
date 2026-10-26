package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/platform"
	"net/http"
)

func (s *Server) knowledgeService() *knowledge.Service {
	return &knowledge.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
}
func (s *Server) knowledgeRoutes(mux *http.ServeMux) {
	service := s.knowledgeService()
	mux.HandleFunc("GET /v1/posts", read(s, "PostListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.List(ctx, a, knowledge.Filter{Query: f.Query, State: f.State, ProjectID: r.URL.Query().Get("project_id"), Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/posts", command(s, "PostCreate", "PostResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.Create) (core.Result, error) {
		return service.Create(ctx, c, input)
	}))
	mux.HandleFunc("GET /v1/posts/{id}", read(s, "PostResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("PATCH /v1/posts/{id}", command(s, "PostPatch", "PostResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.Patch) (core.Result, error) {
		return service.Update(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/posts/{id}/publish", command(s, "PostTransition", "PostResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.Transition) (core.Result, error) {
		return service.Transition(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/posts/{id}/revisions", read(s, "PostRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.History(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/posts/{id}/restore", command(s, "PostRestore", "PostResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.Restore) (core.Result, error) {
		return service.Restore(ctx, c, r.PathValue("id"), input)
	}))
	for _, parent := range []struct{ table, kind string }{{"posts", "post"}, {"tasks", "task"}} {
		inputSchema := "CommentCreate"
		if parent.kind == "post" {
			inputSchema = "PostCommentCreate"
		}
		mux.HandleFunc("GET /v1/"+parent.table+"/{id}/comments", read(s, "CommentsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			f, err := filter(r)
			if err != nil {
				return nil, err
			}
			return service.Comments(ctx, a, parent.kind, r.PathValue("id"), knowledge.Filter{Cursor: f.Cursor, Limit: f.Limit})
		}))
		mux.HandleFunc("POST /v1/"+parent.table+"/{id}/comments", command(s, inputSchema, "CommentResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.CommentInput) (core.Result, error) {
			return service.WriteComment(ctx, c, parent.kind, r.PathValue("id"), "", input)
		}))
	}
	mux.HandleFunc("PATCH /v1/comments/{id}", command(s, "CommentPatch", "CommentResponse", func(ctx context.Context, c core.Command, r *http.Request, input knowledge.CommentInput) (core.Result, error) {
		return service.EditComment(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/comments/{id}/history", read(s, "CommentsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.CommentHistory(ctx, a, r.PathValue("id"), knowledge.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
}
