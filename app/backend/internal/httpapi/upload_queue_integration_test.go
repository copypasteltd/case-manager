package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
)

func TestUploadQueue(t *testing.T) {
	t.Run("owner paging rechecks ACL before limit and binds authorization", func(t *testing.T) {
		f := newUploadFixture(t)
		first, second := f.create("first.bin", 1), f.create("second.bin", 1)
		rec, result := f.f.request("GET", "/v1/uploads?state=all&limit=1", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.Upload]](t, result)
		if len(page.Items) != 1 || page.Items[0].ID != second.ID || page.NextCursor == nil {
			t.Fatal("incorrect first queue page")
		}
		rec, result = f.f.request("GET", "/v1/uploads?state=all&limit=1&cursor="+*page.NextCursor, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		next := data[core.Page[storage.Upload]](t, result)
		if len(next.Items) != 1 || next.Items[0].ID != first.ID || next.NextCursor != nil {
			t.Fatal("incorrect continuation")
		}
		rec, result = f.f.request("GET", "/v1/uploads?state=active&cursor="+*page.NextCursor, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 400, result)
		for _, token := range []string{f.f.bobToken, f.f.administratorToken()} {
			rec, result = f.f.request("GET", "/v1/uploads?state=all", token, "", nil, nil)
			expect(t, rec, 200, result)
			if len(data[core.Page[storage.Upload]](t, result).Items) != 0 {
				t.Fatal("another user's queue disclosed")
			}
		}
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE folders SET state='trashed' WHERE id=$1", f.root); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("GET", "/v1/uploads?state=all", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.Upload]](t, result).Items) != 0 {
			t.Fatal("inaccessible target leaked through queue")
		}
	})
	t.Run("active scope omits expired and terminal sessions and unsupported channels", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("expired.bin", 1)
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE upload_sessions SET expires_at=$2 WHERE id=$1", u.ID, time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("GET", "/v1/uploads", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.Upload]](t, result).Items) != 0 {
			t.Fatal("expired session shown active")
		}
		rec, result = f.f.request("GET", "/v1/uploads?state=all", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.Upload]](t, result).Items) != 1 {
			t.Fatal("expired history unavailable")
		}
		for _, channel := range []string{"wechat", "douyin", "telegram"} {
			a := core.Actor{OrgID: f.f.org, UserID: f.f.alice, Channel: channel}
			if _, err := f.service.List(context.Background(), a, "", "", 20); core.PublicError(err).Code != "UPLOAD_CHANNEL_UNSUPPORTED" {
				t.Fatal("non-upload channel admitted", channel, err)
			}
		}
	})
	t.Run("control configuration and folder capability have distinct meaning", func(t *testing.T) {
		f := newUploadFixture(t)
		rec, result := f.f.request("GET", "/v1/storage/upload-capabilities", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		cap := data[storage.UploadCapabilities](t, result)
		if !cap.ControlReady || cap.MaxBytes != "120000000000" || cap.SessionHours != 48 || cap.ReasonCode != nil {
			t.Fatal("configured control plane unavailable")
		}
		rec, result = f.f.request("GET", "/v1/folders/"+f.root, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if !data[storage.Folder](t, result).CanUpload {
			t.Fatal("uploader lost target capability")
		}
		f.f.api.Storage = nil
		f.f.handler = f.f.api.Handler()
		rec, result = f.f.request(http.MethodGet, "/v1/storage/upload-capabilities", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		cap = data[storage.UploadCapabilities](t, result)
		if cap.ControlReady || cap.ReasonCode == nil || *cap.ReasonCode != "STORAGE_NOT_CONFIGURED" {
			t.Fatal("disabled storage advertised as available")
		}
	})
}
