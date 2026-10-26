package contracts

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
)

func hiddenReference(err error) bool {
	status := core.PublicError(err).Status
	return status == 403 || status == 404 || status == 409 || status == 422
}

// References are fixed to the saved revisions. Linked content still requires
// its own current access policy; unavailable content yields no hidden name.
func references(ctx context.Context, db platform.DBTX, a core.Actor, scope Contract, value Content, partyRef **PartyReference, fileRef **fileref.Evidence) error {
	if value.CounterpartyID != nil && value.CounterpartyRevisionID != nil {
		_, r, err := party.ResolveBusinessReference(ctx, db, a, *value.CounterpartyID, *value.CounterpartyRevisionID, false)
		if err == nil {
			*partyRef = &PartyReference{ID: *value.CounterpartyID, RevisionID: r.ID, Name: r.Name, Kind: r.Kind}
		} else if !hiddenReference(err) {
			return err
		}
	}
	if value.DraftFileVersionID != nil {
		v, err := fileref.Resolve(ctx, db, a, *value.DraftFileVersionID, scope.ProjectID, scope.Classification)
		if err == nil {
			*fileRef = &v
		} else if !hiddenReference(err) {
			return err
		}
	}
	return nil
}
