package workflow

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
)

type AccessCheck struct {
	Action                      string         `json:"action"`
	Resource                    authz.Resource `json:"resource"`
	ClassificationPolicyVersion int64          `json:"classification_policy_version"`
	RoleBindingIDs              []string       `json:"role_binding_ids"`
	ResourceGrantIDs            []string       `json:"resource_grant_ids"`
}
type AuthorityEvidence struct {
	UserID          string        `json:"user_id"`
	UserAuthVersion int64         `json:"user_auth_version"`
	OrgAuthVersion  int64         `json:"org_auth_version"`
	Channel         string        `json:"channel"`
	Checks          []AccessCheck `json:"checks"`
}

func authorityCheck(ctx context.Context, db platform.DBTX, a core.Actor, action string, resource authz.Resource) (AccessCheck, error) {
	c := AccessCheck{Action: action, Resource: resource, RoleBindingIDs: []string{}, ResourceGrantIDs: []string{}}
	if err := authz.Require(ctx, db, a, action, resource); err != nil {
		return c, err
	}
	if err := db.QueryRow(ctx, "SELECT max(policy_version) FROM classification_policies WHERE org_id=$1 AND level=$2", a.OrgID, resource.Classification).Scan(&c.ClassificationPolicyVersion); err != nil {
		return c, err
	}
	rows, err := db.Query(ctx, `SELECT rb.id FROM role_bindings rb JOIN role_actions ra ON ra.org_id=rb.org_id AND ra.role_id=rb.role_id
        WHERE rb.org_id=$1 AND rb.user_id=$2 AND rb.revoked_at IS NULL AND ra.action=$3 AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())
        AND classification_rank(rb.classification_ceiling)>=classification_rank($4)
        AND (rb.scope_type='org' OR (rb.scope_type='project' AND rb.scope_id=$5::uuid) OR (rb.scope_type='resource' AND rb.scope_id=NULLIF($6,'')::uuid)
            OR (rb.scope_type='own' AND $7::uuid=$2)) ORDER BY rb.id`, a.OrgID, a.UserID, action, resource.Classification, resource.ProjectID, resource.ID, resource.OwnerID)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return c, err
		}
		c.RoleBindingIDs = append(c.RoleBindingIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return c, err
	}
	rows, err = db.Query(ctx, `SELECT id FROM resource_grants WHERE org_id=$1 AND user_id=$2 AND resource_type=$3 AND resource_id=NULLIF($4,'')::uuid AND action=$5
        AND (expires_at IS NULL OR expires_at>now()) AND classification_rank(classification_ceiling)>=classification_rank($6) ORDER BY id`, a.OrgID, a.UserID, resource.Type, resource.ID, action, resource.Classification)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return c, err
		}
		c.ResourceGrantIDs = append(c.ResourceGrantIDs, id)
	}
	err = rows.Err()
	rows.Close()
	return c, err
}
func approvalAuthority(ctx context.Context, db platform.DBTX, a core.Actor, r Request, p Snapshot) (AuthorityEvidence, error) {
	out := AuthorityEvidence{UserID: a.UserID, OrgAuthVersion: a.OrgAuthVersion, Channel: a.Channel, Checks: []AccessCheck{}}
	if err := db.QueryRow(ctx, "SELECT auth_version FROM users WHERE org_id=$1 AND id=$2 AND status='active'", a.OrgID, a.UserID).Scan(&out.UserAuthVersion); err != nil {
		return out, platform.DatabaseError(err)
	}
	d, err := businessRead(ctx, db, a, r)
	if err != nil {
		return out, err
	}
	checks := []struct {
		action string
		r      authz.Resource
	}{{"request.read", requestResource(r)}, {"approval.decide", requestResource(r)},
		{d.Kind + ".read", businessResource(d)}}
	if d.Kind == "change" {
		var owner, classification string
		if err := db.QueryRow(ctx, "SELECT creator_id,classification FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, d.ProjectID).Scan(&owner, &classification); err != nil {
			return out, platform.DatabaseError(err)
		}
		checks = append(checks, struct {
			action string
			r      authz.Resource
		}{"budget.read", authz.Resource{Type: "project", ID: *d.ProjectID, ProjectID: d.ProjectID, OwnerID: &owner, Classification: classification}})
	}
	if p.PurchaseVariance != nil {
		var owner string
		if err := db.QueryRow(ctx, "SELECT creator_id FROM spend_documents WHERE org_id=$1 AND id=$2 AND kind='purchase'", a.OrgID, p.PurchaseVariance.PurchaseID).Scan(&owner); err != nil {
			return out, platform.DatabaseError(err)
		}
		checks = append(checks, struct {
			action string
			r      authz.Resource
		}{"purchase.read", authz.Resource{Type: "purchase", ID: p.PurchaseVariance.PurchaseID, ProjectID: d.ProjectID, OwnerID: &owner, Classification: d.Classification}})
	}
	refs, err := snapshotSuppliers(p)
	if err != nil {
		return out, err
	}
	for _, ref := range refs {
		v, _, e := party.ResolveBusinessReference(ctx, db, a, ref.PartyID, ref.RevisionID, false)
		if e != nil {
			return out, e
		}
		checks = append(checks, struct {
			action string
			r      authz.Resource
		}{"party.read", authz.Resource{Type: "party", ID: v.ID, ProjectID: v.ProjectID, OwnerID: &v.CreatorID, Classification: v.Classification}})
	}
	for _, fixed := range p.Attachments {
		v, e := fileref.ForApproval(ctx, db, a, fixed.VersionID, p.ProjectID, p.Classification)
		if e != nil {
			return out, e
		}
		if v.FileID != fixed.FileID || v.SHA256 != fixed.SHA256 || v.SizeBytes != fixed.SizeBytes || v.ScanState != fixed.ScanState || v.Seq != fixed.Seq || v.Name != fixed.Name || v.MediaType != fixed.MediaType {
			return out, core.Fail(409, "APPROVAL_EVIDENCE_CHANGED", "固定附件依据已变化，不能继续审批")
		}
		var owner, project *string
		var class string
		if e = db.QueryRow(ctx, "SELECT f.creator_id,d.project_id,f.classification FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=$1 AND f.id=$2", a.OrgID, v.FileID).Scan(&owner, &project, &class); e != nil {
			return out, e
		}
		checks = append(checks, struct {
			action string
			r      authz.Resource
		}{"file.read", authz.Resource{Type: "file", ID: v.FileID, ProjectID: project, OwnerID: owner, Classification: class}})
	}
	for _, c := range checks {
		v, e := authorityCheck(ctx, db, a, c.action, c.r)
		if e != nil {
			return out, e
		}
		out.Checks = append(out.Checks, v)
	}
	return out, nil
}
func reviewerAuthority(ctx context.Context, db platform.DBTX, submitter core.Actor, r Request, p Snapshot, user string) (AuthorityEvidence, error) {
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		a := core.Actor{OrgID: submitter.OrgID, UserID: user, OrgAuthVersion: submitter.OrgAuthVersion, Channel: channel}
		proof, err := approvalAuthority(ctx, db, a, r, p)
		if err == nil {
			return proof, nil
		}
		status := core.PublicError(err).Status
		if status != 403 && status != 404 {
			return proof, err
		}
	}
	return AuthorityEvidence{}, core.Fail(409, "APPROVER_PERMISSION_REQUIRED", "制度指定的审批人缺少申请、业务或附件访问权限，不能自动跳过")
}
