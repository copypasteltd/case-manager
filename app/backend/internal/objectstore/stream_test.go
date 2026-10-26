package objectstore

import (
	"context"
	"io"
	"testing"
)

type streamStore struct {
	Store
	read  func(context.Context, Source, int64, int64) (ReadResult, error)
	write func(context.Context, Multipart, int, io.Reader, int64) (Part, error)
}

func (s streamStore) Read(ctx context.Context, source Source, offset, length int64) (ReadResult, error) {
	return s.read(ctx, source, offset, length)
}
func (s streamStore) WritePart(ctx context.Context, m Multipart, n int, body io.Reader, size int64) (Part, error) {
	return s.write(ctx, m, n, body, size)
}
func TestFrozenSourceHashUsesRangesAndClosesEveryBody(t *testing.T) {
	size := 2*DefaultPartBytes + 123
	var bodies []*trackedBody
	var offsets []int64
	source := Source{Key: fixtureKey(), ETag: "opaque-multipart-etag", Size: size}
	store := streamStore{read: func(ctx context.Context, s Source, offset, length int64) (ReadResult, error) {
		if s != source || length > DefaultPartBytes {
			t.Fatal("unbounded or unpinned source read")
		}
		offsets = append(offsets, offset)
		body := &trackedBody{Reader: &boundedReader{remaining: length}}
		bodies = append(bodies, body)
		return ReadResult{Body: body, Size: length, ETag: s.ETag}, nil
	}}
	result, err := HashSource(context.Background(), store, source)
	expected, expectedError := HashExact(context.Background(), &boundedReader{remaining: size}, size)
	if err != nil || expectedError != nil || result != expected || len(offsets) != 3 || offsets[1] != DefaultPartBytes || offsets[2] != 2*DefaultPartBytes {
		t.Fatal("whole source hash did not match exact content", result, err)
	}
	for _, body := range bodies {
		if !body.closed {
			t.Fatal("range body leaked")
		}
	}
}
func TestTransferRequiresSourceConsumptionBeforeRecordingPart(t *testing.T) {
	var body *trackedBody
	consume := true
	source := Source{Key: fixtureKey(), ETag: "opaque-etag", Size: DefaultPartBytes + 7}
	layout, err := Plan(source.Size, 0)
	if err != nil {
		t.Fatal(err)
	}
	from := streamStore{read: func(ctx context.Context, s Source, offset, length int64) (ReadResult, error) {
		if offset != DefaultPartBytes || length != 7 {
			t.Fatal("copy requested a different range")
		}
		body = &trackedBody{Reader: &boundedReader{remaining: length}}
		return ReadResult{Body: body, Size: length, ETag: s.ETag}, nil
	}}
	to := streamStore{write: func(ctx context.Context, m Multipart, n int, reader io.Reader, size int64) (Part, error) {
		if consume {
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Fatal(err)
			}
		}
		return Part{Number: n, Size: size, ETag: "cloud-part"}, nil
	}}
	target := Multipart{Key: fixtureKey(), UploadID: "replica-upload"}
	if _, err = TransferPart(context.Background(), from, to, source, target, layout, 2); err != nil || !body.closed {
		t.Fatal("bounded transfer failed or leaked body", err)
	}
	consume = false
	if _, err = TransferPart(context.Background(), from, to, source, target, layout, 2); err == nil || !body.closed {
		t.Fatal("unconsumed source falsely recorded as copied")
	}
}
