package agent

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type Message struct {
	ID         string           `json:"id"`
	RunID      string           `json:"run_id"`
	Seq        int64            `json:"seq"`
	Role       string           `json:"role"`
	Body       string           `json:"body"`
	SourceRefs []SourceSnapshot `json:"source_refs"`
	Visibility string           `json:"visibility"`
	CreatedAt  time.Time        `json:"created_at"`
}
type RunEvent struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Seq       int64     `json:"seq"`
	Type      string    `json:"type"`
	State     string    `json:"state"`
	SafeCode  *string   `json:"safe_code"`
	CreatedAt time.Time `json:"created_at"`
}
type EventPage struct {
	Items    []RunEvent `json:"items"`
	AfterSeq int64      `json:"after_seq"`
	HasMore  bool       `json:"has_more"`
	Run      Run        `json:"run"`
}

func (s *Service) readableRun(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Run, Conversation, error) {
	r, err := runRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return r, Conversation{}, err
	}
	c, err := metadata(ctx, tx, a.OrgID, r.ConversationID, false)
	if err != nil {
		return r, c, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return r, c, err
	}
	if !slices.Contains(r.audience, a.UserID) {
		return r, c, core.Missing()
	}
	r.CanCancel = r.ActorID == a.UserID && a.RunID == "" && r.CancelRequestedAt == nil && slices.Contains([]string{"queued", "running", "awaiting_approval"}, r.State) && require(ctx, tx, a, c, "agent.run") == nil
	return r, c, nil
}
func snapshotsReadable(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, raw []SourceSnapshot) (bool, error) {
	refs := []SourceReference{}
	for _, snapshot := range raw {
		refs = append(refs, snapshot.SourceReference)
	}
	current, err := sourceSnapshots(ctx, tx, a, c, refs, []string{a.UserID}, false)
	if err != nil {
		status := core.PublicError(err).Status
		if status == 403 || status == 404 || status == 409 || status == 422 {
			return false, nil
		}
		return false, err
	}
	if len(current) != len(raw) {
		return false, nil
	}
	for i, item := range current {
		if item.SHA256 != raw[i].SHA256 || item.ResourceVersion != raw[i].ResourceVersion {
			return false, nil
		}
	}
	return true, nil
}
func (s *Service) GetRun(ctx context.Context, a core.Actor, id string) (Run, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	r, err := NotificationRun(ctx, tx, a, id)
	if err != nil {
		return Run{}, err
	}
	return r, platform.DatabaseError(tx.Commit(ctx))
}

// NotificationRun shares all current audience and source checks with GetRun.
// The caller owns the authorization snapshot and its commit.
func NotificationRun(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Run, error) {
	s := &Service{}
	r, c, err := s.readableRun(ctx, tx, a, id)
	if err != nil {
		return Run{}, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT source_snapshots FROM conversation_messages WHERE org_id=$1 AND run_id=$2 AND role='user'", a.OrgID, id).Scan(&raw); err != nil {
		return Run{}, platform.DatabaseError(err)
	}
	snapshots := []SourceSnapshot{}
	if err = json.Unmarshal(raw, &snapshots); err != nil {
		return Run{}, err
	}
	allowed, err := snapshotsReadable(ctx, tx, a, c, snapshots)
	if err != nil {
		return Run{}, err
	}
	if !allowed {
		return Run{}, core.Missing()
	}
	if b, e := readContractBody(ctx, tx, a.OrgID, id); e != nil {
		return Run{}, e
	} else if b != nil {
		view, e := contractBodyViewRead(ctx, tx, a, b)
		if e != nil {
			return Run{}, e
		}
		r.ContractBody = &view
	}
	if i, e := fileIntentRecordRead(ctx, tx, a.OrgID, id); e != nil {
		return Run{}, e
	} else if i != nil {
		view, e := fileUpdateViewRead(ctx, tx, i)
		if e != nil {
			return Run{}, e
		}
		assignFileIntent(&r, i, view)
	}
	if m, e := mutationRecordRead(ctx, tx, a.OrgID, id); e != nil {
		return Run{}, e
	} else if m != nil {
		r.Mutation = &m.MutationView
	}
	if i, e := readOARequestRecord(ctx, tx, a.OrgID, id); e != nil {
		return Run{}, e
	} else if i != nil {
		r.OARequest = &i.OARequestView
	}
	if p, e := readPostWrite(ctx, tx, a.OrgID, id); e != nil {
		return Run{}, e
	} else if p != nil {
		r.PostWrite = &p.PostWriteView
	}
	return r, nil
}
func (s *Service) Messages(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Message], error) {
	out := core.Page[Message]{Items: []Message{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "请指定1至100项消息")
	}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return out, err
	}
	bound := core.CursorBinding(a, "messages", id, c.CurrentRevisionID)
	var at *time.Time
	var after *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		after = &v.ID
	}
	rows, err := tx.Query(ctx, `SELECT id,run_id,seq,role,body,source_snapshots,created_at FROM conversation_messages
  WHERE org_id=$1 AND conversation_id=$2 AND $3=ANY(audience_ids) AND ($4::timestamptz IS NULL OR (created_at,id)<($4,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6`, a.OrgID, id, a.UserID, at, after, limit+1)
	if err != nil {
		return out, err
	}
	messages := []Message{}
	for rows.Next() {
		m := Message{SourceRefs: []SourceSnapshot{}, Visibility: "available"}
		var raw []byte
		if err = rows.Scan(&m.ID, &m.RunID, &m.Seq, &m.Role, &m.Body, &raw, &m.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(raw, &m.SourceRefs); err != nil {
			rows.Close()
			return out, err
		}
		messages = append(messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(messages) > limit {
		last := messages[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &next
		messages = messages[:limit]
	}
	for _, m := range messages {
		allowed, e := snapshotsReadable(ctx, tx, a, c, m.SourceRefs)
		if e != nil {
			return out, e
		}
		if !allowed {
			m.Body = ""
			m.SourceRefs = []SourceSnapshot{}
			m.Visibility = "unavailable"
		}
		out.Items = append(out.Items, m)
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) RunEvents(ctx context.Context, a core.Actor, id string, after int64, limit int) (EventPage, error) {
	out := EventPage{Items: []RunEvent{}, AfterSeq: after}
	if after < 0 || limit < 1 || limit > 100 {
		return out, core.Invalid("events", "事件序号与数量无效")
	}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	r, c, err := s.readableRun(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	// The event stream has the same source visibility as the run, without
	// opening another transaction or holding one across network writes.
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT source_snapshots FROM conversation_messages WHERE org_id=$1 AND run_id=$2 AND role='user'", a.OrgID, id).Scan(&raw); err != nil {
		return out, platform.DatabaseError(err)
	}
	snapshots := []SourceSnapshot{}
	if err = json.Unmarshal(raw, &snapshots); err != nil {
		return out, err
	}
	allowed, err := snapshotsReadable(ctx, tx, a, c, snapshots)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, core.Missing()
	}
	if after > r.LastEventSeq {
		return out, core.Invalid("after_seq", "事件序号超出当前任务")
	}
	rows, err := tx.Query(ctx, "SELECT id,run_id,seq,type,state,safe_code,created_at FROM agent_run_events WHERE org_id=$1 AND run_id=$2 AND seq>$3 ORDER BY seq LIMIT $4", a.OrgID, id, after, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var e RunEvent
		if err = rows.Scan(&e.ID, &e.RunID, &e.Seq, &e.Type, &e.State, &e.SafeCode, &e.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	if len(out.Items) > 0 {
		out.AfterSeq = out.Items[len(out.Items)-1].Seq
	}
	out.Run = r
	if b, e := readContractBody(ctx, tx, a.OrgID, id); e != nil {
		return out, e
	} else if b != nil {
		view, e := contractBodyViewRead(ctx, tx, a, b)
		if e != nil {
			return out, e
		}
		out.Run.ContractBody = &view
	}
	if i, e := fileIntentRecordRead(ctx, tx, a.OrgID, id); e != nil {
		return out, e
	} else if i != nil {
		view, e := fileUpdateViewRead(ctx, tx, i)
		if e != nil {
			return out, e
		}
		assignFileIntent(&out.Run, i, view)
	}
	if m, e := mutationRecordRead(ctx, tx, a.OrgID, id); e != nil {
		return out, e
	} else if m != nil {
		out.Run.Mutation = &m.MutationView
	}
	if i, e := readOARequestRecord(ctx, tx, a.OrgID, id); e != nil {
		return out, e
	} else if i != nil {
		out.Run.OARequest = &i.OARequestView
	}
	if p, e := readPostWrite(ctx, tx, a.OrgID, id); e != nil {
		return out, e
	} else if p != nil {
		out.Run.PostWrite = &p.PostWriteView
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
