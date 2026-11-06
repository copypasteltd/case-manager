package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"testing"
)

func TestRound168MiniOrdinaryUploads(t *testing.T) {
	for _, channel := range []string{"wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			f := newUploadFixture(t)
			ctx := context.Background()
			configureMiniapps(f.f)
			passwordSession := f.f.login("alice", channel)
			f.f.bindMiniapp(passwordSession.AccessToken, f.f.miniappProof(passwordSession.AccessToken, channel, "alice:bind"))
			f.f.aliceToken = f.f.miniappLogin(channel, "alice:login").AccessToken
			r, v := f.f.request("GET", "/v1/storage/upload-capabilities", f.f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			if data[storage.UploadCapabilities](t, v).MaxBytes != "20971520" {
				t.Fatal("mini bounded capability not projected")
			}
			input := map[string]any{"folder_id": f.root, "name": "ordinary-receipt.bin", "size_bytes": "20971521", "media_type": "application/octet-stream", "source_fingerprint": "source-sha256-v2:" + zeroSHA(1027)}
			r, v = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
			expect(t, r, 422, v)
			input["size_bytes"] = "1027"
			input["source_fingerprint"] = "legacy-opaque-source"
			r, v = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
			expect(t, r, 422, v)
			input["source_fingerprint"] = "source-sha256-v2:" + zeroSHA(1027)
			r, v = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
			expect(t, r, 201, v)
			u := data[storage.Upload](t, v)
			u = f.freezeUpload(u)
			from := scannedUpload(f, u)
			to := newPromotionCloud(f)
			if worked, err := promotionHost(f, from, to).RunOne(ctx); err != nil || !worked {
				t.Fatal("ordinary mini candidate promotion failed", err)
			}
			u = f.get(u.ID)
			configureStorageTargets(f)
			r, v = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
			expect(t, r, 201, v)
			committed := data[storage.CommitResult](t, v)
			if committed.Version.SHA256 != zeroSHA(1027) {
				t.Fatal("mini upload did not retain full original hash")
			}
			var bound bool
			if err := f.f.owner.QueryRow(ctx, `SELECT source_channel=$2 AND committed_version_id=$3 FROM upload_sessions WHERE id=$1`, u.ID, channel, committed.Version.ID).Scan(&bound); err != nil || !bound {
				t.Fatal("mini ordinary commit provenance missing", err)
			}
		})
	}
}
