package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

const proxyBytesPerSecond = 2 << 20
const proxyBufferBytes = 64 << 10
const proxyRecheckInterval = 2 * time.Second
const mobileChunkBytes = 1 << 20

type downloadChunk struct {
	DownloadID  string `json:"download_id"`
	VersionID   string `json:"file_version_id"`
	Offset      string `json:"offset"`
	Length      string `json:"length"`
	Total       string `json:"total_size_bytes"`
	SHA256      string `json:"sha256"`
	ChunkSHA256 string `json:"chunk_sha256"`
	Data        []byte `json:"data_base64"`
}

type downloadGate struct {
	mu    sync.Mutex
	total int
	users map[string]bool
}

func (g *downloadGate) acquire(a core.Actor) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := a.OrgID + "/" + a.UserID
	if g.total >= 3 || g.users[key] {
		return false
	}
	g.total++
	g.users[key] = true
	return true
}
func (g *downloadGate) release(a core.Actor) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.users, a.OrgID+"/"+a.UserID)
	g.total--
}
func (s *Server) downloadRoutes(mux *http.ServeMux, domain *files.Service) {
	service := &files.DownloadService{Files: domain, Backends: s.Downloads, PublicOrigin: s.Config.PublicOrigin}
	gate := &downloadGate{users: map[string]bool{}}
	mux.HandleFunc("POST /v1/file-versions/{id}/download", command(s, "DownloadRequest", "DownloadResultResponse",
		func(ctx context.Context, c core.Command, r *http.Request, in files.DownloadRequest) (core.Result, error) {
			return service.Create(ctx, c, r.PathValue("id"), in)
		}))
	mux.HandleFunc("GET /v1/downloads/{id}/content", func(w http.ResponseWriter, r *http.Request) {
		s.authenticatedDownload(w, r, service, gate)
	})
	mux.HandleFunc("GET /v1/downloads/{id}/chunks", func(w http.ResponseWriter, r *http.Request) {
		// A bounded JSON transport avoids depending on native SDK response-header
		// or redirect APIs. It uses the exact same authorization and source read.
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 2 || len(query["offset"]) != 1 || len(query["length"]) != 1 || r.Header.Get("Range") != "" {
			s.fail(w, r, core.Fail(400, "DOWNLOAD_CHUNK_INVALID", "请提供唯一的字节起点与长度"))
			return
		}
		offset, er1 := strconv.ParseInt(query.Get("offset"), 10, 64)
		length, er2 := strconv.ParseInt(query.Get("length"), 10, 64)
		if er1 != nil || er2 != nil || offset < 0 || length < 0 || length > mobileChunkBytes ||
			strconv.FormatInt(offset, 10) != query.Get("offset") || strconv.FormatInt(length, 10) != query.Get("length") ||
			(length == 0 && offset != 0) || offset > (1<<63-1)-length {
			s.fail(w, r, core.Fail(416, "DOWNLOAD_CHUNK_INVALID", "每段最多读取1MiB，起点与长度必须为规范的非负整数"))
			return
		}
		copy := r.Clone(r.Context())
		copy.Header = r.Header.Clone()
		if length > 0 {
			copy.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		}
		s.serveAuthenticatedDownload(w, copy, service, gate, true, length == 0)
	})
}
func (s *Server) authenticatedDownload(w http.ResponseWriter, r *http.Request, service *files.DownloadService, gate *downloadGate) {
	s.serveAuthenticatedDownload(w, r, service, gate, false, false)
}
func (s *Server) serveAuthenticatedDownload(w http.ResponseWriter, r *http.Request, service *files.DownloadService, gate *downloadGate, asChunk, emptyChunk bool) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		s.fail(w, r, core.Fail(405, "METHOD_NOT_ALLOWED", "此入口仅支持带鉴权的GET范围读取"))
		return
	}
	a, err := s.actor(r, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	deny := func(err error) {
		s.denyAudit(r, a, err)
		s.fail(w, r, err)
	}
	p, err := service.Resolve(r.Context(), a, r.PathValue("id"))
	if err != nil {
		deny(err)
		return
	}
	if p.Mode != "authenticated_proxy" {
		deny(core.Fail(409, "DOWNLOAD_MODE_MISMATCH", "此下载授权使用对象存储直读"))
		return
	}
	etag := `"` + p.SHA256 + `"`
	if asChunk && r.Header.Get("If-Range") == "" {
		deny(core.Fail(428, "DOWNLOAD_CONDITION_REQUIRED", "分段读取必须核对固定版本摘要"))
		return
	}
	if value := r.Header.Get("If-Range"); value != "" && value != etag {
		deny(core.Fail(412, "DOWNLOAD_SOURCE_CHANGED", "读取条件与固定文件版本不一致"))
		return
	}
	offset, length, partial, err := files.DownloadRange(r.Header.Get("Range"), p.Size)
	if err != nil {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(p.Size, 10))
		deny(err)
		return
	}
	if asChunk && (length > mobileChunkBytes || (emptyChunk && p.Size != 0)) {
		deny(core.Fail(416, "DOWNLOAD_CHUNK_INVALID", "空分段只适用于空文件，每段最多读取1MiB"))
		return
	}
	if !gate.acquire(a) {
		w.Header().Set("Retry-After", "2")
		deny(core.Fail(429, "DOWNLOAD_BUSY", "当前下载正在读取，请稍后继续下一段"))
		return
	}
	defer gate.release(a)
	timeout, stopTimeout := context.WithTimeout(r.Context(), 2*time.Minute)
	defer stopTimeout()
	ctx, cancel := context.WithCancelCause(timeout)
	defer cancel(nil)
	p, readID, err := service.StartRead(ctx, a, p.ID, requestID(r), offset, length)
	if err != nil {
		deny(err)
		return
	}
	stage, code := "failed", "DOWNLOAD_INTERRUPTED"
	written := int64(0)
	defer func() {
		// Outcome auditing must also work after a client disconnect or revocation.
		audit, stop := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
		defer stop()
		if err := service.FinishRead(audit, a, p, readID, requestID(r), stage, code, offset, length, written); err != nil {
			s.Logger.Error("download outcome audit failed", "request_id", requestID(r), "read_id", readID, "bytes_written", written)
		}
	}()
	check := func() error {
		checkCtx, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer stop()
		current, err := s.actor(r.WithContext(checkCtx), false)
		if err == nil {
			_, err = service.Resolve(checkCtx, current, p.ID)
		}
		return err
	}
	watchDone := make(chan struct{})
	defer func() { cancel(nil); <-watchDone }()
	go func() {
		defer close(watchDone)
		tick := time.NewTicker(proxyRecheckInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := check(); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	var body io.ReadCloser
	if length > 0 {
		read, readErr := service.Backends[p.Provider].Reader.Read(ctx, p.Source, offset, length)
		if readErr != nil {
			code = downloadReadCode(ctx, readErr)
			deny(core.Fail(502, code, "固定文件内容暂时无法读取"))
			return
		}
		body = read.Body
		if body == nil || read.Size != length || read.ETag != p.Source.ETag || (p.Source.VersionID != "" && read.VersionID != p.Source.VersionID) {
			if body != nil {
				body.Close()
			}
			code = "DOWNLOAD_SOURCE_MISMATCH"
			deny(core.Fail(502, code, "存储返回的版本或范围与授权不符"))
			return
		}
		stopClose := context.AfterFunc(ctx, func() { body.Close() })
		defer stopClose()
		defer body.Close()
	}
	if err = check(); err != nil {
		code = core.PublicError(err).Code
		deny(err)
		return
	}
	// Pre-read at most 64 KiB. For the last chunk, verify exact EOF before
	// writing it, so oversized/truncated provider bodies cannot report success.
	buffer := make([]byte, proxyBufferBytes)
	first, err := readDownloadChunk(body, buffer, length)
	if err != nil {
		code = downloadReadCode(ctx, err)
		deny(core.Fail(502, code, "存储未返回完整的固定范围"))
		return
	}
	disposition, mediaType, err := (objectstore.ReadOptions{Filename: p.Filename, Disposition: p.Disposition, MediaType: p.MediaType}).Headers()
	if err != nil {
		code = "DOWNLOAD_METADATA_INVALID"
		deny(core.Fail(409, code, "下载元信息无效"))
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'self'")
	w.Header().Set("X-Accel-Buffering", "no")
	if !asChunk {
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Disposition", disposition)
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", etag)
		status := http.StatusOK
		if partial {
			status = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, p.Size))
		}
		w.WriteHeader(status)
	}
	var content []byte
	if asChunk {
		content = make([]byte, 0, length)
	}
	started := time.Now()
	chunk := first
	for written < length {
		if err = paceDownload(ctx, started, written+int64(len(chunk))); err != nil {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}
		// The API's ordinary 30-second write timeout becomes an idle timeout
		// for each bounded streaming write. Unsupported test recorders are safe.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
		var n int
		var writeErr error
		if asChunk {
			content = append(content, chunk...)
			n = len(chunk)
		} else {
			n, writeErr = w.Write(chunk)
		}
		written += int64(n)
		if writeErr != nil || n != len(chunk) {
			err = io.ErrShortWrite
			break
		}
		if written < length {
			chunk, err = readDownloadChunk(body, buffer, length-written)
			if err != nil {
				break
			}
		}
	}
	if asChunk {
		// No partial JSON/body escapes on truncation, revocation or cancellation.
		// Audited bytes count only a fully written envelope, never a private buffer.
		complete := err == nil && written == length
		written = 0
		if complete {
			if err = check(); err != nil {
				code = core.PublicError(err).Code
				deny(err)
				return
			}
		}
		if !complete {
			code = downloadReadCode(ctx, err)
			s.fail(w, r, core.Fail(502, code, "固定范围未完整读取，请重新核对权限后重试"))
			return
		}
		digest := sha256.Sum256(content)
		value := downloadChunk{p.ID, p.VersionID, strconv.FormatInt(offset, 10), strconv.FormatInt(length, 10), strconv.FormatInt(p.Size, 10), p.SHA256, fmt.Sprintf("%x", digest), content}
		encoded, encodeErr := json.Marshal(map[string]any{"data": value, "meta": newMeta(r)})
		if encodeErr == nil {
			encodeErr = s.Contract.Validate("DownloadChunkResponse", encoded)
		}
		if encodeErr != nil {
			code = "RESPONSE_CONTRACT_ERROR"
			s.fail(w, r, core.Fail(500, code, "分段响应未通过契约校验"))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		w.WriteHeader(http.StatusOK)
		n, writeErr := w.Write(encoded)
		if writeErr == nil && n == len(encoded) {
			written = length
			stage, code = "completed", "RANGE_WRITTEN"
		}
		return
	}
	if err == nil && written == length {
		stage, code = "completed", "RANGE_WRITTEN"
	} else {
		if written > 0 {
			stage = "partial"
		}
		code = downloadReadCode(ctx, err)
		// Headers already went out: never append a JSON error to file bytes.
		s.Logger.Info("download interrupted", "request_id", requestID(r), "read_id", readID, "bytes_written", written, "code", code)
	}
}
func readDownloadChunk(body io.Reader, buffer []byte, remaining int64) ([]byte, error) {
	if remaining == 0 {
		return buffer[:0], nil
	}
	size := int64(len(buffer))
	if remaining < size {
		size = remaining
	}
	if _, err := io.ReadFull(body, buffer[:size]); err != nil {
		return nil, err
	}
	if remaining == size {
		var extra [1]byte
		n, err := body.Read(extra[:])
		if n != 0 || err != io.EOF {
			return nil, errors.New("provider range length mismatch")
		}
	}
	return buffer[:size], nil
}
func paceDownload(ctx context.Context, started time.Time, total int64) error {
	target := time.Duration(total) * time.Second / proxyBytesPerSecond
	delay := time.Until(started.Add(target))
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func downloadReadCode(ctx context.Context, err error) string {
	if cause := context.Cause(ctx); cause != nil {
		if errors.Is(cause, context.DeadlineExceeded) {
			return "DOWNLOAD_DEADLINE_EXCEEDED"
		}
		if public, ok := cause.(*core.Error); ok {
			return public.Code
		}
		return "DOWNLOAD_CANCELLED"
	}
	if public, ok := err.(*objectstore.Error); ok {
		return public.Code
	}
	return "DOWNLOAD_STREAM_INCOMPLETE"
}
