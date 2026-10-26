package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB         *pgxpool.Pool
	Commands   platform.Commands
	Pager      platform.Pager
	FileText   *files.AgentTextService
	FileDrafts *files.AgentDraftBackend
}
type Member struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
	Creator     bool   `json:"is_creator"`
}
type Conversation struct {
	ID                     string    `json:"id"`
	Version                int64     `json:"version"`
	Type                   string    `json:"type"`
	ProjectID              *string   `json:"project_id"`
	Classification         string    `json:"classification"`
	CreatorID              string    `json:"creator_id"`
	CurrentRevisionID      string    `json:"current_revision_id"`
	Title                  string    `json:"title"`
	State                  string    `json:"state"`
	ContextSecurityVersion int64     `json:"context_security_version"`
	Members                []Member  `json:"members"`
	CanEdit                bool      `json:"can_edit"`
	CanManageMembers       bool      `json:"can_manage_members"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
type ConversationCreate struct {
	Type      string   `json:"type"`
	ProjectID *string  `json:"project_id"`
	Title     string   `json:"title"`
	MemberIDs []string `json:"member_ids"`
}
type ConversationPatch struct {
	ExpectedVersion int64  `json:"expected_version"`
	Title           string `json:"title"`
	State           string `json:"state"`
	Reason          string `json:"reason"`
}
type MemberChange struct {
	ExpectedVersion int64  `json:"expected_version"`
	UserID          string `json:"user_id"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type AccessResult struct {
	ConversationID         string `json:"conversation_id"`
	Version                int64  `json:"version"`
	ContextSecurityVersion int64  `json:"context_security_version"`
	AccessLost             bool   `json:"access_lost"`
}
type Filter struct {
	Type, State, Query, Cursor string
	ProjectID                  *string
	Limit                      int
}

const columns = `c.id,c.version,c.type,c.project_id,c.classification,c.creator_id,c.current_revision_id,
    r.title,r.state,r.context_security_version,c.created_at,c.updated_at`
const currentJoin = ` FROM conversations c JOIN conversation_revisions r ON r.org_id=c.org_id AND r.conversation_id=c.id AND r.id=c.current_revision_id `
const visibleSQL = `EXISTS(SELECT 1 FROM conversation_members cm WHERE cm.org_id=c.org_id AND cm.revision_id=c.current_revision_id AND cm.user_id=$2)
    AND can_access(c.org_id,$2,'conversation.read','conversation',c.id,c.project_id,c.creator_id,c.classification,$3,$4)
    AND (c.project_id IS NULL OR EXISTS(SELECT 1 FROM projects p WHERE p.org_id=c.org_id AND p.id=c.project_id
        AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.org_id=p.org_id AND pm.project_id=p.id AND pm.user_id=$2)
        AND can_access(p.org_id,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,$4)))`

func scan(row pgx.Row) (Conversation, error) {
	var c Conversation
	err := row.Scan(&c.ID, &c.Version, &c.Type, &c.ProjectID, &c.Classification, &c.CreatorID, &c.CurrentRevisionID,
		&c.Title, &c.State, &c.ContextSecurityVersion, &c.CreatedAt, &c.UpdatedAt)
	return c, platform.DatabaseError(err)
}
func metadata(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Conversation, error) {
	if !core.ValidID(id) {
		return Conversation{}, core.Missing()
	}
	if lock {
		// Lock the head before joining its immutable revision. At READ COMMITTED,
		// a joined SELECT FOR UPDATE can follow a concurrently replaced head
		// while retaining its old statement snapshot for the revision, yielding
		// no row instead of the expected version conflict.
		var locked string
		if err := db.QueryRow(ctx, "SELECT id FROM conversations WHERE org_id=$1 AND id=$2 FOR UPDATE", org, id).Scan(&locked); err != nil {
			return Conversation{}, platform.DatabaseError(err)
		}
	}
	query := "SELECT " + columns + currentJoin + "WHERE c.org_id=$1 AND c.id=$2"
	return scan(db.QueryRow(ctx, query, org, id))
}
func resource(c Conversation) authz.Resource {
	return authz.Resource{Type: "conversation", ID: c.ID, ProjectID: c.ProjectID, OwnerID: &c.CreatorID, Classification: c.Classification}
}
func require(ctx context.Context, db platform.DBTX, a core.Actor, c Conversation, action string) error {
	var allowed bool
	err := db.QueryRow(ctx, "SELECT "+visibleSQL+" FROM conversations c WHERE c.org_id=$1 AND c.id=$5", a.OrgID, a.UserID, a.Channel, a.RunID != "", c.ID).Scan(&allowed)
	if err != nil {
		return platform.DatabaseError(err)
	}
	if !allowed {
		return core.Missing()
	}
	return authz.Require(ctx, db, a, action, resource(c))
}
func memberIDs(ctx context.Context, db platform.DBTX, org string, c Conversation) ([]string, error) {
	rows, err := db.Query(ctx, "SELECT user_id FROM conversation_members WHERE org_id=$1 AND revision_id=$2 ORDER BY user_id", org, c.CurrentRevisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func present(ctx context.Context, db platform.DBTX, a core.Actor, c Conversation) (Conversation, error) {
	if err := require(ctx, db, a, c, "conversation.read"); err != nil {
		return Conversation{}, err
	}
	c.CanEdit = c.Type != "telegram" && a.RunID == "" && authz.Require(ctx, db, a, "conversation.update", resource(c)) == nil
	c.CanManageMembers = a.RunID == "" && c.Type == "project" && c.State == "active" && authz.Require(ctx, db, a, "conversation.manage_members", resource(c)) == nil
	c.Members = []Member{}
	rows, err := db.Query(ctx, `SELECT u.id,u.display_name,u.status='active'
        AND can_access(c.org_id,u.id,'conversation.read','conversation',c.id,c.project_id,c.creator_id,c.classification,$3,false)
        AND (c.project_id IS NULL OR EXISTS(SELECT 1 FROM projects p WHERE p.org_id=c.org_id AND p.id=c.project_id
            AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.org_id=p.org_id AND pm.project_id=p.id AND pm.user_id=u.id)
            AND can_access(p.org_id,u.id,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,false))),u.id=c.creator_id
        FROM conversations c JOIN conversation_members cm ON cm.org_id=c.org_id AND cm.revision_id=c.current_revision_id
        JOIN users u ON u.org_id=cm.org_id AND u.id=cm.user_id WHERE c.org_id=$1 AND c.id=$2 ORDER BY u.id`, a.OrgID, c.ID, a.Channel)
	if err != nil {
		return Conversation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.DisplayName, &m.Available, &m.Creator); err != nil {
			return Conversation{}, err
		}
		c.Members = append(c.Members, m)
	}
	return c, rows.Err()
}

// A response must not combine one revision's title/security epoch with another
// revision's members. The organization lock also orders reads against every
// supported authorization mutation; the snapshot covers metadata-only edits.
func (s *Service) readSnapshot(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	var organizationVersion, userVersion int64
	var status string
	err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&organizationVersion)
	if err == nil {
		err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, platform.DatabaseError(err)
	}
	if status != "active" || organizationVersion != a.OrgAuthVersion || userVersion != a.AuthVersion {
		tx.Rollback(ctx)
		return nil, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	return tx, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Conversation, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return Conversation{}, err
	}
	defer tx.Rollback(ctx)
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return Conversation{}, err
	}
	c, err = present(ctx, tx, a, c)
	if err == nil {
		err = platform.DatabaseError(tx.Commit(ctx))
	}
	return c, err
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Conversation], error) {
	out := core.Page[Conversation]{Items: []Conversation{}}
	if !slices.Contains([]string{"", "private", "project", "telegram"}, f.Type) || !slices.Contains([]string{"", "active", "archived"}, f.State) ||
		len([]rune(f.Query)) > 200 || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) {
		return out, core.Invalid("filter", "会话筛选无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	project := ""
	if f.ProjectID != nil {
		project = *f.ProjectID
	}
	bound := core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/conversations/", f.Type, "/", f.State, "/", f.Query, "/", project)))
	var at *time.Time
	var after *string
	if f.Cursor != "" {
		cursor, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &cursor.UpdatedAt
		after = &cursor.ID
	}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT "+columns+currentJoin+`WHERE c.org_id=$1 AND `+visibleSQL+`
        AND ($5='' OR c.type=$5) AND ($6='' OR r.state=$6) AND ($7='' OR position(lower($7) in lower(r.title))>0)
        AND ($8::uuid IS NULL OR c.project_id=$8) AND ($9::timestamptz IS NULL OR (c.updated_at,c.id)<($9,$10::uuid))
        ORDER BY c.updated_at DESC,c.id DESC LIMIT $11`, a.OrgID, a.UserID, a.Channel, a.RunID != "", f.Type, f.State, f.Query, f.ProjectID, at, after, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		c, e := scan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		last := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &cursor
		out.Items = out.Items[:f.Limit]
	}
	for i, c := range out.Items {
		out.Items[i], err = present(ctx, tx, a, c)
		if err != nil {
			return core.Page[Conversation]{Items: []Conversation{}}, err
		}
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}

func projectScope(ctx context.Context, db platform.DBTX, a core.Actor, project *string) (string, error) {
	if project == nil {
		return "internal", nil
	}
	if !core.ValidID(*project) {
		return "", core.Invalid("project_id", "请选择项目")
	}
	var class, owner string
	var member bool
	err := db.QueryRow(ctx, `SELECT p.classification,p.creator_id,EXISTS(SELECT 1 FROM project_members pm
        WHERE pm.org_id=p.org_id AND pm.project_id=p.id AND pm.user_id=$3) FROM projects p WHERE p.org_id=$1 AND p.id=$2`, a.OrgID, *project, a.UserID).Scan(&class, &owner, &member)
	if err != nil {
		return "", platform.DatabaseError(err)
	}
	if !member {
		return "", core.Missing()
	}
	err = authz.Require(ctx, db, a, "project.read", authz.Resource{Type: "project", ID: *project, ProjectID: project, OwnerID: &owner, Classification: class})
	return class, err
}
func eligible(ctx context.Context, db platform.DBTX, a core.Actor, c Conversation, user string) error {
	if !core.ValidID(user) {
		return core.Invalid("user_id", "请选择有效成员")
	}
	target := a
	target.UserID = user
	target.RunID = ""
	if _, err := projectScope(ctx, db, target, c.ProjectID); err != nil {
		return core.Invalid("user_id", "该成员当前不具备所选范围的访问权限")
	}
	if err := authz.Require(ctx, db, target, "conversation.read", resource(c)); err != nil {
		return core.Invalid("user_id", "该成员当前不具备所选范围的访问权限")
	}
	return nil
}
func normalizeMembers(a core.Actor, in ConversationCreate) ([]string, error) {
	if in.Type != "private" && in.Type != "project" {
		return nil, core.Invalid("type", "会话类型无效")
	}
	if in.Type == "project" && in.ProjectID == nil {
		return nil, core.Invalid("project_id", "共享会话必须属于指定项目")
	}
	if in.Type == "private" && len(in.MemberIDs) > 0 {
		return nil, core.Invalid("member_ids", "个人会话仅本人可访问")
	}
	if len(in.MemberIDs) > 99 {
		return nil, core.Invalid("member_ids", "会话成员数量超出限制")
	}
	result := []string{a.UserID}
	seen := map[string]bool{a.UserID: true}
	for _, id := range in.MemberIDs {
		id = strings.ToLower(id)
		if !core.ValidID(id) || seen[id] {
			return nil, core.Invalid("member_ids", "成员重复或无效；创建人已自动加入")
		}
		seen[id] = true
		result = append(result, id)
	}
	slices.Sort(result)
	return result, nil
}
