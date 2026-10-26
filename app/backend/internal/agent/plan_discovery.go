package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func executionPlanScope(ctx context.Context, tx pgx.Tx, st executionState) error {
	if st.Profile.SkillID != "project-create" {
		return runtimeFailure("SKILL_TOOL_DENIED")
	}
	c := st.Conversation
	if c.Type != "private" || c.ProjectID != nil || c.CreatorID != st.Actor.UserID {
		return core.Missing()
	}
	return authz.RequireOrganization(ctx, tx, st.Actor, "agent.plan.read")
}

// Read immutable intent separately from the live status. A status observation is
// never an approval; the commit path always checks current state again.
func executionPlanText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, limit int) (any, []SourceSnapshot, error) {
	if err := executionPlanScope(ctx, tx, st); err != nil {
		return nil, nil, err
	}
	if !core.ValidID(id) || version != nil || offset < 0 || offset > 200000 || limit < 1 || limit > 10000 {
		return nil, nil, core.Invalid("arguments", "请指定准确计划和有限字符范围")
	}
	p, src, err := executionPlan(ctx, tx, st, id)
	if err != nil {
		return nil, nil, err
	}
	var text string
	var observed time.Time
	if err = tx.QueryRow(ctx, "SELECT intent,clock_timestamp() FROM agent_plans WHERE org_id=$1 AND id=$2", st.Actor.OrgID, id).Scan(&text, &observed); err != nil {
		return nil, nil, err
	}
	if core.Digest([]byte(text)) != src.SHA256 {
		return nil, nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	body := []rune(text)
	if len(body) > 200000 || offset > len(body) {
		return nil, nil, core.Invalid("offset", "计划内容或字符范围超出受控读取上限")
	}
	end := min(offset+limit, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	return map[string]any{"type": "agent_plan", "id": p.ID, "version_id": nil, "resource_version": 1, "title": p.Project.Name, "state": p.State, "plan_version": p.Version, "plan_hash": p.PlanHash, "blocked_reason": p.BlockedReason, "expires_at": p.ExpiresAt, "observed_at": observed, "text": string(body[offset:end]), "offset": offset, "total_chars": len(body), "next_offset": next, "source_sha256": src.SHA256, "untrusted_data": true}, []SourceSnapshot{src}, nil
}

type planSearchCursor struct {
	ID      string `json:"id"`
	Binding string `json:"binding"`
}

func executionPlanSearch(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, query string, kinds, states []string, cursor string, limit int) (any, []SourceSnapshot, error) {
	if err := executionPlanScope(ctx, tx, st); err != nil {
		return nil, nil, err
	}
	if len(kinds) != 1 || kinds[0] != "agent_plan" || utf8.RuneCountInString(query) > 200 || limit < 1 || limit > 20 || len(states) > 4 {
		return nil, nil, core.Invalid("arguments", "项目计划必须单独查询，最多返回20项")
	}
	for _, state := range states {
		if !slices.Contains([]string{"proposed", "approved", "committed", "revoked"}, state) {
			return nil, nil, core.Invalid("plan_states", "计划状态无效")
		}
	}
	if states == nil {
		states = []string{}
	}
	slices.Sort(states)
	states = slices.Compact(states)
	bound, _ := json.Marshal([]any{lease.RunID, lease.AuthorityStamp, st.Conversation.ID, query, states, limit})
	binding := core.Digest(bound)
	if lease.credentialSHA256 == "" {
		return nil, nil, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	key := []byte(lease.credentialSHA256)
	after := planSearchCursor{Binding: binding}
	if cursor != "" {
		parts := strings.Split(cursor, ".")
		if len(parts) != 2 || len(cursor) > 2048 {
			return nil, nil, core.Invalid("cursor", "计划查询位置无效")
		}
		raw, e1 := base64.RawURLEncoding.DecodeString(parts[0])
		sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
		mac := hmac.New(sha256.New, key)
		mac.Write(raw)
		if e1 != nil || e2 != nil || !hmac.Equal(mac.Sum(nil), sig) || json.Unmarshal(raw, &after) != nil || after.Binding != binding || !core.ValidID(after.ID) {
			return nil, nil, core.Invalid("cursor", "计划查询位置与当前任务或筛选不一致")
		}
	}
	var afterID any
	if after.ID != "" {
		afterID = after.ID
	}
	rows, err := tx.Query(ctx, `SELECT id FROM agent_plans WHERE org_id=$1 AND conversation_id=$2 AND actor_id=$3
 AND ($4='' OR strpos(lower(payload->>'name'),lower($4))>0 OR id::text=$4)
 AND (cardinality($5::text[])=0 OR state=ANY($5::text[])) AND ($6::uuid IS NULL OR id>$6::uuid) ORDER BY id LIMIT 10001`, st.Actor.OrgID, st.Conversation.ID, st.Actor.UserID, query, states, afterID)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(ids) > 10000 {
		return nil, nil, runtimeFailure("RUNTIME_SEARCH_LIMIT")
	}
	var observed time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&observed); err != nil {
		return nil, nil, err
	}
	items := []map[string]any{}
	sources := []SourceSnapshot{}
	last := ""
	more := false
	for _, id := range ids {
		p, src, e := executionPlan(ctx, tx, st, id)
		if e != nil {
			status := core.PublicError(e).Status
			if status == 403 || status == 404 || status == 409 {
				continue
			}
			return nil, nil, e
		}
		if len(items) == limit {
			more = true
			break
		}
		items = append(items, map[string]any{"type": "agent_plan", "id": id, "title": p.Project.Name, "state": p.State, "resource_version": 1, "plan_version": p.Version, "plan_hash": p.PlanHash, "source_sha256": src.SHA256, "expires_at": p.ExpiresAt, "blocked_reason": p.BlockedReason, "untrusted_data": true})
		sources = append(sources, src)
		last = id
	}
	var next *string
	if more {
		raw, _ := json.Marshal(planSearchCursor{ID: last, Binding: binding})
		mac := hmac.New(sha256.New, key)
		mac.Write(raw)
		value := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		next = &value
	}
	return map[string]any{"items": items, "next_cursor": next, "observed_at": observed, "untrusted_data": true}, sources, nil
}
