package statusbridge

import (
	"crypto/ed25519"
	"encoding/base64"
	"strconv"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

func (j *journal) validResourceCheckpoint(next checkpoint) bool {
	if next.Generation == 0 || next.Binding != j.binding || next.Sequence < 1 || next.Sequence > 9007199254740991 {
		return false
	}
	wire, e := servicestatus.ParseResourceObservation(next.Body)
	if e != nil || wire.RevisionID != j.target.RevisionID || wire.Sequence != strconv.FormatInt(next.Sequence, 10) || !wire.ObservedAt.Equal(next.ObservedAt) ||
		wire.ResourceMetrics.Validate(resourceMounts(j.target)) != nil || resourceSampleDigest(wire.ObservedAt, wire.ResourceMetrics) != next.SampleSHA {
		return false
	}
	sig, e := base64.StdEncoding.Strict().DecodeString(next.Signature)
	if e != nil || !ed25519.Verify(j.key.Public().(ed25519.PublicKey), servicestatus.ResourceSigningMessage(j.target.SourceID, next.Body), sig) {
		return false
	}
	return next.Ack == nil || validAck(*next.Ack, j.target, wire.Sequence, core.Digest(next.Body))
}
