package contracts

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"encoding/json"
)

// The operation's business revision remains fixed. Derived linked names and
// action capabilities must still respect today's access and business state.
func (s *Service) replay(ctx context.Context, a core.Actor, result core.Result, err error) (core.Result, error) {
	if err != nil || !result.Replayed {
		return result, err
	}
	raw, e := json.Marshal(result.Data)
	if e != nil {
		return core.Result{}, e
	}
	var d Contract
	if e = json.Unmarshal(raw, &d); e != nil {
		return core.Result{}, e
	}
	d.Counterparty = nil
	d.DraftFile = nil
	d.CanRegisterSignature = false
	d.CanActivate = false
	d.CanEdit = false
	d.CanPrepareRequest = false
	d.CanReadHistory = false
	d.CanTrash = false
	current, e := metadata(ctx, s.DB, a.OrgID, d.ID)
	if e != nil {
		return core.Result{}, e
	}
	current, e = present(ctx, s.DB, a, current)
	if e != nil {
		return core.Result{}, e
	}
	if current.Version == d.Version {
		d.CanRegisterSignature = current.CanRegisterSignature
		d.CanActivate = current.CanActivate
		d.CanEdit = current.CanEdit
		d.CanPrepareRequest = current.CanPrepareRequest
		d.CanTrash = current.CanTrash
	}
	d.CanReadHistory = current.CanReadHistory
	if e = references(ctx, s.DB, a, d, d.Content, &d.Counterparty, &d.DraftFile); e != nil {
		return core.Result{}, e
	}
	if e = signatureEvidence(ctx, s.DB, a, d, d.Signature); e != nil {
		return core.Result{}, e
	}
	result.Data = d
	return result, nil
}
