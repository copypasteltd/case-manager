package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

// Small in-memory provider used only by this explicit test. The target keeps
// bytes actually received by WritePart; it never substitutes a client digest.
type contentFixtureCloud struct {
	*promotionCloud
	input  []byte
	stored map[string]map[int][]byte
}

func (c *contentFixtureCloud) WritePart(ctx context.Context, m objectstore.Multipart, n int, r io.Reader, size int64) (objectstore.Part, error) {
	body, err := io.ReadAll(io.LimitReader(r, size+1))
	if err != nil || int64(len(body)) != size {
		return objectstore.Part{}, fmt.Errorf("invalid owned fixture write")
	}
	if c.stored[m.Key] == nil {
		c.stored[m.Key] = map[int][]byte{}
	}
	c.stored[m.Key][n] = body
	return c.promotionCloud.WritePart(ctx, m, n, bytes.NewReader(body), size)
}
func (c *contentFixtureCloud) Read(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
	content := c.input
	if content == nil {
		for n := 1; n <= len(c.stored[source.Key]); n++ {
			content = append(content, c.stored[source.Key][n]...)
		}
	}
	if offset < 0 || length < 0 || offset+length > int64(len(content)) {
		return objectstore.ReadResult{}, fmt.Errorf("invalid owned fixture range")
	}
	return objectstore.ReadResult{Body: io.NopCloser(bytes.NewReader(content[offset : offset+length])), Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
}
func commitContentFixture(t *testing.T, f *uploadFixture, name, mime string, body []byte, base *storage.CommitResult) (storage.CommitResult, *contentFixtureCloud) {
	t.Helper()
	in := map[string]any{"folder_id": f.root, "name": name, "size_bytes": strconv.Itoa(len(body)), "media_type": mime, "source_fingerprint": "owned-content/" + core.NewID()}
	if base != nil {
		in["file_id"] = base.File.ID
		in["base_version_id"] = base.Version.ID
	}
	rec, res := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), in, nil)
	expect(t, rec, 201, res)
	u := f.freezeUpload(data[storage.Upload](t, res))
	from := &contentFixtureCloud{promotionCloud: &promotionCloud{integrityCloud: &integrityCloud{uploadCloudFixture: f.cloud}}, input: body, stored: map[string]map[int][]byte{}}
	to := &contentFixtureCloud{promotionCloud: newPromotionCloud(f), stored: map[string]map[int][]byte{}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	verify := &uploadworker.Runner{Service: f.service, Store: from, Owner: "owned-content-verify", Mode: "verify-small", Scanner: cleanIntegrityScanner(), Logger: logger}
	if worked, err := verify.RunOne(context.Background()); err != nil || !worked {
		t.Fatal("owned content hash and scan", worked, err)
	}
	promote := &uploadworker.Runner{Service: f.service, Store: from, TargetStore: to, Owner: "owned-content-promote", Mode: "promote-small", Logger: logger}
	if worked, err := promote.RunOne(context.Background()); err != nil || !worked {
		t.Fatal("owned content promotion", worked, err)
	}
	u = f.get(u.ID)
	if u.SHA256 == nil || *u.SHA256 != contentSHA(body) {
		t.Fatal("authoritative hash did not read actual fixture bytes")
	}
	rec, res = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
	expect(t, rec, 201, res)
	return data[storage.CommitResult](t, res), to
}

func smallContentPDF() []byte {
	// Actual one-page PDF with a large comment stream, exercising two 1MiB
	// ranges without a native PDF renderer or real cloud in this test.
	stream := "BT /F1 16 Tf 30 100 Td (CopyPaste fixed file review) Tj ET\n" + strings.Repeat("% fixed-content-padding\n", 47000)
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 360 180] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)}
	var result bytes.Buffer
	result.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, result.Len())
		fmt.Fprintf(&result, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := result.Len()
	fmt.Fprintf(&result, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&result, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&result, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return result.Bytes()
}

func TestMobileContentBrowserFixture(t *testing.T) {
	f := newUploadFixture(t)
	configureStorageTargets(f)
	allowMobileContent(f)
	files := map[string]any{}
	readers := map[string]*contentFixtureCloud{}
	special := map[string]string{}
	add := func(key, name, mime string, body []byte, base *storage.CommitResult) storage.CommitResult {
		committed, cloud := commitContentFixture(t, f, name, mime, body, base)
		for storedKey := range cloud.stored {
			readers[storedKey] = cloud
			if key == "delayed" || key == "truncated" {
				special[storedKey] = key
			}
		}
		files[key] = map[string]any{"file_id": committed.File.ID, "version_id": committed.Version.ID, "seq": committed.Version.Seq, "sha256": committed.Version.SHA256, "size_bytes": committed.Version.SizeBytes, "name": name}
		return committed
	}
	historical := add("historical", "历史固定依据.txt", "text/plain", []byte("香港项目 · 固定版本一\n🙂 请逐项核对。\n<script>window.__cpFileScriptExecuted=true</script>\n这是原文，不执行标记。"), nil)
	current := add("current", "后续修订.txt", "text/plain", []byte("这是后续的当前版本，不能代替历史固定依据。"), &historical)
	files["historical"].(map[string]any)["current_version_id"] = current.Version.ID
	canvas := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			c := color.RGBA{246, 246, 244, 255}
			if x >= 24 && x < 296 && y >= 24 && y < 156 {
				c = color.RGBA{21, 23, 26, 255}
			}
			if x >= 24 && x < 32 && y >= 24 && y < 156 {
				c = color.RGBA{31, 94, 255, 255}
			}
			canvas.Set(x, y, c)
		}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, canvas); err != nil {
		t.Fatal(err)
	}
	add("image", "固定版本图片.png", "image/png", picture.Bytes(), nil)
	add("pdf", "固定版本文档.pdf", "application/pdf", smallContentPDF(), nil)
	add("unsupported", "未支持格式.bin", "application/octet-stream", []byte("raw binary fixture"), nil)
	add("large_text", "超出阅读范围.txt", "text/plain", bytes.Repeat([]byte("x"), mobileChunkBytes+1), nil)
	add("truncated", "未完整读取.txt", "text/plain", bytes.Repeat([]byte("truncated-test\n"), 6000), nil)
	add("delayed", "可取消读取.txt", "text/plain", []byte("这段内容只应在读取完成后出现。"), nil)
	cloud := &downloadCloud{readFn: func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		if special[source.Key] == "delayed" {
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return objectstore.ReadResult{}, ctx.Err()
			}
		}
		reader, ok := readers[source.Key]
		if !ok {
			return objectstore.ReadResult{}, fmt.Errorf("unknown owned fixture object")
		}
		result, err := reader.Read(ctx, source, offset, length)
		if err == nil && special[source.Key] == "truncated" {
			result.Body = io.NopCloser(io.LimitReader(result.Body, length-1))
		}
		return result, err
	}}
	f.f.api.Downloads = map[string]*storage.DownloadBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", Signer: cloud, Reader: cloud}, "r2": {Provider: "r2", Bucket: "fixture-r2-bucket", Signer: cloud, Reader: cloud}}
	f.f.handler = f.f.api.Handler()
	mobile := f.f.login("bob", "wechat").AccessToken
	for _, key := range []string{"historical", "current", "image", "pdf", "large_text"} {
		entry := files[key].(map[string]any)
		rec, res := f.f.request("POST", "/v1/file-versions/"+entry["version_id"].(string)+"/download", mobile, core.NewID(), map[string]string{"disposition": "attachment"}, nil)
		expect(t, rec, 200, res)
		grant := data[storage.DownloadResult](t, res)
		total, _ := strconv.ParseInt(grant.SizeBytes, 10, 64)
		var original bytes.Buffer
		for offset := int64(0); offset < total; {
			length := min(int64(mobileChunkBytes), total-offset)
			rec, res = f.f.request("GET", fmt.Sprintf("/v1/downloads/%s/chunks?offset=%d&length=%d", grant.ID, offset, length), mobile, "", nil, map[string]string{"If-Range": `"` + grant.SHA256 + `"`})
			expect(t, rec, 200, res)
			chunk := data[downloadChunk](t, res)
			original.Write(chunk.Data)
			offset += int64(len(chunk.Data))
		}
		if contentSHA(original.Bytes()) != grant.SHA256 {
			t.Fatal("round trip did not preserve the actual committed bytes", key)
		}
	}
	// Enroll an actual disposable MFA credential before handing the owned fixture
	// to the UI. Ordinary content reads do not themselves require a step-up.
	if _, err := f.f.owner.Exec(context.Background(), "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='contract_operator'", core.NewID(), f.f.org, f.f.alice); err != nil {
		t.Fatal(err)
	}
	f.f.handler = f.f.api.Handler()
	f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
	holdBrowserBusinessFixture(t, f.f, map[string]any{"content_fixture": true, "content_files": files, "project_id": historical.File.ProjectID, "file_id": historical.File.ID, "file_version_id": historical.Version.ID, "file_name": historical.File.Name})
}
