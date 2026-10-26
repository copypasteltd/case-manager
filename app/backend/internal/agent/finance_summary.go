package agent

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
)

func contextFinanceReport(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d := contextDocument{Type: "agent_finance_report", ID: in.ID, State: "snapshot", ResourceVersion: 1}
	if in.VersionID != nil {
		return d, core.Missing()
	}
	var conversation string
	var audience []string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT conversation_id,actor_id,project_id,classification,title,content_text,evidence,audience FROM agent_finance_reports WHERE org_id=$1 AND id=$2`, a.OrgID, in.ID).
		Scan(&conversation, &d.Creator, &d.ProjectID, &d.Class, &d.Title, &d.Body, &raw, &audience)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	if conversation != c.ID {
		return d, core.Missing()
	}
	for _, user := range members {
		if !slices.Contains(audience, user) {
			return d, core.Missing()
		}
	}
	if err = resourceAudience(ctx, tx, a, c, d, members); err != nil {
		return d, err
	}
	var refs []finance.AgentSummaryEvidence
	if err = json.Unmarshal(raw, &refs); err != nil {
		return d, err
	}
	if err = finance.ValidateAgentSummary(ctx, tx, a, d.ProjectID, d.Class, members, refs); err != nil {
		return d, err
	}
	return d, nil
}

func executionFinanceSummary(ctx context.Context, tx pgx.Tx, st executionState, args json.RawMessage) (any, []SourceSnapshot, error) {
	var in struct {
		Month    string `json:"month"`
		View     string `json:"view"`
		Currency string `json:"currency"`
	}
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	a := st.Actor
	a.RunID = st.Run.ID
	content, refs, err := finance.AgentSummary(ctx, tx, a, st.Conversation.ProjectID, st.Conversation.Classification, st.Run.audience, in.Month, in.View, in.Currency)
	if err != nil {
		return nil, nil, err
	}
	labels := map[string]string{"forecast": "订阅预测", "actual": "发生支出", "cash": "现金收付"}
	return saveFinanceSnapshot(ctx, tx, st, in.Month+" · "+labels[in.View], content, refs)
}

func (s *Service) executionSubscriptionReview(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (any, []SourceSnapshot, error) {
	a := st.Actor
	a.RunID = st.Run.ID
	scope := finance.ReviewScope{ProjectID: st.Conversation.ProjectID, Classification: st.Conversation.Classification, ConversationID: st.Conversation.ID, Audience: st.Run.audience}
	var content map[string]any
	var refs []finance.AgentSummaryEvidence
	var err error
	switch tool {
	case "list_subscriptions":
		var in finance.SubscriptionDiscovery
		if err = strictArguments(args, &in); err == nil {
			content, refs, err = finance.AgentSubscriptions(ctx, tx, s.Pager, a, scope, in)
		}
	case "read_subscription_review":
		var in finance.SubscriptionReviewInput
		if err = strictArguments(args, &in); err == nil {
			content, refs, err = finance.AgentSubscriptionReview(ctx, tx, a, scope, in)
		}
	case "read_finance_record":
		var in finance.FinanceRecordInput
		if err = strictArguments(args, &in); err == nil {
			content, refs, err = finance.AgentFinanceRecord(ctx, tx, a, scope, in)
		}
	default:
		return nil, nil, core.Forbidden()
	}
	if err != nil {
		return nil, nil, err
	}
	labels := map[string]string{"list_subscriptions": "订阅发现页", "read_subscription_review": "订阅逐笔核对", "read_finance_record": "关联财务原记录"}
	return saveFinanceSnapshot(ctx, tx, st, labels[tool], content, refs)
}

func saveFinanceSnapshot(ctx context.Context, tx pgx.Tx, st executionState, title string, content map[string]any, refs []finance.AgentSummaryEvidence) (any, []SourceSnapshot, error) {
	a := st.Actor
	id := core.NewID()
	content["report_id"] = id
	d := contextDocument{Type: "agent_finance_report", ID: id, Title: title, State: "snapshot", ResourceVersion: 1, Class: st.Conversation.Classification, Creator: a.UserID, ProjectID: st.Conversation.ProjectID}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > 90000 {
		return nil, nil, core.Invalid("scope", "财务快照超出受控上限")
	}
	d.Body = string(raw)
	hash, err := documentDigest(d)
	if err != nil {
		return nil, nil, err
	}
	evidence, err := json.Marshal(refs)
	if err != nil {
		return nil, nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_finance_reports(id,org_id,run_id,conversation_id,actor_id,project_id,classification,audience,title,content_text,evidence,document_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, a.OrgID, st.Run.ID, st.Conversation.ID, a.UserID, d.ProjectID, d.Class, st.Run.audience, d.Title, d.Body, evidence, hash)
	if err != nil {
		return nil, nil, err
	}
	return content, []SourceSnapshot{{SourceReference: SourceReference{Type: d.Type, ID: id}, ResourceVersion: 1, SHA256: hash}}, nil
}
