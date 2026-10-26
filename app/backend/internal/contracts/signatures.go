package contracts

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func signatureMetadata(ctx context.Context, db platform.DBTX, org string, d Contract) (*Signature, error) {
	if d.ApprovedRevisionID == nil {
		return nil, nil
	}
	var v Signature
	err := db.QueryRow(ctx, `SELECT id,revision_id,request_id,request_revision_id,instance_id,signed_on::text,effective_on::text,
        signed_file_version_id,attestation,reason,actor_id,created_at FROM contract_signatures WHERE org_id=$1 AND contract_id=$2 AND revision_id=$3`,
		org, d.ID, *d.ApprovedRevisionID).Scan(&v.ID, &v.RevisionID, &v.RequestID, &v.RequestRevisionID, &v.InstanceID, &v.SignedOn, &v.EffectiveOn,
		&v.SignedFileVersionID, &v.Attestation, &v.Reason, &v.ActorID, &v.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, err
}
func signatureEvidence(ctx context.Context, db platform.DBTX, a core.Actor, d Contract, v *Signature) error {
	if v == nil {
		return nil
	}
	v.SignedFile = nil
	evidence, err := fileref.Resolve(ctx, db, a, v.SignedFileVersionID, d.ProjectID, d.Classification)
	if err == nil {
		v.SignedFile = &evidence
		return nil
	}
	if hiddenReference(err) {
		return nil
	}
	return err
}
func todayHK(ctx context.Context, db platform.DBTX) (string, error) {
	var today string
	err := db.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today)
	return today, err
}
func signedStage(d Contract, revision, hash string) error {
	if d.DeletedAt != nil || d.Lifecycle != "pending_signature" || d.ApprovalState != "approved" || d.ApprovedRevisionID == nil || d.RequestID == nil || d.StartDate == nil ||
		*d.ApprovedRevisionID != revision || d.CurrentRevisionID != revision || d.ContentHash != hash {
		return core.Fail(409, "CONTRACT_APPROVED_REVISION_REQUIRED", "请核对待签署合同的准确批准修订")
	}
	return nil
}
func validateExecutionBase(revision, hash, reason string) error {
	if !core.ValidID(revision) || (len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "") || core.Name(reason) != nil {
		return core.Invalid("revision_id", "请固定准确批准修订与摘要，并说明操作依据")
	}
	return nil
}

// Approved content remains immutable; current access and evidence are checked
// again before recording a new real-world signature or activation fact.
func executionReferences(ctx context.Context, tx pgx.Tx, a core.Actor, d Contract) error {
	if d.DraftFileVersionID == nil || d.CounterpartyID == nil || d.CounterpartyRevisionID == nil {
		return core.Fail(409, "CONTRACT_APPROVED_REVISION_REQUIRED", "批准内容缺少完整依据")
	}
	if _, _, err := party.ResolveBusinessReference(ctx, tx, a, *d.CounterpartyID, *d.CounterpartyRevisionID, false); err != nil {
		return err
	}
	_, err := fileref.ForApproval(ctx, tx, a, *d.DraftFileVersionID, d.ProjectID, d.Classification)
	return err
}

func (s *Service) RegisterSignature(ctx context.Context, c core.Command, id string, in SignatureInput) (core.Result, error) {
	in.RevisionID = strings.ToLower(in.RevisionID)
	in.SignedFileVersionID = strings.ToLower(in.SignedFileVersionID)
	in.Attestation = strings.TrimSpace(in.Attestation)
	if err := validateExecutionBase(in.RevisionID, in.ContentHash, in.Reason); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(in.SignedFileVersionID) || in.SignedOn == "" || core.Date(in.SignedOn) != nil || len([]rune(in.Attestation)) < 1 || len([]rune(in.Attestation)) > 20000 {
		return core.Result{}, core.Invalid("signature", "请登记真实签署日期、准确签署件版本及条款核对说明")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, "contract.execute", func(tx pgx.Tx, d Contract) (core.Result, error) {
		if err := signedStage(d, in.RevisionID, in.ContentHash); err != nil {
			return core.Result{}, err
		}
		prior, err := signatureMetadata(ctx, tx, c.Actor.OrgID, d)
		if err != nil {
			return core.Result{}, err
		}
		if prior != nil {
			return core.Result{}, core.Fail(409, "CONTRACT_SIGNATURE_EXISTS", "此批准修订已有不可变签署登记")
		}
		today, err := todayHK(ctx, tx)
		if err != nil {
			return core.Result{}, err
		}
		if in.SignedOn > today {
			return core.Result{}, core.Invalid("signed_on", "签署日期不能晚于香港当前日期")
		}
		if err = executionReferences(ctx, tx, c.Actor, d); err != nil {
			return core.Result{}, err
		}
		if _, err = fileref.ForApproval(ctx, tx, c.Actor, in.SignedFileVersionID, d.ProjectID, d.Classification); err != nil {
			return core.Result{}, err
		}
		var requestRevision, instance string
		if err = tx.QueryRow(ctx, `SELECT r.current_revision_id,r.active_instance_id FROM requests r JOIN request_revisions v ON v.org_id=r.org_id AND v.id=r.current_revision_id
            JOIN approval_instances i ON i.org_id=r.org_id AND i.id=r.active_instance_id WHERE r.org_id=$1 AND r.id=$2 AND r.kind='contract' AND r.business_id=$3
            AND r.state='approved' AND i.state='approved' AND v.business_revision_id=$4 AND v.business_content_hash=$5`, c.Actor.OrgID, *d.RequestID, id, in.RevisionID, in.ContentHash).Scan(&requestRevision, &instance); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO contract_signatures(id,org_id,contract_id,revision_id,request_id,request_revision_id,instance_id,signed_on,effective_on,signed_file_version_id,attestation,reason,actor_id,session_id,mfa_verified_at)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,s.id,s.mfa_verified_at FROM sessions s WHERE s.org_id=$2 AND s.id=$14`,
			core.NewID(), c.Actor.OrgID, id, in.RevisionID, *d.RequestID, requestRevision, instance, in.SignedOn, *d.StartDate, in.SignedFileVersionID, in.Attestation, in.Reason, c.Actor.UserID, c.Actor.SessionID)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE contracts SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		current, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		current, err = present(ctx, tx, c.Actor, current)
		return result(current, "contract.signature.registered", 201, in.Reason), err
	})
}

func (s *Service) Activate(ctx context.Context, c core.Command, id string, in ActivationInput) (core.Result, error) {
	in.RevisionID = strings.ToLower(in.RevisionID)
	in.SignatureID = strings.ToLower(in.SignatureID)
	if err := validateExecutionBase(in.RevisionID, in.ContentHash, in.Reason); err != nil {
		return core.Result{}, err
	}
	if in.Action != "activate" || !core.ValidID(in.SignatureID) {
		return core.Result{}, core.Invalid("action", "请明确按固定签署登记确认生效")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, "contract.execute", func(tx pgx.Tx, d Contract) (core.Result, error) {
		if err := signedStage(d, in.RevisionID, in.ContentHash); err != nil {
			return core.Result{}, err
		}
		signature, err := signatureMetadata(ctx, tx, c.Actor.OrgID, d)
		if err != nil {
			return core.Result{}, err
		}
		if signature == nil || signature.ID != in.SignatureID {
			return core.Result{}, core.Fail(409, "CONTRACT_SIGNATURE_REQUIRED", "合同生效须有准确批准修订的签署登记")
		}
		today, err := todayHK(ctx, tx)
		if err != nil {
			return core.Result{}, err
		}
		if signature.EffectiveOn > today {
			return core.Result{}, core.Fail(409, "CONTRACT_NOT_YET_EFFECTIVE", "尚未到已批准的合同生效日期")
		}
		if err = executionReferences(ctx, tx, c.Actor, d); err != nil {
			return core.Result{}, err
		}
		if _, err = fileref.ForApproval(ctx, tx, c.Actor, signature.SignedFileVersionID, d.ProjectID, d.Classification); err != nil {
			return core.Result{}, err
		}
		if d.ParentContractID != nil {
			parent, err := metadata(ctx, tx, c.Actor.OrgID, *d.ParentContractID)
			if err != nil {
				return core.Result{}, err
			}
			if _, err = present(ctx, tx, c.Actor, parent); err != nil {
				return core.Result{}, err
			}
			if parent.DeletedAt != nil || parent.Lifecycle != "effective" {
				return core.Result{}, core.Fail(409, "CONTRACT_PARENT_NOT_EFFECTIVE", "补充协议生效前须核对主合同处于有效阶段")
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO contract_lifecycle_events(id,org_id,contract_id,revision_id,signature_id,from_state,to_state,reason,actor_id,session_id,mfa_verified_at)
            SELECT $1,$2,$3,$4,$5,'pending_signature','effective',$6,$7,s.id,s.mfa_verified_at FROM sessions s WHERE s.org_id=$2 AND s.id=$8`,
			core.NewID(), c.Actor.OrgID, id, in.RevisionID, in.SignatureID, in.Reason, c.Actor.UserID, c.Actor.SessionID)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE contracts SET lifecycle='effective',active_revision_id=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, in.RevisionID); err != nil {
			return core.Result{}, err
		}
		current, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		current, err = present(ctx, tx, c.Actor, current)
		return result(current, "contract.activated", 200, in.Reason), err
	})
}
