package files

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// This role exposes only HEAD. Its credentials belong to a separate native
// worker; API processes retain only configured location fingerprints.
type OriginalObserver interface {
	Head(context.Context, string) (objectstore.Source, error)
}
type terminalCoordinate struct {
	TargetID           string
	Role               string
	Provider           string
	Bucket             string
	Key                string
	BindingSHA256      string
	ExclusiveNamespace bool
	RowSHA256          string
}
type terminalSnapshot struct {
	UploadSHA256 string
	Coordinates  []terminalCoordinate
}
type terminalObserved struct {
	Coordinate terminalCoordinate
	Outcome    string
	Source     *objectstore.Source
}
type TerminalObjectView struct {
	TargetID             string  `json:"target_id"`
	Role                 string  `json:"role"`
	Provider             string  `json:"provider"`
	Outcome              string  `json:"outcome"`
	IdentitySHA256       *string `json:"identity_sha256"`
	NativeVersionPresent bool    `json:"native_version_present"`
	SizeBytes            *string `json:"size_bytes"`
}
type TerminalObservation struct {
	ID          string               `json:"id"`
	UploadID    string               `json:"upload_id"`
	ActorID     string               `json:"actor_id"`
	BasisSHA256 string               `json:"basis_sha256"`
	Reason      string               `json:"reason"`
	CreatedAt   time.Time            `json:"created_at"`
	ExpiresAt   time.Time            `json:"expires_at"`
	Outcome     string               `json:"outcome"`
	ErrorCode   *string              `json:"error_code"`
	ObservedAt  *time.Time           `json:"observed_at"`
	Objects     []TerminalObjectView `json:"objects"`
	AllPresent  bool                 `json:"all_present"`
}
type TerminalCandidate struct {
	UploadID        string   `json:"upload_id"`
	UploadVersion   int64    `json:"upload_version"`
	FolderID        string   `json:"folder_id"`
	FolderVersion   int64    `json:"folder_version"`
	Name            string   `json:"name"`
	State           string   `json:"state"`
	BasisSHA256     string   `json:"basis_sha256"`
	ObjectCount     int      `json:"object_count"`
	CanObserve      bool     `json:"can_observe"`
	BlockingReasons []string `json:"blocking_reasons"`
}
type TerminalOverview struct {
	Candidate TerminalCandidate     `json:"candidate"`
	Items     []TerminalObservation `json:"items"`
	HasMore   bool                  `json:"has_more"`
}
type TerminalObserveCreate struct {
	ExpectedFolderVersion int64  `json:"expected_folder_version"`
	ExpectedUploadVersion int64  `json:"expected_upload_version"`
	BasisSHA256           string `json:"basis_sha256"`
	Reason                string `json:"reason"`
	Reviewed              bool   `json:"reviewed"`
}

func (s *ObjectRetirementService) terminalCoordinates(ctx context.Context, tx pgx.Tx, org string, u Upload) (terminalSnapshot, error) {
	out := terminalSnapshot{Coordinates: []terminalCoordinate{}}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(u)-'version'-'retirement_plan_id'-'updated_at' FROM upload_sessions u WHERE org_id=$1 AND id=$2`, org, u.ID).Scan(&raw)
	if e != nil {
		return out, e
	}
	out.UploadSHA256 = core.Digest(raw)
	if u.Provider == nil || u.Bucket == nil {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT id,'upload_draft',draft_key,$3::text,$4::text,to_jsonb(a) FROM upload_attempts a WHERE org_id=$1 AND session_id=$2
 UNION ALL SELECT id,'promotion_candidate',object_key,target_provider,target_bucket,to_jsonb(t) FROM object_transfers t WHERE org_id=$1 AND session_id=$2
 ORDER BY 2,1 LIMIT 65`, org, u.ID, u.Provider, u.Bucket)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var c terminalCoordinate
		var source []byte
		if e = rows.Scan(&c.TargetID, &c.Role, &c.Key, &c.Provider, &c.Bucket, &source); e != nil {
			break
		}
		c.RowSHA256 = core.Digest(source)
		if c.Role == "upload_draft" {
			key, _ := objectstore.DraftKey(org, c.TargetID)
			if key != c.Key {
				e = core.Fail(409, "SOURCE_CHANGED", "准确上传键与原记录不一致")
				break
			}
		} else {
			var r struct {
				FileID    string `json:"file_id"`
				VersionID string `json:"version_id"`
			}
			if json.Unmarshal(source, &r) != nil {
				e = core.Missing()
				break
			}
			key, _ := objectstore.VersionKey(org, r.FileID, r.VersionID)
			if key != c.Key {
				e = core.Fail(409, "SOURCE_CHANGED", "准确候选键与原记录不一致")
				break
			}
		}
		if b := s.Backends[c.Provider]; b != nil && b.Provider == c.Provider && b.Bucket == c.Bucket {
			c.BindingSHA256 = b.BindingSHA256
			c.ExclusiveNamespace = b.ExclusiveNamespace
		}
		out.Coordinates = append(out.Coordinates, c)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	return out, e
}
func (s *ObjectRetirementService) terminalCandidate(ctx context.Context, tx pgx.Tx, a core.Actor, id string, mfa bool) (TerminalCandidate, terminalSnapshot, error) {
	out := TerminalCandidate{BlockingReasons: []string{}}
	empty := terminalSnapshot{}
	u, e := upload(ctx, tx, a.OrgID, id, true)
	if e != nil {
		return out, empty, e
	}
	f, e := s.folderActor(ctx, tx, a, u.FolderID, mfa)
	if e != nil {
		return out, empty, e
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, mfa); e != nil {
		return out, empty, e
	}
	if e = completedDraftUploadAccess(ctx, tx, a, f, u); e != nil {
		return out, empty, e
	}
	out.UploadID = u.ID
	out.UploadVersion = u.Version
	out.FolderID = f.ID
	out.FolderVersion = f.Version
	out.Name = u.Name
	out.State = u.State
	snap, e := s.terminalCoordinates(ctx, tx, a.OrgID, u)
	if e != nil {
		return out, snap, e
	}
	out.BasisSHA256 = digest(snap)
	out.ObjectCount = len(snap.Coordinates)
	block := func(code string) { out.BlockingReasons = append(out.BlockingReasons, code) }
	if u.ResultVersionID != nil || u.State == "committed" {
		block("FORMAL_OBJECT_PROTECTED")
	}
	if u.RetirementPlanID != nil {
		block("ORIGINAL_ALREADY_RETIRED")
	}
	if time.Now().Before(u.ExpiresAt.Add(objectstore.MaxSignTTL)) {
		block("UPLOAD_OR_PERMIT_ACTIVE")
	}
	if f.State != "active" || activeAncestors(ctx, tx, a.OrgID, f.ID) != nil {
		block("UPLOAD_TARGET_CHANGED")
	}
	if len(snap.Coordinates) < 1 || len(snap.Coordinates) > 64 {
		block("OBJECT_REVIEW_LIMIT")
	}
	var busy bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE org_id=$1 AND payload->>'session_id'=$2::text AND (state IN('queued','running','retry_wait') OR updated_at>clock_timestamp()-$3::bigint*interval '1 second'))`, a.OrgID, u.ID, int64(objectstore.MaxSignTTL/time.Second)).Scan(&busy)
	if e != nil {
		return out, snap, e
	}
	if busy {
		block("UPLOAD_JOB_ACTIVE")
	}
	keys := map[string]bool{}
	for _, c := range snap.Coordinates {
		if len(c.BindingSHA256) != 64 {
			block("CLEANUP_BACKEND_UNAVAILABLE")
		}
		if c.Provider == "r2" && !c.ExclusiveNamespace {
			block("EXCLUSIVE_NAMESPACE_REQUIRED")
		}
		if keys[c.Key] {
			block("SOURCE_CHANGED")
		}
		keys[c.Key] = true
		var protected bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_versions WHERE org_id=$1 AND object_key=$2) OR EXISTS(SELECT 1 FROM file_replicas WHERE org_id=$1 AND object_key=$2)
   OR EXISTS(SELECT 1 FROM object_retirement_barriers WHERE org_id=$1 AND object_key=$2)
   OR EXISTS(SELECT 1 FROM upload_sessions WHERE org_id=$1 AND id<>$3 AND (draft_key=$2 OR candidate_key=$2))`, a.OrgID, c.Key, u.ID).Scan(&protected)
		if e != nil {
			return out, snap, e
		}
		if protected {
			block("FORMAL_OR_SHARED_OBJECT_PROTECTED")
		}
	}
	slices.Sort(out.BlockingReasons)
	out.BlockingReasons = slices.Compact(out.BlockingReasons)
	out.CanObserve = len(out.BlockingReasons) == 0
	return out, snap, nil
}
func (s *ObjectRetirementService) TerminalCandidates(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[TerminalCandidate], error) {
	out := core.Page[TerminalCandidate]{Items: []TerminalCandidate{}}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = s.folderActor(ctx, tx, a, id, false); e != nil {
		return out, e
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return out, e
	}
	at, rid, limit, binding, e := cleanupPage(s.Files, a, "terminal-upload-candidates/"+id, f)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id,created_at FROM upload_sessions WHERE org_id=$1 AND target_folder_id=$2 AND state<>'committed' AND committed_version_id IS NULL
  AND expires_at<=clock_timestamp() AND source_channel IN('web','android','wechat','douyin','agent') AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, rid, limit+1)
	if e != nil {
		return out, e
	}
	type target struct {
		id string
		at time.Time
	}
	all := []target{}
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.id, &t.at); e != nil {
			break
		}
		all = append(all, t)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for i, t := range all {
		if i == limit {
			break
		}
		v, _, err := s.terminalCandidate(ctx, tx, a, t.id, false)
		if err != nil {
			if slices.Contains([]int{403, 404}, core.PublicError(err).Status) {
				continue
			}
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if len(all) > limit {
		last := all[limit-1]
		v := s.Files.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.at, Binding: binding})
		out.NextCursor = &v
	}
	return out, tx.Commit(ctx)
}
func terminalObservation(ctx context.Context, db platform.DBTX, org, id string) (TerminalObservation, error) {
	out := TerminalObservation{Outcome: "pending", Objects: []TerminalObjectView{}}
	var resultID *string
	var raw []byte
	e := db.QueryRow(ctx, `SELECT q.id,q.upload_id,q.actor_id,q.basis_sha256,q.reason,q.created_at,q.expires_at,x.id,coalesce(x.outcome,'pending'),x.error_code,x.created_at,coalesce(x.all_present,false),coalesce(x.objects,'[]'::jsonb)
 FROM terminal_upload_observations q LEFT JOIN terminal_upload_observation_results x ON x.org_id=q.org_id AND x.request_id=q.id WHERE q.org_id=$1 AND q.id=$2`, org, id).Scan(&out.ID, &out.UploadID, &out.ActorID, &out.BasisSHA256, &out.Reason, &out.CreatedAt, &out.ExpiresAt, &resultID, &out.Outcome, &out.ErrorCode, &out.ObservedAt, &out.AllPresent, &raw)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	var objects []terminalObserved
	if e = json.Unmarshal(raw, &objects); e != nil {
		return out, e
	}
	for _, v := range objects {
		item := TerminalObjectView{TargetID: v.Coordinate.TargetID, Role: v.Coordinate.Role, Provider: v.Coordinate.Provider, Outcome: v.Outcome}
		if v.Source != nil {
			hash := digest(v.Source)
			size := strconv.FormatInt(v.Source.Size, 10)
			item.IdentitySHA256 = &hash
			item.NativeVersionPresent = v.Source.VersionID != "" && v.Source.VersionID != "null"
			item.SizeBytes = &size
		}
		out.Objects = append(out.Objects, item)
	}
	return out, nil
}
func (s *ObjectRetirementService) TerminalOverview(ctx context.Context, a core.Actor, id string) (TerminalOverview, error) {
	out := TerminalOverview{Items: []TerminalObservation{}}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	out.Candidate, _, e = s.terminalCandidate(ctx, tx, a, id, false)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT id FROM terminal_upload_observations WHERE org_id=$1 AND upload_id=$2 ORDER BY created_at DESC,id DESC LIMIT 51", a.OrgID, id)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			break
		}
		ids = append(ids, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	out.HasMore = len(ids) > 50
	for _, rid := range ids[:min(50, len(ids))] {
		v, err := terminalObservation(ctx, tx, a.OrgID, rid)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	return out, tx.Commit(ctx)
}
func (s *ObjectRetirementService) ObserveTerminal(ctx context.Context, c core.Command, id string, in TerminalObserveCreate) (core.Result, error) {
	if !core.ValidID(id) || in.ExpectedFolderVersion < 1 || in.ExpectedUploadVersion < 1 || len(in.BasisSHA256) != 64 || core.Name(in.Reason) != nil || !in.Reviewed {
		return core.Result{}, core.Invalid("reviewed", "请核对准确上传来源、当前版本并确认只读观察")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, _, e := s.terminalCandidate(ctx, tx, c.Actor, id, true); return e }, func(tx pgx.Tx) (core.Result, error) {
		v, snap, e := s.terminalCandidate(ctx, tx, c.Actor, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if !v.CanObserve {
			return core.Result{}, core.Fail(409, "TERMINAL_OBSERVATION_BLOCKED", "活跃操作、原件引用或来源保护尚未排除")
		}
		if v.UploadVersion != in.ExpectedUploadVersion || v.FolderVersion != in.ExpectedFolderVersion || v.BasisSHA256 != in.BasisSHA256 {
			return core.Result{}, core.Conflict()
		}
		rid := core.NewID()
		raw, _ := json.Marshal(snap)
		_, e = tx.Exec(ctx, `INSERT INTO terminal_upload_observations(id,org_id,folder_id,upload_id,actor_id,session_id,channel,actor_auth_version,organization_auth_version,folder_version,upload_version,basis_sha256,snapshot,reason,mfa_verified_at,expires_at)
   SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,s.mfa_verified_at,LEAST(clock_timestamp()+interval '5 minutes',s.mfa_verified_at+interval '10 minutes') FROM sessions s WHERE s.org_id=$2 AND s.id=$6`, rid, c.Actor.OrgID, v.FolderID, id, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, v.FolderVersion, v.UploadVersion, v.BasisSHA256, raw, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		if e = s.enqueueTerminal(ctx, tx, c.Actor.OrgID, rid); e != nil {
			return core.Result{}, e
		}
		out, e := terminalObservation(ctx, tx, c.Actor.OrgID, rid)
		return core.Result{Status: 201, Data: out, Event: core.Event{Type: "file.terminal.observation.requested", ResourceType: "terminal_upload_observation", ResourceID: rid, Version: 1, Payload: map[string]any{"upload_id": id, "basis_sha256": v.BasisSHA256, "object_count": v.ObjectCount, "reason": in.Reason}}}, e
	})
}

// A receipt supplies exact sources only when every recorded coordinate still
// matches. It does not change old state, validate bytes or authorize deletion.
func (s *ObjectRetirementService) terminalObservedObjects(ctx context.Context, tx pgx.Tx, org string, u Upload) ([]RetirementObject, *time.Time, string, error) {
	out := []RetirementObject{}
	current, e := s.terminalCoordinates(ctx, tx, org, u)
	if e != nil {
		return out, nil, "", e
	}
	basis := digest(current)
	var id string
	var observed time.Time
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT x.id,x.created_at,x.objects FROM terminal_upload_observation_results x JOIN terminal_upload_observations q ON q.org_id=x.org_id AND q.id=x.request_id
 WHERE q.org_id=$1 AND q.upload_id=$2 AND q.basis_sha256=$3 AND x.all_present ORDER BY x.created_at DESC,x.id DESC LIMIT 1`, org, u.ID, basis).Scan(&id, &observed, &raw)
	if e == pgx.ErrNoRows {
		return out, nil, "", nil
	}
	if e != nil {
		return out, nil, "", e
	}
	var values []terminalObserved
	if json.Unmarshal(raw, &values) != nil || len(values) != len(current.Coordinates) {
		return out, nil, "", core.Fail(409, "SOURCE_CHANGED", "准确原生观察与当前来源不一致")
	}
	for i, v := range values {
		if v.Coordinate != current.Coordinates[i] || v.Source == nil || v.Outcome != "observed_present" {
			return nil, nil, "", core.Fail(409, "SOURCE_CHANGED", "准确原生观察与当前来源不一致")
		}
		out = append(out, RetirementObject{Provider: v.Coordinate.Provider, Bucket: v.Coordinate.Bucket, Source: *v.Source, Role: v.Coordinate.Role})
	}
	return out, &observed, digest([]any{id, basis, values}), nil
}
