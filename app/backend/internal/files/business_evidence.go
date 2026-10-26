package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// VerifyBusinessEvidence applies the same exact report and current waiver
// checks used for restoring content. It never turns a waiver into clean.
func VerifyBusinessEvidence(ctx context.Context, db platform.DBTX, org, file, version, classification string) error {
	_, err := restoreOriginEvidence(ctx, db, org, file, version, classification)
	if err != nil && core.PublicError(err).Code == "RESTORE_SCAN_EVIDENCE_REQUIRED" {
		return core.Fail(409, "BUSINESS_SCAN_EVIDENCE_REQUIRED", "正文缺少完整扫描依据，或原扫描例外已失效，需先重新核验")
	}
	return err
}
