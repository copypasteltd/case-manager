package httpapi

import (
	"context"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
)

func TestUploadFullSourceExpectation(t *testing.T) {
	for _, match := range []bool{true, false} {
		name := "mismatch"
		if match {
			name = "match"
		}
		t.Run(name, func(t *testing.T) {
			f := newUploadFixture(t)
			ctx := context.Background()
			expected := zeroSHA(1027)
			if !match {
				expected = core.Digest([]byte("different original with same size and sampled edges"))
			}
			rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{"folder_id": f.root, "name": "source.bin", "size_bytes": "1027", "media_type": "application/octet-stream", "source_fingerprint": "source-sha256-v2:" + expected}, nil)
			expect(t, rec, 201, result)
			u := f.freezeUpload(data[storage.Upload](t, result))
			cloud := &integrityCloud{uploadCloudFixture: f.cloud}
			if worked, err := integrityHost(f, cloud, filescan.Disabled{}).RunOne(ctx); err != nil || !worked {
				t.Fatal(err)
			}
			current := f.get(u.ID)
			if !match {
				if current.State != "failed" || current.ErrorCode == nil || *current.ErrorCode != "UPLOAD_SOURCE_HASH_MISMATCH" || current.ScanResultID != nil {
					t.Fatal("different original advanced to scan or promotion")
				}
			} else if current.ScanResultID == nil || current.SHA256 == nil || *current.SHA256 != expected {
				t.Fatal("matching full source did not advance to ordinary scanner policy")
			}
		})
	}
}
