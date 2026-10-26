package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

type uploadSignerCall func(context.Context, objectstore.Multipart, int, int64, time.Duration) (objectstore.Signed, error)

func (fn uploadSignerCall) SignPart(ctx context.Context, m objectstore.Multipart, n int, size int64, ttl time.Duration) (objectstore.Signed, error) {
	return fn(ctx, m, n, size, ttl)
}

type uploadListerCall func(context.Context, objectstore.Multipart, string) (objectstore.PartPage, error)

func (fn uploadListerCall) Parts(ctx context.Context, m objectstore.Multipart, marker string) (objectstore.PartPage, error) {
	return fn(ctx, m, marker)
}

func TestUploadCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	t.Run("logout during signing returns no fresh permit", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.prepared(f.create("sign-logout.bin", 10))
		a, err := f.f.api.Identity.Authenticate(ctx, f.f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		f.f.api.Storage.Signer = uploadSignerCall(func(ctx context.Context, m objectstore.Multipart, n int, size int64, ttl time.Duration) (objectstore.Signed, error) {
			result, err := f.cloud.SignPart(ctx, m, n, size, ttl)
			if err == nil {
				err = f.f.api.Identity.Logout(ctx, a, core.NewID())
			}
			return result, err
		})
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "part_numbers": []int{1}}, nil)
		expect(t, rec, 412, result)
		if len(result.Data) > 0 && string(result.Data) != "null" {
			t.Fatal("logout still received signing material")
		}
	})
	t.Run("logout during reconciliation prevents receipt replacement", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.prepared(f.create("resume-logout.bin", 10))
		a, err := f.f.api.Identity.Authenticate(ctx, f.f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		f.f.api.Storage.Lister = uploadListerCall(func(ctx context.Context, m objectstore.Multipart, marker string) (objectstore.PartPage, error) {
			page, err := f.cloud.Parts(ctx, m, marker)
			if err == nil {
				err = f.f.api.Identity.Logout(ctx, a, core.NewID())
			}
			return page, err
		})
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/resume", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "source_fingerprint": u.SourceFingerprint}, nil)
		expect(t, rec, 412, result)
		var reconciled int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE resource_id=$1 AND action='upload.parts.reconciled'", u.ID).Scan(&reconciled); err != nil || reconciled != 0 {
			t.Fatal("revoked reconciliation committed", err)
		}
	})
	t.Run("queue and direct metadata hide a changed target classification", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("classified.bin", 10)
		if _, err := f.f.owner.Exec(ctx, "UPDATE folders SET classification='internal' WHERE id=$1", f.root); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("GET", "/v1/uploads/"+u.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("GET", "/v1/uploads?state=all", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.Upload]](t, result).Items) != 0 {
			t.Fatal("changed classification appeared in queue")
		}
	})
	t.Run("committed upload history rechecks the formal file permission before paging", func(t *testing.T) {
		f := newUploadFixture(t)
		u, _, _ := readyUpload(f, "formal-history.bin", 15)
		configureStorageTargets(f)
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		expect(t, rec, 201, result)
		committed := data[storage.CommitResult](t, result)
		rec, result = f.f.request("GET", "/v1/uploads/"+u.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		current := data[storage.Upload](t, result)
		if current.ResultFileID == nil || *current.ResultFileID != committed.File.ID {
			t.Fatal("formal file linkage missing")
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE files SET classification='confidential' WHERE id=$1", committed.File.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("GET", "/v1/uploads/"+u.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("GET", "/v1/uploads?state=all", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.Upload]](t, result).Items) != 0 {
			t.Fatal("restricted committed file leaked through upload history")
		}
	})
}
