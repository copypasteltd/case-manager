package agentworker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
)

const RuntimeVersion = "codex-sdk-0.164.0"
const MCPPath = "/internal/v1/agent-mcp"
const ControlPath = "/internal/v1/agent-control/"

type NativeProfile struct {
	SDKVersion          string   `json:"sdk_version"`
	ProfileID           string   `json:"profile_id"`
	Model               string   `json:"model"`
	WorkingDirectory    string   `json:"working_directory"`
	ConfigurationSHA256 string   `json:"configuration_sha256"`
	ExpectedTools       []string `json:"expected_tools"`
}
type ConfinementProof struct {
	SHA256    string
	ExpiresAt time.Time
}
type NativeVerifier interface {
	Verify(context.Context, agent.ExecutionLease, NativeProfile) (ConfinementProof, error)
}
type Bridge struct {
	Service                          *agent.Service
	Verifier                         NativeVerifier
	Address, Model, WorkingDirectory string
}

func decode(w http.ResponseWriter, r *http.Request, out any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return core.Invalid("body", "请求体过大或无效")
	}
	return decodeRaw(raw, out)
}
func decodeRaw(raw []byte, out any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return core.Invalid("body", "需要一个请求对象")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return core.Invalid("body", "请求格式无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return core.Invalid("body", "只允许一个请求对象")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	p := core.PublicError(err)
	writeJSON(w, p.Status, map[string]any{"error": map[string]any{"code": p.Code}})
}
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || r.Host != b.Address || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		fail(w, core.Fail(403, "RUNTIME_ORIGIN_DENIED", "本地受控入口拒绝此来源"))
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(405)
		return
	}
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, core.Fail(415, "RUNTIME_PROTOCOL_INVALID", "需要 JSON"))
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		fail(w, core.Fail(401, "RUNTIME_CREDENTIAL_INVALID", "需要任务凭证"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	lease, err := b.Service.LeaseForCredential(ctx, strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		fail(w, core.Fail(401, "RUNTIME_CREDENTIAL_INVALID", "任务凭证无效"))
		return
	}
	if r.URL.Path == MCPPath {
		b.mcp(ctx, w, r, lease)
		return
	}
	switch r.URL.Path {
	case ControlPath + "current", ControlPath + "receipts":
		var empty struct{}
		if err = decode(w, r, &empty); err == nil {
			if r.URL.Path == ControlPath+"current" {
				err = b.Service.CheckExecution(ctx, lease)
			} else {
				var receipts []agent.ExecutionReceipt
				receipts, err = b.Service.ExecutionReceipts(ctx, lease)
				if err == nil {
					writeJSON(w, 200, map[string]any{"receipts": receipts})
					return
				}
			}
		}
	case ControlPath + "confinement":
		var in NativeProfile
		if err = decode(w, r, &in); err == nil {
			var profile agent.RuntimeProfile
			var channel string
			profile, channel, err = b.Service.ExecutionScope(ctx, lease)
			expected := []string{"company/read_resource", "company/search_resources", "company/inspect_operation"}
			if profile.SkillID == "project-briefing" && channel != "telegram" {
				expected = append(expected, "company/draft_post", "company/publish_post")
			}
			if profile.SkillID == "subscription-review" {
				expected = []string{"company/read_resource", "company/read_finance_summary", "company/list_subscriptions", "company/read_subscription_review", "company/read_finance_record", "company/inspect_operation"}
			}
			if profile.SkillID == "incident-summary" {
				expected = append(expected, "company/read_service_summary")
			}
			if profile.SkillID == "project-create" {
				expected = append(expected, "company/propose_project", "company/create_project")
			}
			if profile.SkillID == "project-update" {
				expected = []string{"company/read_resource", "company/inspect_operation", "company/create_risk", "company/update_risk", "company/create_issue", "company/update_issue", "company/create_deliverable", "company/update_deliverable", "company/create_milestone", "company/update_project", "company/update_task", "company/transition_task", "company/transition_project", "company/update_milestone", "company/transition_milestone"}
			}
			if profile.SkillID == "file-update" {
				expected = []string{"company/read_resource", "company/inspect_operation", "company/create_file_draft", "company/propose_file_update", "company/commit_file_update", "company/stage_file_create", "company/propose_file_create", "company/commit_file_create"}
			}
			if profile.SkillID == "information-organize" {
				expected = []string{"company/read_resource", "company/inspect_operation", "company/list_directory", "company/propose_organization", "company/commit_organization"}
			}
			if profile.SkillID == "oa-assist" {
				expected = []string{"company/read_resource", "company/inspect_operation", "company/draft_request", "company/submit_request"}
			}
			if err == nil && (profile.RuntimeVersion != RuntimeVersion || (profile.SkillID != "project-briefing" && profile.SkillID != "project-create" && profile.SkillID != "project-update" && profile.SkillID != "file-update" && profile.SkillID != "information-organize" && profile.SkillID != "oa-assist" && profile.SkillID != "incident-summary" && profile.SkillID != "subscription-review") || in.SDKVersion != "0.164.0" || in.ProfileID != profile.ID || in.ConfigurationSHA256 != profile.ConfigurationSHA256 || in.Model != b.Model || in.WorkingDirectory != b.WorkingDirectory || !sameTools(expected, in.ExpectedTools) || b.Verifier == nil) {
				err = core.Fail(409, "RUNTIME_CONFINEMENT_UNVERIFIED", "执行环境尚未验证")
			}
			if err == nil {
				var proof ConfinementProof
				proof, err = b.Verifier.Verify(ctx, lease, in)
				if err == nil {
					err = b.Service.RecordVerifiedConfinement(ctx, lease, in.ConfigurationSHA256, proof.SHA256, proof.ExpiresAt)
				}
			}
		}
	case ControlPath + "events":
		var in struct {
			Type                   string                `json:"type"`
			Tool                   string                `json:"tool,omitempty"`
			Code                   string                `json:"code,omitempty"`
			ReconciliationRequired bool                  `json:"reconciliation_required,omitempty"`
			Result                 *agent.ProposedResult `json:"result,omitempty"`
			Usage                  *agent.ExecutionUsage `json:"usage,omitempty"`
		}
		if err = decode(w, r, &in); err == nil {
			switch in.Type {
			case "run.started":
				if in.Tool != "" || in.Code != "" || in.Result != nil || in.Usage != nil || in.ReconciliationRequired {
					err = core.Invalid("event", "事件字段不匹配")
				} else {
					err = b.Service.StartExecution(ctx, lease)
				}
			case "run.completed":
				if in.Result == nil || in.Usage == nil || in.Tool != "" || in.Code != "" || in.ReconciliationRequired {
					err = core.Invalid("event", "结果与用量必须准确提供")
				} else {
					err = b.Service.CompleteExecution(ctx, lease, *in.Result, *in.Usage)
				}
			case "tool.started", "tool.finished", "run.interrupted":
				if in.Result != nil || in.Usage != nil {
					err = core.Invalid("event", "观察事件不接受回复正文")
				} else {
					err = b.Service.ObserveExecution(ctx, lease, in.Type, in.Tool, in.Code)
				}
			default:
				err = core.Invalid("type", "事件不受支持")
			}
		}
	default:
		err = core.Missing()
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"accepted": true})
}
func sameTools(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
