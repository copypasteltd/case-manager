package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
)

func financeReportQuery(r *http.Request, list bool) (finance.ReportQuery, string, int, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return finance.ReportQuery{}, "", 0, core.Invalid("query", "查询参数编码或分隔无效")
	}
	allowed := map[string]bool{"kind": true, "cursor": true, "limit": true}
	if list {
		allowed["state"] = true
	} else {
		for _, key := range []string{"from", "to", "currency", "category", "q", "project_id", "company_only", "supplier_id", "owner_id", "fx_book_id"} {
			allowed[key] = true
		}
	}
	for key, items := range values {
		if !allowed[key] || len(items) != 1 {
			return finance.ReportQuery{}, "", 0, core.Invalid("query", "查询参数未知或重复，不会忽略筛选条件")
		}
	}
	page, err := filter(r)
	if err != nil {
		return finance.ReportQuery{}, "", 0, err
	}
	company, err := queryBoolean(r, "company_only")
	if err != nil {
		return finance.ReportQuery{}, "", 0, err
	}
	f := finance.ReportQuery{Kind: values.Get("kind"), From: values.Get("from"), To: values.Get("to"), Currency: values.Get("currency"), Category: values.Get("category"), Query: values.Get("q"), CompanyOnly: company}
	for key, target := range map[string]**string{"project_id": &f.ProjectID, "supplier_id": &f.SupplierID, "owner_id": &f.OwnerID, "fx_book_id": &f.FXBookID} {
		if value := values.Get(key); value != "" {
			*target = &value
		}
	}
	return f, page.Cursor, page.Limit, nil
}

func (s *Server) financeReportRoutes(mux *http.ServeMux) {
	service := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	s.financeFXRoutes(mux, service)
	mux.HandleFunc("GET /v1/finance/reports", read(s, "FinancialReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, cursor, limit, err := financeReportQuery(r, false)
		if err != nil {
			return nil, err
		}
		return service.FinancialReport(ctx, a, f, cursor, limit)
	}))
	mux.HandleFunc("GET /v1/finance/report-options", read(s, "FinancialReportOptionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(values["field"]) != 1 || len(values["option_search"]) > 1 {
			return nil, core.Invalid("query", "候选字段或搜索参数无效／重复")
		}
		field, search := values.Get("field"), values.Get("option_search")
		values.Del("field")
		values.Del("option_search")
		copy := r.Clone(ctx)
		copiedURL := *r.URL
		copy.URL = &copiedURL
		copy.URL.RawQuery = values.Encode()
		f, cursor, limit, err := financeReportQuery(copy, false)
		if err != nil {
			return nil, err
		}
		return service.ReportOptions(ctx, a, f, field, search, cursor, limit)
	}))
	mux.HandleFunc("POST /v1/finance/report-exports", command(s, "FinancialReportExportCreate", "FinancialReportExportResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.ReportExportCreate) (core.Result, error) {
		return service.CreateReportExport(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/finance/report-exports", read(s, "FinancialReportExportListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, cursor, limit, err := financeReportQuery(r, true)
		if err != nil {
			return nil, err
		}
		return service.ReportExports(ctx, a, f.Kind, r.URL.Query().Get("state"), cursor, limit)
	}))
	mux.HandleFunc("GET /v1/finance/report-exports/{id}", read(s, "FinancialReportExportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "导出状态不接受附加筛选")
		}
		return service.ReportExport(ctx, a, r.PathValue("id"))
	}))
	type cancelInput struct {
		Reason string `json:"reason"`
	}
	mux.HandleFunc("POST /v1/finance/report-exports/{id}/cancel", command(s, "FinancialReportExportCancel", "FinancialReportExportResponse", func(ctx context.Context, c core.Command, r *http.Request, in cancelInput) (core.Result, error) {
		return service.CancelReportExport(ctx, c, r.PathValue("id"), in.Reason)
	}))
	type chunk struct {
		finance.ReportDownload
		Offset      string `json:"offset"`
		Length      string `json:"length"`
		ChunkSHA256 string `json:"chunk_sha256"`
		Data        []byte `json:"data_base64"`
	}
	gate := &downloadGate{users: map[string]bool{}}
	mux.HandleFunc("GET /v1/finance/report-exports/{id}/chunks", read(s, "FinancialReportExportChunkResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 2 || len(query["offset"]) != 1 || len(query["length"]) != 1 {
			return nil, core.Invalid("query", "请提供唯一的字节起点与长度")
		}
		offset, e1 := strconv.Atoi(query.Get("offset"))
		length, e2 := strconv.Atoi(query.Get("length"))
		if e1 != nil || e2 != nil || offset < 0 || length < 1 || length > 512<<10 || offset > finance.ReportExportMaxBytes || strconv.Itoa(offset) != query.Get("offset") || strconv.Itoa(length) != query.Get("length") {
			return nil, core.Invalid("range", "每段最多512KiB，字节范围需为规范整数")
		}
		if !gate.acquire(a) {
			return nil, core.Fail(429, "EXPORT_DOWNLOAD_BUSY", "本账号已有下载，请等待结束")
		}
		defer gate.release(a)
		content, err := service.DownloadReportExport(ctx, a, r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		if offset >= len(content.Content) {
			return nil, core.Fail(416, "EXPORT_RANGE_INVALID", "字节起点超出报表")
		}
		data := content.Content[offset:min(offset+length, len(content.Content))]
		return chunk{ReportDownload: content, Offset: strconv.Itoa(offset), Length: strconv.Itoa(len(data)), ChunkSHA256: core.Digest(data), Data: data}, nil
	}))
	mux.HandleFunc("GET /v1/finance/report-exports/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Range") != "" {
			s.fail(w, r, core.Invalid("download", "请使用完整鉴权GET下载，不接受查询参数或范围头"))
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
		content, err := service.DownloadReportExport(r.Context(), a, r.PathValue("id"))
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+content.Filename+`"`)
		w.Header().Set("Content-Length", content.Bytes)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Content-SHA256", content.SHA256)
		checked := time.Now()
		for offset := 0; offset < len(content.Content); offset += 64 << 10 {
			if time.Since(checked) > 2*time.Second {
				if _, err = service.DownloadReportExport(r.Context(), a, r.PathValue("id")); err != nil {
					return // Content-Length makes an interrupted download fail closed.
				}
				checked = time.Now()
			}
			if _, err = w.Write(content.Content[offset:min(offset+(64<<10), len(content.Content))]); err != nil {
				return
			}
		}
	})
}
