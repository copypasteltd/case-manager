package files

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type RetentionPolicyInput struct {
	ExpectedVersion    int64  `json:"expected_version"`
	Kind               string `json:"kind"`
	Enabled            bool   `json:"enabled"`
	RetainSeconds      int64  `json:"retain_seconds"`
	ObservationSeconds int64  `json:"observation_seconds"`
	Reason             string `json:"reason"`
}
type RetentionPolicy struct {
	ID                 string    `json:"id"`
	Version            int64     `json:"version"`
	Kind               string    `json:"kind"`
	Enabled            bool      `json:"enabled"`
	RetainSeconds      int64     `json:"retain_seconds"`
	ObservationSeconds int64     `json:"observation_seconds"`
	PublishedBy        string    `json:"published_by"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}

const retentionPolicyColumns = `id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason,created_at`

func scanRetentionPolicy(row pgx.Row) (RetentionPolicy, error) {
	var p RetentionPolicy
	err := row.Scan(&p.ID, &p.Version, &p.Kind, &p.Enabled, &p.RetainSeconds, &p.ObservationSeconds, &p.PublishedBy, &p.Reason, &p.CreatedAt)
	return p, platform.DatabaseError(err)
}
func (s *Service) RetentionPolicies(ctx context.Context, a core.Actor) (core.Page[RetentionPolicy], error) {
	out := core.Page[RetentionPolicy]{Items: []RetentionPolicy{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "config.manage"); err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, "SELECT DISTINCT ON(kind) "+retentionPolicyColumns+" FROM retention_policies WHERE org_id=$1 ORDER BY kind,version DESC", a.OrgID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanRetentionPolicy(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, rows.Err()
}
func (s *Service) PublishRetentionPolicy(ctx context.Context, c core.Command, in RetentionPolicyInput) (core.Result, error) {
	if !slices.Contains([]string{"formal_history", "trash", "draft", "multipart"}, in.Kind) || in.ExpectedVersion < 0 ||
		in.RetainSeconds < 86400 || in.RetainSeconds > 315360000 || in.ObservationSeconds < 86400 || in.ObservationSeconds > 31536000 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("policy", "请填写策略类型、明确保留时长、至少24小时观察期及修订原因")
	}
	c.AuthorizationMutation = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := interactiveMFA(c.Actor); err != nil {
			return err
		}
		if err := currentDownloadActor(ctx, tx, c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "config.manage")
	}, func(tx pgx.Tx) (core.Result, error) {
		var current int64
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM retention_policies WHERE org_id=$1 AND kind=$2", c.Actor.OrgID, in.Kind).Scan(&current); err != nil {
			return core.Result{}, err
		}
		if current != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		id := core.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.Actor.OrgID, current+1, in.Kind, in.Enabled, in.RetainSeconds, in.ObservationSeconds, c.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		p, err := scanRetentionPolicy(tx.QueryRow(ctx, "SELECT "+retentionPolicyColumns+" FROM retention_policies WHERE id=$1", id))
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "storage.retention_policy.published", ResourceType: "retention_policy", ResourceID: id, Version: p.Version, Payload: map[string]any{"kind": in.Kind, "reason": in.Reason}}}, nil
	})
}

type RetentionHold struct {
	ID            string     `json:"id"`
	FileID        string     `json:"file_id"`
	VersionID     *string    `json:"version_id"`
	Kind          string     `json:"kind"`
	Reason        string     `json:"reason"`
	OwnerID       string     `json:"owner_id"`
	ValidUntil    *time.Time `json:"valid_until"`
	CreatedAt     time.Time  `json:"created_at"`
	ReleasedAt    *time.Time `json:"released_at"`
	ReleasedBy    *string    `json:"released_by"`
	ReleaseReason *string    `json:"release_reason"`
	Active        bool       `json:"active"`
}
type RetentionHoldCreate struct {
	ExpectedVersion int64      `json:"expected_version"`
	VersionID       *string    `json:"version_id,omitempty"`
	Kind            string     `json:"kind"`
	Reason          string     `json:"reason"`
	ValidUntil      *time.Time `json:"valid_until,omitempty"`
}
type RetentionHoldRelease struct {
	Reason string `json:"reason"`
}
type RetentionOverview struct {
	FileID             string `json:"file_id"`
	LegalHold          bool   `json:"legal_hold"`
	ActiveHoldCount    int64  `json:"active_hold_count"`
	CanManage          bool   `json:"can_manage"`
	ExecutionAvailable bool   `json:"execution_available"`
}

const holdColumns = `h.id,h.file_id,h.version_id,h.kind,h.reason,h.owner_id,h.valid_until,h.created_at,
    r.created_at,r.released_by,r.reason,r.id IS NULL AND (h.valid_until IS NULL OR h.valid_until>clock_timestamp())`

func scanHold(row pgx.Row) (RetentionHold, error) {
	var h RetentionHold
	err := row.Scan(&h.ID, &h.FileID, &h.VersionID, &h.Kind, &h.Reason, &h.OwnerID, &h.ValidUntil, &h.CreatedAt, &h.ReleasedAt, &h.ReleasedBy, &h.ReleaseReason, &h.Active)
	return h, platform.DatabaseError(err)
}
func holdQuery() string {
	return "SELECT " + holdColumns + " FROM retention_holds h LEFT JOIN retention_hold_releases r ON r.org_id=h.org_id AND r.hold_id=h.id "
}
func retentionAccess(ctx context.Context, db platform.DBTX, a core.Actor, id string) (File, error) {
	f, err := file(ctx, db, a.OrgID, id, false)
	if err != nil {
		return f, err
	}
	if err = authz.Require(ctx, db, a, "file.read", fileResource(f)); err != nil {
		return f, err
	}
	return f, authz.Require(ctx, db, a, "file.retention.manage", fileResource(f))
}
func (s *Service) Retention(ctx context.Context, a core.Actor, id string) (RetentionOverview, error) {
	out := RetentionOverview{FileID: id}
	f, err := authorizeFile(ctx, s.DB, a, id, "file.read")
	if err != nil {
		return out, err
	}
	out.LegalHold = f.LegalHold
	out.CanManage = a.RunID == "" && slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) && authz.Require(ctx, s.DB, a, "file.retention.manage", fileResource(f)) == nil
	err = s.DB.QueryRow(ctx, `SELECT count(*) FROM retention_holds h WHERE h.org_id=$1 AND h.file_id=$2
        AND (h.valid_until IS NULL OR h.valid_until>clock_timestamp())
        AND NOT EXISTS(SELECT 1 FROM retention_hold_releases r WHERE r.org_id=h.org_id AND r.hold_id=h.id)`, a.OrgID, id).Scan(&out.ActiveHoldCount)
	return out, err
}
func (s *Service) RetentionHolds(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[RetentionHold], error) {
	out := core.Page[RetentionHold]{Items: []RetentionHold{}}
	if _, err := retentionAccess(ctx, s.DB, a, id); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/retention-holds/" + id))
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, holdQuery()+`WHERE h.org_id=$1 AND h.file_id=$2
        AND ($3::timestamptz IS NULL OR (h.created_at,h.id)<($3,$4::uuid))
        ORDER BY h.created_at DESC,h.id DESC LIMIT $5`, a.OrgID, id, at, before, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		h, e := scanHold(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &v
	}
	return out, nil
}
func retentionActor(ctx context.Context, tx pgx.Tx, a core.Actor, id string, mfa bool) error {
	if mfa {
		if err := interactiveMFA(a); err != nil {
			return err
		}
	}
	if err := currentDownloadActor(ctx, tx, a); err != nil {
		return err
	}
	_, err := retentionAccess(ctx, tx, a, id)
	return err
}
func (s *Service) CreateRetentionHold(ctx context.Context, c core.Command, id string, in RetentionHoldCreate) (core.Result, error) {
	if in.ExpectedVersion < 1 || !slices.Contains([]string{"legal", "business", "investigation"}, in.Kind) || core.Name(in.Reason) != nil ||
		(in.VersionID != nil && !core.ValidID(*in.VersionID)) {
		return core.Result{}, core.Invalid("hold", "请核对文件版本、保留类型、原因和有效期")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return retentionActor(ctx, tx, c.Actor, id, true) }, func(tx pgx.Tx) (core.Result, error) {
		f, err := file(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = retentionActor(ctx, tx, c.Actor, id, true); err != nil {
			return core.Result{}, err
		}
		if f.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if in.ValidUntil != nil && (!in.ValidUntil.After(time.Now()) || in.ValidUntil.After(time.Now().AddDate(10, 0, 0))) {
			return core.Result{}, core.Invalid("valid_until", "保留有效期必须晚于现在且不超过十年")
		}
		holdID := core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO retention_holds(id,org_id,file_id,version_id,kind,reason,owner_id,valid_until)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, holdID, c.Actor.OrgID, id, in.VersionID, in.Kind, in.Reason, c.Actor.UserID, in.ValidUntil); err != nil {
			return core.Result{}, err
		}
		h, err := scanHold(tx.QueryRow(ctx, holdQuery()+"WHERE h.org_id=$1 AND h.id=$2", c.Actor.OrgID, holdID))
		return core.Result{Status: 201, Data: h, Event: core.Event{Type: "file.retention_hold.created", ResourceType: "retention_hold", ResourceID: holdID, Version: 1, Payload: map[string]any{"file_id": id, "version_id": in.VersionID, "reason": in.Reason}}}, err
	})
}
func (s *Service) ReleaseRetentionHold(ctx context.Context, c core.Command, id string, in RetentionHoldRelease) (core.Result, error) {
	if !core.ValidID(id) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写释放保留约束的原因")
	}
	var fileID string
	if err := s.DB.QueryRow(ctx, "SELECT file_id FROM retention_holds WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id).Scan(&fileID); err != nil {
		return core.Result{}, platform.DatabaseError(err)
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return retentionActor(ctx, tx, c.Actor, fileID, true) }, func(tx pgx.Tx) (core.Result, error) {
		if _, err := file(ctx, tx, c.Actor.OrgID, fileID, true); err != nil {
			return core.Result{}, err
		}
		if err := retentionActor(ctx, tx, c.Actor, fileID, true); err != nil {
			return core.Result{}, err
		}
		h, err := scanHold(tx.QueryRow(ctx, holdQuery()+"WHERE h.org_id=$1 AND h.id=$2", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if h.ReleasedAt != nil {
			return core.Result{}, core.Fail(409, "RETENTION_HOLD_RELEASED", "该保留约束已经释放")
		}
		before := digest(h)
		if _, err = tx.Exec(ctx, "INSERT INTO retention_hold_releases(id,org_id,hold_id,released_by,reason) VALUES($1,$2,$3,$4,$5)", core.NewID(), c.Actor.OrgID, id, c.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		h, err = scanHold(tx.QueryRow(ctx, holdQuery()+"WHERE h.org_id=$1 AND h.id=$2", c.Actor.OrgID, id))
		return core.Result{Data: h, Event: core.Event{Type: "file.retention_hold.released", ResourceType: "retention_hold", ResourceID: id, Version: 2, BeforeDigest: before, Payload: map[string]any{"file_id": fileID, "reason": in.Reason}}}, err
	})
}
