package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"copypaste.hk/platform/internal/auditlog"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func auditQuery(r *http.Request, events bool) (auditlog.Filter, string, int, error) {
	var f auditlog.Filter
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return f, "", 0, core.Invalid("query", "查询参数编码无效")
	}
	fields := map[string]*string{"from": &f.From, "to": &f.To, "actor_id": &f.ActorID, "executor_id": &f.ExecutorID, "channel": &f.Channel, "action": &f.Action, "result": &f.Result, "resource_type": &f.ResourceType, "resource_id": &f.ResourceID, "request_id": &f.RequestID, "run_id": &f.RunID, "job_id": &f.JobID}
	for k, values := range q {
		if len(values) != 1 {
			return f, "", 0, core.Invalid("query", "不接受重复筛选")
		}
		if k == "cursor" || k == "limit" {
			continue
		}
		target, ok := fields[k]
		if !events || !ok {
			return f, "", 0, core.Invalid("query", "不接受未知筛选；没有忽略条件")
		}
		*target = values[0]
	}
	limit := 20
	if v, ok := q["limit"]; ok {
		n, e := strconv.Atoi(v[0])
		if e != nil || n < 1 || n > 100 || strconv.Itoa(n) != v[0] {
			return f, "", 0, core.Invalid("limit", "每页需为1至100项的规范整数")
		}
		limit = n
	}
	if events {
		if err = f.Validate(); err != nil {
			return f, "", 0, err
		}
	}
	return f, q.Get("cursor"), limit, nil
}

func (s *Server) auditRoutes(mux *http.ServeMux) {
	service := &auditlog.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/audit/events", read(s, "AuditEventPageResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, cursor, limit, err := auditQuery(r, true)
		if err != nil {
			return nil, err
		}
		return service.Events(ctx, a, f, cursor, limit)
	}))
	mux.HandleFunc("GET /v1/audit/events/{id}", read(s, "AuditEventResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "详情不接受附加条件")
		}
		return service.Event(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/audit/exports", command(s, "AuditExportCreate", "AuditExportResponse", func(ctx context.Context, c core.Command, r *http.Request, in auditlog.ExportCreate) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "导出条件必须在正文中完整固定")
		}
		return service.CreateExport(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/audit/exports", read(s, "AuditExportListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		_, cursor, limit, err := auditQuery(r, false)
		if err != nil {
			return nil, err
		}
		return service.Exports(ctx, a, cursor, limit)
	}))
	mux.HandleFunc("GET /v1/audit/exports/{id}", read(s, "AuditExportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "详情不接受附加条件")
		}
		return service.Export(ctx, a, r.PathValue("id"))
	}))
	type chunk struct {
		auditlog.Download
		Offset      string `json:"offset"`
		Length      string `json:"length"`
		ChunkSHA256 string `json:"chunk_sha256"`
		Data        []byte `json:"data_base64"`
	}
	gate := &downloadGate{users: map[string]bool{}}
	mux.HandleFunc("GET /v1/audit/exports/{id}/chunks", read(s, "AuditExportChunkResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(q) != 2 || len(q["offset"]) != 1 || len(q["length"]) != 1 || r.Header.Get("Range") != "" {
			return nil, core.Invalid("query", "请提供唯一的字节起点和长度")
		}
		o, e1 := strconv.Atoi(q.Get("offset"))
		n, e2 := strconv.Atoi(q.Get("length"))
		if e1 != nil || e2 != nil || o < 0 || o > auditlog.MaxExportBytes || n < 1 || n > auditlog.ChunkBytes || strconv.Itoa(o) != q.Get("offset") || strconv.Itoa(n) != q.Get("length") {
			return nil, core.Invalid("range", "字节范围无效，每段最多512 KiB")
		}
		if !gate.acquire(a) {
			return nil, core.Fail(429, "EXPORT_DOWNLOAD_BUSY", "本账号已有下载，请等待结束")
		}
		defer gate.release(a)
		content, err := service.Download(ctx, a, r.PathValue("id"), requestID(r))
		if err != nil {
			return nil, err
		}
		if o >= len(content.Content) {
			return nil, core.Fail(416, "EXPORT_RANGE_INVALID", "字节起点超出固定导出")
		}
		part := content.Content[o:min(o+n, len(content.Content))]
		return chunk{Download: content, Offset: strconv.Itoa(o), Length: strconv.Itoa(len(part)), ChunkSHA256: core.Digest(part), Data: part}, nil
	}))
	mux.HandleFunc("GET /v1/audit/exports/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Range") != "" {
			s.fail(w, r, core.Invalid("download", "请使用完整鉴权GET，不接受范围头或附加条件"))
			return
		}
		a, err := s.actor(r, false)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !gate.acquire(a) {
			s.fail(w, r, core.Fail(429, "EXPORT_DOWNLOAD_BUSY", "本账号已有下载，请等待结束"))
			return
		}
		defer gate.release(a)
		content, err := service.Download(r.Context(), a, r.PathValue("id"), requestID(r))
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+content.Filename+`"`)
		w.Header().Set("Content-Length", content.TotalBytes)
		w.Header().Set("X-Content-SHA256", content.SHA256)
		for offset := 0; offset < len(content.Content); offset += 64 << 10 {
			if _, err = service.ValidateDownload(r.Context(), a, r.PathValue("id")); err != nil {
				return
			}
			if _, err = w.Write(content.Content[offset:min(offset+(64<<10), len(content.Content))]); err != nil {
				return
			}
		}
	})
}
