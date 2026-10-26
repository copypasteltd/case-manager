package agent

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/servicestatus"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

func serviceDocument(ctx context.Context, tx pgx.Tx, org, id string) (contextDocument, error) {
	d := contextDocument{ID: strings.ToLower(id), Type: "service"}
	err := tx.QueryRow(ctx, `SELECT name,lifecycle,classification,creator_id,project_id,version,
 jsonb_build_object('name',name,'description',description,'owner_id',owner_id,'lifecycle',lifecycle)::text FROM services WHERE org_id=$1 AND id=$2`, org, id).
		Scan(&d.Title, &d.State, &d.Class, &d.Creator, &d.ProjectID, &d.ResourceVersion, &d.Body)
	return d, platform.DatabaseError(err)
}

func contextService(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d, err := serviceDocument(ctx, tx, a.OrgID, in.ID)
	if err != nil {
		return d, err
	}
	if err = resourceAudience(ctx, tx, a, c, d, members); err != nil {
		return d, err
	}
	if in.VersionID == nil {
		return d, nil
	}
	var conversation string
	var audience, related []string
	err = tx.QueryRow(ctx, `SELECT conversation_id,audience,related_service_ids,title,configuration_version,content_text FROM agent_service_reports WHERE org_id=$1 AND id=$2 AND service_id=$3`, a.OrgID, *in.VersionID, in.ID).
		Scan(&conversation, &audience, &related, &d.Title, &d.ResourceVersion, &d.Body)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	if conversation != c.ID {
		return d, core.Missing()
	}
	for _, u := range members {
		if !slices.Contains(audience, u) {
			return d, core.Missing()
		}
	}
	for _, id := range related {
		r, e := serviceDocument(ctx, tx, a.OrgID, id)
		if e != nil {
			return d, e
		}
		if e = resourceAudience(ctx, tx, a, c, r, members); e != nil {
			return d, e
		}
	}
	d.State = "snapshot"
	d.VersionID = in.VersionID
	return d, nil
}

func executionServiceSummary(ctx context.Context, tx pgx.Tx, st executionState, args json.RawMessage) (any, []SourceSnapshot, error) {
	var in struct {
		ServiceID string     `json:"service_id"`
		From      *time.Time `json:"from"`
		To        *time.Time `json:"to"`
	}
	if e := strictArguments(args, &in); e != nil {
		return nil, nil, e
	}
	in.ServiceID = strings.ToLower(in.ServiceID)
	if !core.ValidID(in.ServiceID) {
		return nil, nil, core.Invalid("service_id", "请选择准确服务")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	allow := func(r servicestatus.Record) error {
		return resourceAudience(ctx, tx, ai, st.Conversation, contextDocument{ID: r.ID, Type: "service", Class: r.Classification, Creator: r.CreatorID, ProjectID: r.ProjectID}, st.Run.audience)
	}
	content, related, err := servicestatus.AgentSummary(ctx, tx, ai, in.ServiceID, in.From, in.To, allow)
	if err != nil {
		return nil, nil, err
	}
	report := core.NewID()
	content["report_id"] = report
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > 90000 {
		return nil, nil, core.Invalid("service_id", "本次摘要超出受控内容上限，请缩小时间窗")
	}
	doc, err := serviceDocument(ctx, tx, ai.OrgID, in.ServiceID)
	if err != nil {
		return nil, nil, err
	}
	doc.State = "snapshot"
	doc.VersionID = &report
	doc.Body = string(raw)
	hash, err := documentDigest(doc)
	if err != nil {
		return nil, nil, err
	}
	slices.Sort(related)
	related = slices.Compact(related)
	_, err = tx.Exec(ctx, `INSERT INTO agent_service_reports(id,org_id,run_id,conversation_id,actor_id,service_id,audience,related_service_ids,title,configuration_version,content_text,document_sha256)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, report, ai.OrgID, st.Run.ID, st.Conversation.ID, ai.UserID, in.ServiceID, st.Run.audience, related, doc.Title, doc.ResourceVersion, string(raw), hash)
	if err != nil {
		return nil, nil, err
	}
	snapshot := SourceSnapshot{SourceReference: SourceReference{Type: "service", ID: doc.ID, VersionID: &report}, ResourceVersion: doc.ResourceVersion, SHA256: hash}
	return content, []SourceSnapshot{snapshot}, nil
}
