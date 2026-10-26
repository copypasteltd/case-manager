package finance

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"github.com/jackc/pgx/v5"
)

// Detailed snapshots disclose more than aggregates. Their private catalogue
// retains the exact fixed files, parties, visible business links and settlement
// authority so later revocation cannot leave a richer old projection readable.
func validateReviewDependency(ctx context.Context, tx pgx.Tx, a core.Actor, class string, ref AgentSummaryEvidence) (bool, error) {
	switch ref.Type {
	case "file_version":
		e, err := fileref.Resolve(ctx, tx, a, ref.RevisionID, ref.DependencyProjectID, class)
		if err == nil && e.FileID != ref.ID {
			err = core.Missing()
		}
		return true, err
	case "party":
		p, _, err := party.ResolveBusinessReference(ctx, tx, a, ref.ID, ref.RevisionID, false)
		if err == nil && financeClassRank(p.Classification) > financeClassRank(class) {
			err = core.Missing()
		}
		return true, err
	case "contract", "purchase", "claim":
		v, err := sourceInfo(ctx, tx, a, ref.Type, ref.ID, ref.RevisionID, false)
		if err == nil && financeClassRank(v.Class) > financeClassRank(class) {
			err = core.Missing()
		}
		return true, err
	case "cost_settlement":
		c, err := metadata(ctx, tx, a.OrgID, ref.ID)
		if err == nil {
			err = costSettlement(ctx, tx, a, &c)
		}
		if err == nil && c.SettlementVisibility != "complete" {
			err = core.Missing()
		}
		return true, err
	}
	return false, nil
}

func reviewDependencies(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, record any) ([]AgentSummaryEvidence, error) {
	refs := []AgentSummaryEvidence{}
	var files []fileref.Evidence
	var supplier *PartyEvidence
	var project *string
	switch v := record.(type) {
	case Subscription:
		files, supplier, project = v.Attachments, v.SupplierEvidence, v.ProjectID
	case Payment:
		files, supplier, project = v.Attachments, v.CounterpartyEvidence, v.ProjectID
	case Cost:
		files, supplier, project = v.Attachments, v.CounterpartyEvidence, v.ProjectID
		for _, link := range v.Links {
			if link.Title != nil && link.SourceType != "subscription_period" {
				refs = append(refs, AgentSummaryEvidence{Type: link.SourceType, ID: link.SourceID, RevisionID: link.SourceRevisionID})
			}
		}
		if v.SettlementVisibility == "complete" {
			refs = append(refs, AgentSummaryEvidence{Type: "cost_settlement", ID: v.ID, RevisionID: v.CurrentRevisionID})
			rows, err := tx.Query(ctx, `SELECT DISTINCT p.id::text,p.current_revision_id::text FROM payment_allocations x JOIN payment_records p ON p.org_id=x.org_id AND p.id=x.payment_id WHERE x.org_id=$1 AND x.cost_id=$2 LIMIT 201`, a.OrgID, v.ID)
			if err != nil {
				return nil, err
			}
			count := 0
			for rows.Next() {
				var ref AgentSummaryEvidence
				ref.Type = "payment"
				if err = rows.Scan(&ref.ID, &ref.RevisionID); err != nil {
					break
				}
				refs = append(refs, ref)
				count++
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			if err != nil {
				return nil, err
			}
			if err = reviewBounded(count, 200); err != nil {
				return nil, err
			}
		}
	}
	for _, f := range files {
		refs = append(refs, AgentSummaryEvidence{Type: "file_version", ID: f.FileID, RevisionID: f.VersionID, DependencyProjectID: project})
	}
	if supplier != nil {
		refs = append(refs, AgentSummaryEvidence{Type: "party", ID: supplier.PartyID, RevisionID: supplier.RevisionID})
	}
	return refs, nil
}
