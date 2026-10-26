package files

import (
	"context"
	"mime"
	"slices"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type WaiverPolicyInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	Classification  string   `json:"classification"`
	Enabled         bool     `json:"enabled"`
	MediaTypes      []string `json:"media_types"`
	AllowedCodes    []string `json:"allowed_codes"`
	MaxBytes        string   `json:"max_bytes"`
	ReviewerID      string   `json:"reviewer_id"`
	Reason          string   `json:"reason"`
}
type WaiverPolicy struct {
	ID             string    `json:"id"`
	Version        int64     `json:"version"`
	Classification string    `json:"classification"`
	Enabled        bool      `json:"enabled"`
	MediaTypes     []string  `json:"media_types"`
	AllowedCodes   []string  `json:"allowed_codes"`
	MaxBytes       string    `json:"max_bytes"`
	ReviewerID     string    `json:"reviewer_id"`
	Reason         string    `json:"reason"`
	PublishedBy    string    `json:"published_by"`
	CreatedAt      time.Time `json:"created_at"`
}

const waiverPolicyColumns = `id,version,classification,enabled,media_types,allowed_codes,max_bytes::text,reviewer_id,reason,published_by,created_at`

func scanWaiverPolicy(row pgx.Row) (WaiverPolicy, error) {
	var p WaiverPolicy
	err := row.Scan(&p.ID, &p.Version, &p.Classification, &p.Enabled, &p.MediaTypes, &p.AllowedCodes, &p.MaxBytes, &p.ReviewerID, &p.Reason, &p.PublishedBy, &p.CreatedAt)
	return p, err
}
func interactiveMFA(a core.Actor) error {
	if !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) || a.RunID != "" {
		return core.Forbidden()
	}
	if a.MFAVerifiedAt == nil || time.Since(*a.MFAVerifiedAt) > 10*time.Minute || a.MFAVerifiedAt.After(time.Now().Add(time.Minute)) {
		return core.Fail(403, "REAUTHENTICATION_REQUIRED", "此操作需要最近10分钟内完成两步验证")
	}
	return nil
}
func (s *UploadService) PublishWaiverPolicy(ctx context.Context, c core.Command, in WaiverPolicyInput) (core.Result, error) {
	if (in.Classification != "internal" && in.Classification != "project_restricted") || in.ExpectedVersion < 0 || core.Name(in.Reason) != nil || !core.ValidID(in.ReviewerID) {
		return core.Result{}, core.Invalid("policy", "仅支持非严格受限资料的明确例外策略")
	}
	max, err := strconv.ParseInt(in.MaxBytes, 10, 64)
	if err != nil || strconv.FormatInt(max, 10) != in.MaxBytes || max < 1 || max > 120000000000 {
		return core.Result{}, core.Invalid("max_bytes", "例外大小上限无效")
	}
	if len(in.MediaTypes) < 1 || len(in.MediaTypes) > 20 || len(in.AllowedCodes) < 1 || len(in.AllowedCodes) > 4 {
		return core.Result{}, core.Invalid("policy", "请明确允许格式及无法扫描原因")
	}
	for _, media := range in.MediaTypes {
		t, p, e := mime.ParseMediaType(media)
		if e != nil || t != media || len(p) > 0 || len(media) > 200 || strings.Contains(media, "*") || slices.Contains([]string{"application/x-msdownload", "application/x-executable", "application/x-sh", "application/javascript", "text/javascript"}, media) {
			return core.Result{}, core.Invalid("media_types", "不接受可执行类型或非规范媒体类型")
		}
	}
	for _, code := range in.AllowedCodes {
		if !slices.Contains([]string{"SCANNER_NOT_CONFIGURED", "SCAN_SIZE_LIMIT", "SCAN_ENGINE_SIZE_LIMIT", "SCAN_CONTENT_LIMIT"}, code) {
			return core.Result{}, core.Invalid("allowed_codes", "该扫描结论不可例外放行")
		}
	}
	c.AuthorizationMutation = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := interactiveMFA(c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "config.manage")
	}, func(tx pgx.Tx) (core.Result, error) {
		var current int64
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM scan_waiver_policies WHERE org_id=$1 AND classification=$2", c.Actor.OrgID, in.Classification).Scan(&current); err != nil {
			return core.Result{}, err
		}
		if current != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		var active bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND status='active')", c.Actor.OrgID, in.ReviewerID).Scan(&active); err != nil {
			return core.Result{}, err
		}
		if !active {
			return core.Result{}, core.Invalid("reviewer_id", "审核人必须是当前组织的有效用户")
		}
		id := core.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO scan_waiver_policies(id,org_id,classification,version,enabled,media_types,allowed_codes,max_bytes,reviewer_id,reason,published_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, c.Actor.OrgID, in.Classification, current+1, in.Enabled, in.MediaTypes, in.AllowedCodes, max, in.ReviewerID, in.Reason, c.Actor.UserID); err != nil {
			return core.Result{}, err
		}
		p, err := scanWaiverPolicy(tx.QueryRow(ctx, "SELECT "+waiverPolicyColumns+" FROM scan_waiver_policies WHERE id=$1", id))
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "storage.scan_waiver_policy.published", ResourceType: "scan_waiver_policy", ResourceID: id, Version: p.Version, Payload: map[string]any{"classification": in.Classification, "reason": in.Reason}}}, nil
	})
}
func (s *UploadService) WaiverPolicies(ctx context.Context, a core.Actor) (core.Page[WaiverPolicy], error) {
	out := core.Page[WaiverPolicy]{Items: []WaiverPolicy{}}
	if err := authz.RequireOrganization(ctx, s.Files.DB, a, "config.manage"); err != nil {
		return out, err
	}
	rows, err := s.Files.DB.Query(ctx, "SELECT DISTINCT ON(classification) "+waiverPolicyColumns+" FROM scan_waiver_policies WHERE org_id=$1 ORDER BY classification,version DESC", a.OrgID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanWaiverPolicy(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, rows.Err()
}

type WaiverCreate struct {
	ExpectedVersion  int64  `json:"expected_version"`
	ScanReportID     string `json:"scan_report_id"`
	SHA256           string `json:"sha256"`
	Reason           string `json:"reason"`
	AcknowledgedRisk bool   `json:"acknowledged_risk"`
}
type ScanWaiver struct {
	ID             string    `json:"id"`
	Version        int64     `json:"version"`
	UploadID       string    `json:"upload_id"`
	ScanReportID   string    `json:"scan_report_id"`
	PolicyID       string    `json:"policy_id"`
	PolicyVersion  int64     `json:"policy_version"`
	UploadVersion  int64     `json:"upload_version"`
	OwnerID        string    `json:"owner_id"`
	ReviewerID     string    `json:"reviewer_id"`
	Classification string    `json:"classification"`
	Name           string    `json:"name"`
	MediaType      string    `json:"media_type"`
	SizeBytes      string    `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	ScanCode       string    `json:"scan_code"`
	Reason         string    `json:"reason"`
	State          string    `json:"state"`
	DecisionReason *string   `json:"decision_reason"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const waiverColumns = `id,version,session_id,report_id,policy_id,policy_version,upload_version,owner_id,reviewer_id,classification,name,media_type,
	size_bytes::text,sha256,scan_code,reason,state,decision_reason,expires_at,created_at,updated_at`

func scanWaiver(row pgx.Row) (ScanWaiver, error) {
	var w ScanWaiver
	e := row.Scan(&w.ID, &w.Version, &w.UploadID, &w.ScanReportID, &w.PolicyID, &w.PolicyVersion, &w.UploadVersion, &w.OwnerID, &w.ReviewerID, &w.Classification,
		&w.Name, &w.MediaType, &w.SizeBytes, &w.SHA256, &w.ScanCode, &w.Reason, &w.State, &w.DecisionReason, &w.ExpiresAt, &w.CreatedAt, &w.UpdatedAt)
	return w, platform.DatabaseError(e)
}
func (s *UploadService) RequestWaiver(ctx context.Context, c core.Command, id string, in WaiverCreate) (core.Result, error) {
	if !in.AcknowledgedRisk || core.Name(in.Reason) != nil || !core.ValidID(in.ScanReportID) || !shaPattern.MatchString(in.SHA256) {
		return core.Result{}, core.Invalid("waiver", "请核对具体扫描报告、摘要及例外风险")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"blocked"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		if u.ScanState != "unscannable" || u.ScanResultID == nil || *u.ScanResultID != in.ScanReportID || u.SHA256 == nil || *u.SHA256 != in.SHA256 || u.FrozenSourceID == nil {
			return core.Result{}, core.Fail(409, "SCAN_WAIVER_NOT_ELIGIBLE", "该固定源没有可例外处理的扫描报告")
		}
		p, err := scanWaiverPolicy(tx.QueryRow(ctx, "SELECT "+waiverPolicyColumns+" FROM scan_waiver_policies WHERE org_id=$1 AND classification=$2 ORDER BY version DESC LIMIT 1", c.Actor.OrgID, *u.Classification))
		if err == pgx.ErrNoRows {
			return core.Result{}, core.Fail(409, "SCAN_WAIVER_POLICY_REQUIRED", "尚未发布该分类的扫描例外制度")
		}
		if err != nil {
			return core.Result{}, err
		}
		max, _ := strconv.ParseInt(p.MaxBytes, 10, 64)
		var code, state, sourceDigest, sha string
		err = tx.QueryRow(ctx, "SELECT code,state,source_digest,sha256 FROM upload_scan_reports WHERE org_id=$1 AND session_id=$2 AND id=$3", c.Actor.OrgID, u.ID, in.ScanReportID).Scan(&code, &state, &sourceDigest, &sha)
		if err != nil {
			return core.Result{}, err
		}
		if !p.Enabled || !slices.Contains(p.MediaTypes, u.MediaType) || !slices.Contains(p.AllowedCodes, code) || u.layout().Size > max ||
			state != "unscannable" || sourceDigest != core.Digest([]byte(*u.FrozenSourceID)) || sha != in.SHA256 {
			return core.Result{}, core.Fail(409, "SCAN_WAIVER_NOT_ELIGIBLE", "该文件不在当前明确例外范围内")
		}
		if p.ReviewerID == c.Actor.UserID {
			return core.Result{}, core.Fail(409, "INDEPENDENT_REVIEWER_REQUIRED", "例外审核人不能是上传申请人")
		}
		if _, _, e := reviewerAccess(ctx, tx, core.Actor{OrgID: c.Actor.OrgID, UserID: p.ReviewerID, Channel: c.Actor.Channel},
			ScanWaiver{UploadID: u.ID, OwnerID: u.OwnerID, ReviewerID: p.ReviewerID}); e != nil {
			if core.PublicError(e).Status == 404 {
				return core.Result{}, core.Fail(409, "SCAN_REVIEWER_UNAVAILABLE", "当前例外审核人没有有效范围授权")
			}
			return core.Result{}, e
		}
		id := core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO scan_waivers(id,org_id,session_id,report_id,source_digest,sha256,policy_id,policy_version,upload_version,
			owner_id,reviewer_id,classification,name,media_type,size_bytes,scan_code,reason,acknowledged_risk,expires_at,state)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,true,$18,'pending')`,
			id, c.Actor.OrgID, u.ID, in.ScanReportID, sourceDigest, in.SHA256, p.ID, p.Version, u.Version+1, u.OwnerID, p.ReviewerID, *u.Classification, u.Name, u.MediaType, u.layout().Size, code, in.Reason, u.ExpiresAt); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE upload_sessions SET version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID); err != nil {
			return core.Result{}, err
		}
		w, err := scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE id=$1", id))
		return core.Result{Status: 201, Data: w, Event: core.Event{Type: "storage.scan_waiver.requested", ResourceType: "scan_waiver", ResourceID: id, Version: 1, Payload: map[string]any{"upload_id": u.ID, "report_id": in.ScanReportID, "reason": in.Reason}}}, err
	})
}
func reviewerAccess(ctx context.Context, db platform.DBTX, a core.Actor, w ScanWaiver) (Upload, Folder, error) {
	if w.ReviewerID != a.UserID || w.OwnerID == a.UserID {
		return Upload{}, Folder{}, core.Missing()
	}
	u, err := upload(ctx, db, a.OrgID, w.UploadID, false)
	if err != nil {
		return u, Folder{}, err
	}
	f, err := authorizeFolder(ctx, db, a, u.FolderID, "folder.read")
	if err != nil {
		return u, f, err
	}
	if u.FileID != nil {
		if _, err = authorizeFile(ctx, db, a, *u.FileID, "file.read"); err != nil {
			return u, f, err
		}
	}
	err = authz.Require(ctx, db, a, "file.scan.waive", authz.Resource{Type: "upload", ID: u.ID, ProjectID: f.ProjectID, OwnerID: &u.OwnerID, Classification: f.Classification})
	return u, f, err
}
func (s *UploadService) Waiver(ctx context.Context, a core.Actor, id string) (ScanWaiver, error) {
	if a.RunID != "" || !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) {
		return ScanWaiver{}, core.Forbidden()
	}
	if !core.ValidID(id) {
		return ScanWaiver{}, core.Missing()
	}
	w, err := scanWaiver(s.Files.DB.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2", a.OrgID, id))
	if err != nil {
		return w, err
	}
	if w.OwnerID == a.UserID {
		_, _, err = s.authorize(ctx, s.Files.DB, a, w.UploadID)
	} else {
		_, _, err = reviewerAccess(ctx, s.Files.DB, a, w)
	}
	return w, err
}
func (s *UploadService) WaiverReport(ctx context.Context, a core.Actor, id string) (ScanReport, error) {
	w, err := s.Waiver(ctx, a, id)
	if err != nil {
		return ScanReport{}, err
	}
	// An application always refers to its immutable report, even after rescan.
	return s.readScanReport(ctx, a.OrgID, w.UploadID, w.ScanReportID)
}
func (s *UploadService) Waivers(ctx context.Context, a core.Actor, state, cursor string, limit int) (core.Page[ScanWaiver], error) {
	out := core.Page[ScanWaiver]{Items: []ScanWaiver{}}
	if !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) || a.RunID != "" {
		return out, core.Forbidden()
	}
	if state != "" && !slices.Contains([]string{"pending", "approved", "rejected", "cancelled"}, state) {
		return out, core.Invalid("state", "例外状态无效")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/scan-waivers/" + state))
	var at *time.Time
	var beforeID *string
	if cursor != "" {
		c, e := s.Files.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.Files.DB.Query(ctx, `SELECT w.id,w.updated_at FROM scan_waivers w
		JOIN upload_sessions u ON u.org_id=w.org_id AND u.id=w.session_id JOIN folders f ON f.org_id=u.org_id AND f.id=u.target_folder_id
		LEFT JOIN files fl ON fl.org_id=u.org_id AND fl.id=u.file_id
		WHERE w.org_id=$1 AND ($3='' OR w.state=$3) AND ($4::timestamptz IS NULL OR (w.updated_at,w.id)<($4,$5::uuid))
		AND can_access($1,$2,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,$6,$7)
		AND ((w.owner_id=$2 AND can_access($1,$2,CASE WHEN u.file_id IS NULL THEN 'file.upload' ELSE 'file.update' END,
			CASE WHEN u.file_id IS NULL THEN 'folder' ELSE 'file' END,coalesce(fl.id,f.id),f.project_id,coalesce(fl.creator_id,f.creator_id),coalesce(fl.classification,f.classification),$6,$7))
		OR (w.reviewer_id=$2 AND w.owner_id<>$2 AND can_access($1,$2,'file.scan.waive','upload',u.id,f.project_id,u.owner_id,f.classification,$6,$7)
			AND (u.file_id IS NULL OR can_access($1,$2,'file.read','file',fl.id,f.project_id,fl.creator_id,fl.classification,$6,$7))))
		ORDER BY w.updated_at DESC,w.id DESC LIMIT $8`, a.OrgID, a.UserID, state, at, beforeID, a.Channel, a.RunID != "", limit+1)
	if err != nil {
		return out, err
	}
	type candidate struct {
		id string
		at time.Time
	}
	items := []candidate{}
	for rows.Next() {
		var v candidate
		if err = rows.Scan(&v.id, &v.at); err != nil {
			rows.Close()
			return out, err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next := s.Files.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.at, Binding: binding})
		out.NextCursor = &next
	}
	for _, item := range items {
		w, e := s.Waiver(ctx, a, item.id)
		if e != nil {
			if core.PublicError(e).Status == 404 {
				continue
			}
			return out, e
		}
		out.Items = append(out.Items, w)
	}
	return out, nil
}
func (s *UploadService) CancelWaiver(ctx context.Context, c core.Command, id string, in Transition) (core.Result, error) {
	if in.Action != "cancel" || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("action", "请指定取消并说明原因")
	}
	var target Folder
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		w, e := scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		if e != nil {
			return e
		}
		if w.OwnerID != c.Actor.UserID {
			return core.Missing()
		}
		_, target, e = s.authorize(ctx, tx, c.Actor, w.UploadID)
		return e
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := namespaceLock(ctx, tx, c.Actor, target.ProjectID); err != nil {
			return core.Result{}, err
		}
		w, err := scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if w.OwnerID != c.Actor.UserID || w.Version != in.ExpectedVersion || w.State != "pending" {
			return core.Result{}, core.Conflict()
		}
		if _, _, err = s.authorize(ctx, tx, c.Actor, w.UploadID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO scan_waiver_decisions(id,org_id,waiver_id,actor_id,decision,reason) VALUES($1,$2,$3,$4,'cancel',$5)", core.NewID(), c.Actor.OrgID, w.ID, c.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE scan_waivers SET state='cancelled',decision_reason=$2,decided_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE id=$1", id, in.Reason); err != nil {
			return core.Result{}, err
		}
		w, err = scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE id=$1", id))
		return core.Result{Data: w, Event: core.Event{Type: "storage.scan_waiver.cancelled", ResourceType: "scan_waiver", ResourceID: w.ID, Version: w.Version, Payload: map[string]any{"reason": in.Reason}}}, err
	})
}

type WaiverDecision struct {
	ExpectedVersion  int64  `json:"expected_version"`
	UploadVersion    int64  `json:"upload_version"`
	ScanReportID     string `json:"scan_report_id"`
	SHA256           string `json:"sha256"`
	Decision         string `json:"decision"`
	Reason           string `json:"reason"`
	AcknowledgedRisk bool   `json:"acknowledged_risk"`
}

func (s *UploadService) DecideWaiver(ctx context.Context, c core.Command, id string, in WaiverDecision) (core.Result, error) {
	if !core.ValidID(id) || !core.ValidID(in.ScanReportID) || !shaPattern.MatchString(in.SHA256) || core.Name(in.Reason) != nil ||
		(in.Decision != "approve" && in.Decision != "reject") || (in.Decision == "approve" && !in.AcknowledgedRisk) {
		return core.Result{}, core.Invalid("decision", "请核对固定报告和风险并说明决定")
	}
	var target Folder
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := interactiveMFA(c.Actor); err != nil {
			return err
		}
		w, err := scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		if err != nil {
			return err
		}
		_, target, err = reviewerAccess(ctx, tx, c.Actor, w)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := namespaceLock(ctx, tx, c.Actor, target.ProjectID); err != nil {
			return core.Result{}, err
		}
		w, err := scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if _, _, err = reviewerAccess(ctx, tx, c.Actor, w); err != nil {
			return core.Result{}, err
		}
		u, err := upload(ctx, tx, c.Actor.OrgID, w.UploadID, true)
		if err != nil {
			return core.Result{}, err
		}
		w, err = scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if w.Version != in.ExpectedVersion || w.State != "pending" || in.UploadVersion != w.UploadVersion || in.ScanReportID != w.ScanReportID || in.SHA256 != w.SHA256 {
			return core.Result{}, core.Conflict()
		}
		if !time.Now().Before(w.ExpiresAt) {
			return core.Result{}, core.Fail(410, "SCAN_WAIVER_EXPIRED", "扫描例外申请已过期")
		}
		events := []core.Event{}
		state := "rejected"
		if in.Decision == "approve" {
			if u.State != "blocked" || u.ScanState != "unscannable" || u.Version != w.UploadVersion || u.ScanResultID == nil || *u.ScanResultID != w.ScanReportID ||
				u.SHA256 == nil || *u.SHA256 != w.SHA256 || u.Classification == nil || *u.Classification != w.Classification {
				return core.Result{}, core.Fail(409, "SCAN_EVIDENCE_MISMATCH", "上传阶段、分类或扫描证据已改变")
			}
			p, e := scanWaiverPolicy(tx.QueryRow(ctx, "SELECT "+waiverPolicyColumns+" FROM scan_waiver_policies WHERE org_id=$1 AND classification=$2 ORDER BY version DESC LIMIT 1", c.Actor.OrgID, w.Classification))
			if e != nil {
				return core.Result{}, e
			}
			if !p.Enabled || p.ID != w.PolicyID || p.Version != w.PolicyVersion || p.ReviewerID != c.Actor.UserID {
				return core.Result{}, core.Fail(409, "SCAN_WAIVER_POLICY_CHANGED", "扫描例外制度已变更，请重新申请")
			}
			if err = s.workerAllowed(ctx, tx, jobqueue.Job{OrgID: c.Actor.OrgID}, u); err != nil {
				return core.Result{}, err
			}
			before := digest(u)
			if _, err = tx.Exec(ctx, "UPDATE upload_sessions SET scan_state='waived',scan_waiver_ref=$2,state='verifying',last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID, w.ID); err != nil {
				return core.Result{}, err
			}
			u, err = upload(ctx, tx, c.Actor.OrgID, u.ID, false)
			if err != nil {
				return core.Result{}, err
			}
			event := uploadEvent("upload.scan.waived", u, map[string]any{"source_digest": core.Digest([]byte(*u.FrozenSourceID)), "size_bytes": u.SizeBytes, "waiver_id": w.ID, "reason": in.Reason})
			event.BeforeDigest = before
			events = append(events, event)
			state = "approved"
		}
		if _, err = tx.Exec(ctx, "INSERT INTO scan_waiver_decisions(id,org_id,waiver_id,actor_id,decision,reason,acknowledged_risk,mfa_verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", core.NewID(), c.Actor.OrgID, w.ID, c.Actor.UserID, in.Decision, in.Reason, in.AcknowledgedRisk, c.Actor.MFAVerifiedAt); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE scan_waivers SET state=$2,decision_reason=$3,decided_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE id=$1", id, state, in.Reason); err != nil {
			return core.Result{}, err
		}
		w, err = scanWaiver(tx.QueryRow(ctx, "SELECT "+waiverColumns+" FROM scan_waivers WHERE id=$1", id))
		return core.Result{Data: w, Event: core.Event{Type: "storage.scan_waiver.decided", ResourceType: "scan_waiver", ResourceID: id, Version: w.Version, Payload: map[string]any{"decision": in.Decision, "reason": in.Reason}}, Events: events}, err
	})
}
