package finance

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// BusinessCostBasis pins an approved business head. Its approved amount is a
// ceiling, not an incurred amount, a reserved balance or a payment instruction.
type BusinessCostSource struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Version     int64  `json:"version"`
	RevisionID  string `json:"revision_id"`
	ContentHash string `json:"content_hash"`
}

type BusinessCostBasis struct {
	Source         BusinessCostSource `json:"source"`
	Title          string             `json:"title"`
	ProjectID      *string            `json:"project_id"`
	Classification string             `json:"classification"`
	Currency       string             `json:"currency"`
	ApprovedAmount string             `json:"approved_amount"`
	AsOf           time.Time          `json:"as_of"`
}

type BusinessCostRecord struct {
	Content
	Source         BusinessCostSource `json:"source"`
	SourceKey      string             `json:"source_key"`
	Reason         string             `json:"reason"`
	ActualIncurred bool               `json:"actual_incurred"`
	SourceReviewed bool               `json:"source_reviewed"`
}

func businessCostSource(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, lock bool) (BusinessCostBasis, bool, error) {
	out := BusinessCostBasis{Source: BusinessCostSource{Type: kind, ID: id}}
	if !slices.Contains([]string{"contract", "purchase", "claim"}, kind) || !core.ValidID(id) {
		return out, false, core.Missing()
	}
	query := `SELECT version,COALESCE(approved_revision_id,current_revision_id) FROM spend_documents WHERE org_id=$1 AND id=$2 AND kind=$3`
	args := []any{a.OrgID, id, kind}
	table := "spend_revisions"
	if kind == "contract" {
		query = `SELECT version,COALESCE(active_revision_id,current_revision_id) FROM contracts WHERE org_id=$1 AND id=$2`
		args = args[:2]
		table = "contract_revisions"
	}
	if lock {
		query += " FOR UPDATE"
	}
	if err := db.QueryRow(ctx, query, args...).Scan(&out.Source.Version, &out.Source.RevisionID); err != nil {
		return out, false, platform.DatabaseError(err)
	}
	src, err := sourceInfo(ctx, db, a, kind, id, out.Source.RevisionID, false)
	if err != nil {
		return out, false, err
	}
	out.Title, out.ProjectID, out.Classification, out.Currency = src.Title, src.Project, src.Class, src.Currency
	if src.Amount != nil {
		out.ApprovedAmount = *src.Amount
	}
	// table is chosen from the two fixed literals above; values stay parameters.
	if err = db.QueryRow(ctx, "SELECT content_hash FROM "+table+" WHERE org_id=$1 AND id=$2", a.OrgID, out.Source.RevisionID).Scan(&out.Source.ContentHash); err != nil {
		return out, false, platform.DatabaseError(err)
	}
	if err = db.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, false, err
	}
	return out, src.Valid && src.Amount != nil && src.Currency != "", nil
}

func businessCostAuthority(ctx context.Context, db platform.DBTX, a core.Actor, c Cost, mfa bool) error {
	if err := identity.RequireInteractiveSession(ctx, db, a, mfa); err != nil {
		return err
	}
	for _, action := range []string{"cost.read", "cost.record", "cost.confirm"} {
		if err := authz.Require(ctx, db, a, action, resource(c)); err != nil {
			return err
		}
	}
	return projects(ctx, db, a, c, Content{}, false)
}

func businessCostProspect(a core.Actor, b BusinessCostBasis, id string) Cost {
	return Cost{ID: id, CreatorID: a.UserID, ProjectID: b.ProjectID, Classification: b.Classification}
}

func (s *Service) BusinessCostBasis(ctx context.Context, a core.Actor, kind, id string) (BusinessCostBasis, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return BusinessCostBasis{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var orgVersion, userVersion int64
	var status string
	if err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion); err != nil {
		return BusinessCostBasis{}, platform.DatabaseError(err)
	}
	if err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status); err != nil {
		return BusinessCostBasis{}, platform.DatabaseError(err)
	}
	if orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion || status != "active" {
		return BusinessCostBasis{}, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取当前授权")
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return BusinessCostBasis{}, err
	}
	basis, valid, err := businessCostSource(ctx, tx, a, kind, id, false)
	if err != nil {
		return BusinessCostBasis{}, err
	}
	c := businessCostProspect(a, basis, core.NewID())
	if err = businessCostAuthority(ctx, tx, a, c, false); err != nil {
		return BusinessCostBasis{}, err
	}
	if !valid {
		return BusinessCostBasis{}, core.Fail(409, "COST_SOURCE_NOT_APPROVED", "请选择已批准采购、报销或生效应付合同")
	}
	if !writable(ctx, tx, a, c) {
		return BusinessCostBasis{}, core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许登记实际费用")
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return BusinessCostBasis{}, err
	}
	return basis, platform.DatabaseError(tx.Commit(ctx))
}

// RecordBusinessCost composes ordinary create/confirm/link commands in one
// transaction. Each keeps its existing authority, MFA, journal, references,
// idempotency and audit checks; a failure rolls back all three and their events.
func (s *Service) RecordBusinessCost(ctx context.Context, cmd core.Command, in BusinessCostRecord) (out core.Result, err error) {
	defer func() { out, err = s.replay(ctx, cmd.Actor, out, err) }()
	if !slices.Contains([]string{"contract", "purchase", "claim"}, in.Source.Type) || !core.ValidID(in.Source.ID) || !core.ValidID(in.Source.RevisionID) || core.RequireVersion(in.Source.Version) != nil || len(in.Source.ContentHash) != 64 || strings.Trim(in.Source.ContentHash, "0123456789abcdef") != "" {
		return out, core.Invalid("source", "请固定准确批准业务的类型、版本、修订与摘要")
	}
	if !in.ActualIncurred || !in.SourceReviewed {
		return out, core.Invalid("actual_incurred", "请明确确认实际已发生，并核对准确批准来源；批准金额不自动成为费用")
	}
	prospectID := core.NewID()
	authorize := func(tx pgx.Tx) error {
		if e := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); e != nil {
			return e
		}
		basis, _, e := businessCostSource(ctx, tx, cmd.Actor, in.Source.Type, in.Source.ID, false)
		if e != nil {
			return e
		}
		return businessCostAuthority(ctx, tx, cmd.Actor, businessCostProspect(cmd.Actor, basis, prospectID), true)
	}
	return s.Commands.Do(ctx, cmd, authorize, func(tx pgx.Tx) (core.Result, error) {
		basis, _, e := businessCostSource(ctx, tx, cmd.Actor, in.Source.Type, in.Source.ID, false)
		if e != nil {
			return core.Result{}, e
		}
		// Follow ordinary expense lock order: sorted projects before the source.
		if e = projects(ctx, tx, cmd.Actor, businessCostProspect(cmd.Actor, basis, prospectID), in.Content, true); e != nil {
			return core.Result{}, e
		}
		basis, valid, e := businessCostSource(ctx, tx, cmd.Actor, in.Source.Type, in.Source.ID, true)
		if e != nil {
			return core.Result{}, e
		}
		if basis.Source != in.Source {
			return core.Result{}, core.Fail(409, "COST_SOURCE_CHANGED", "批准业务或执行依据已变更，请重新查阅后核对")
		}
		if !valid || in.Currency == nil || *in.Currency != basis.Currency {
			return core.Result{}, core.Fail(409, "COST_SOURCE_NOT_APPROVED", "请核对有效批准来源及其准确币种")
		}
		if _, e = normalize(ctx, tx, in.Content, true); e != nil {
			return core.Result{}, e
		}
		child := *s
		child.Commands.Tx = tx
		invoke := func(step string, body any) core.Command {
			next := cmd
			next.Route = cmd.Route + "/composed/" + step
			next.Key = "business-cost:" + core.Digest([]byte(cmd.Route+"\n"+cmd.Key+"\n"+step))
			next.Body, _ = json.Marshal(body)
			return next
		}
		create := Create{Content: in.Content, SourceKey: in.SourceKey, ProjectID: basis.ProjectID, Classification: basis.Classification, Reason: in.Reason}
		created, e := child.Create(ctx, invoke("create", create), create)
		if e != nil {
			return core.Result{}, e
		}
		cost := created.Data.(Cost)
		base := Base{ExpectedVersion: cost.Version, RevisionID: cost.CurrentRevisionID, ContentHash: cost.ContentHash, Reason: in.Reason}
		confirmed, e := child.Confirm(ctx, invoke("confirm", base), cost.ID, base)
		if e != nil {
			return core.Result{}, e
		}
		cost = confirmed.Data.(Cost)
		relations := map[string]string{"purchase": "incurred_for", "claim": "reimbursed_via", "contract": "contractual_basis"}
		link := LinkInput{Base: Base{ExpectedVersion: cost.Version, RevisionID: cost.CurrentRevisionID, ContentHash: cost.ContentHash, Reason: in.Reason}, SourceType: in.Source.Type, SourceID: in.Source.ID, SourceRevisionID: in.Source.RevisionID, Relation: relations[in.Source.Type]}
		linked, e := child.Link(ctx, invoke("link", link), cost.ID, link)
		if e != nil {
			return core.Result{}, e
		}
		cost = linked.Data.(Cost)
		return result(cost, "business.recorded", in.Reason, 201), nil
	})
}
