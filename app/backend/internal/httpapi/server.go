package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/contract"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/telegram"
	"copypaste.hk/platform/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	DB               *pgxpool.Pool
	Config           platform.Config
	Identity         *identity.Service
	Projects         *project.Service
	Contract         *contract.Validator
	Logger           *slog.Logger
	Storage          *files.UploadBackend
	Downloads        map[string]*files.DownloadBackend
	AgentFileReaders map[string]*files.AgentTextBackend
	AgentDraftWriter *files.AgentDraftBackend
	Telegram         *telegram.Service
}
type requestKey struct{}
type meta struct {
	RequestID  string    `json:"request_id"`
	ServerTime time.Time `json:"server_time"`
}

func requestID(r *http.Request) string { id, _ := r.Context().Value(requestKey{}).(string); return id }
func newMeta(r *http.Request) meta     { return meta{requestID(r), time.Now().UTC()} }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { s.write(w, 200, []byte(`{"status":"ok"}`)) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := migrations.CheckCompiled(ctx, s.DB); err != nil {
			s.fail(w, r, core.Fail(503, "NOT_READY", "服务正在准备"))
			return
		}
		s.write(w, 200, []byte(`{"status":"ready"}`))
	})
	s.authRoutes(mux)
	s.administrationRoutes(mux)
	s.projectRoutes(mux)
	s.knowledgeRoutes(mux)
	s.filesRoutes(mux)
	s.workflowRoutes(mux)
	s.partyRoutes(mux)
	s.contractRoutes(mux)
	s.spendRoutes(mux)
	s.costRoutes(mux)
	s.paymentRoutes(mux)
	s.subscriptionRoutes(mux)
	s.requestRoutes(mux)
	s.conversationRoutes(mux)
	s.agentRunRoutes(mux)
	s.agentPlanRoutes(mux)
	s.contractFinanceRoutes(mux)
	s.contractClosureRoutes(mux)
	s.serviceStatusRoutes(mux)
	s.notificationRoutes(mux)
	s.telegramRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, core.Missing()) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, core.NewID()))
		w.Header().Set("X-Request-ID", requestID(r))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				s.Logger.Error("request panic", "request_id", requestID(r))
				s.fail(w, r, core.Fail(500, "INTERNAL_ERROR", "请求未能完成"))
			}
			s.Logger.Info("request", "request_id", requestID(r), "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
		}()
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if !s.Config.AllowedOrigins[origin] {
				s.fail(w, r, core.Forbidden())
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Operation-ID, X-Idempotent-Replay, Content-Range, Content-Length, Accept-Ranges, ETag, Content-Disposition")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				s.fail(w, r, core.Forbidden())
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, Idempotency-Key, Last-Event-ID, Range, If-Range")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) write(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
func (s *Server) respond(w http.ResponseWriter, r *http.Request, status int, schema string, data any) {
	body, err := json.Marshal(map[string]any{"data": data, "meta": newMeta(r)})
	if err == nil {
		err = s.Contract.Validate(schema, body)
	}
	if err != nil {
		s.Logger.Error("response contract violated", "request_id", requestID(r), "schema", schema)
		s.fail(w, r, core.Fail(500, "RESPONSE_CONTRACT_ERROR", "响应未通过契约校验，请保留请求编号"))
		return
	}
	s.write(w, status, body)
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		return
	}
	public := core.PublicError(platform.DatabaseError(err))
	if public.Status >= 500 {
		fields := []any{"request_id", requestID(r), "code", public.Code, "error_type", fmt.Sprintf("%T", err)}
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			fields = append(fields, "database_code", databaseError.Code, "constraint", databaseError.ConstraintName)
		}
		s.Logger.Error("request failed", fields...)
	}
	body, _ := json.Marshal(map[string]any{"error": public, "meta": newMeta(r)})
	s.write(w, public.Status, body)
}
func decode[T any](s *Server, w http.ResponseWriter, r *http.Request, schema string) (T, []byte, error) {
	var input T
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return input, nil, core.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "请发送 application/json 请求")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return input, nil, core.Fail(413, "BODY_TOO_LARGE", "业务请求超过大小限制；文件应直接上传至对象存储")
		}
		return input, nil, core.Invalid("body", "无法读取请求")
	}
	if err = s.Contract.Validate(schema, body); err != nil {
		return input, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&input); err != nil {
		return input, nil, core.Invalid("body", "请求字段格式无效")
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return input, nil, core.Invalid("body", "请求只能包含一个 JSON 对象")
	}
	return input, body, nil
}
func (s *Server) actor(r *http.Request, write bool) (core.Actor, error) {
	var token string
	cookieAuth := false
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		scheme, value, ok := strings.Cut(authorization, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.Contains(value, " ") {
			return core.Actor{}, core.Fail(401, "UNAUTHENTICATED", "登录凭证格式无效")
		}
		token = value
		// Do not silently accept mixed credentials or bypass CSRF on a cookie session.
		if _, err := r.Cookie("cp_session"); err == nil {
			return core.Actor{}, core.Fail(400, "MIXED_AUTHENTICATION", "一次请求只能使用一种会话凭证")
		}
	} else if cookie, err := r.Cookie("cp_session"); err == nil {
		token = cookie.Value
		cookieAuth = true
	}
	actor, err := s.Identity.Authenticate(r.Context(), token)
	if err != nil {
		return actor, err
	}
	if cookieAuth && actor.Channel != "web" {
		return actor, core.Forbidden()
	}
	if !cookieAuth && actor.Channel == "web" {
		return actor, core.Forbidden()
	}
	if cookieAuth && write {
		if !s.Config.AllowedOrigins[r.Header.Get("Origin")] {
			return actor, core.Forbidden()
		}
		if err = s.Identity.CSRF(r.Context(), actor.SessionID, r.Header.Get("X-CSRF-Token")); err != nil {
			return actor, err
		}
	}
	return actor, nil
}
func (s *Server) denyAudit(r *http.Request, a core.Actor, err error) {
	if a.UserID == "" {
		return
	}
	public := core.PublicError(platform.DatabaseError(err))
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	_, auditErr := s.DB.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,result,reason)
        VALUES($1,$2,$3,$3,$4,$5,'http.rejected','request','denied',$6)`,
		core.NewID(), a.OrgID, a.UserID, a.Channel, requestID(r), public.Code)
	if auditErr != nil {
		s.Logger.Error("denial audit failed", "request_id", requestID(r))
	}
}
func command[T any](s *Server, schema, response string, fn func(context.Context, core.Command, *http.Request, T) (core.Result, error), afterSuccess ...func(http.ResponseWriter, *http.Request, core.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, true)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		input, body, err := decode[T](s, w, r, schema)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		c := core.Command{Actor: a, RequestID: requestID(r), Route: r.Method + " " + r.URL.Path, Key: r.Header.Get("Idempotency-Key"), Body: body}
		result, err := fn(r.Context(), c, r, input)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		w.Header().Set("X-Operation-ID", result.OperationID)
		if result.Replayed {
			w.Header().Set("X-Idempotent-Replay", "true")
		}
		for _, after := range afterSuccess {
			after(w, r, a)
		}
		s.respond(w, r, result.Status, response, result.Data)
	}
}
func read(s *Server, response string, fn func(context.Context, core.Actor, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, false)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data, err := fn(r.Context(), a, r)
		if err != nil {
			s.denyAudit(r, a, err)
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, response, data)
	}
}
func filter(r *http.Request) (project.Filter, error) {
	f := project.Filter{State: r.URL.Query().Get("state"), Query: r.URL.Query().Get("q"), Cursor: r.URL.Query().Get("cursor"), Limit: 20}
	if n := r.URL.Query().Get("limit"); n != "" {
		v, err := strconv.Atoi(n)
		if err != nil || v < 1 || v > 100 {
			return f, core.Invalid("limit", "分页数量需为1至100")
		}
		f.Limit = v
	}
	if len([]rune(f.Query)) > 200 || len(f.Cursor) > 2048 {
		return f, core.Invalid("query", "查询参数过长")
	}
	return f, nil
}
func clientIP(r *http.Request) string {
	// X-Forwarded-For is not trusted unless a dedicated ingress adapter verifies it.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
