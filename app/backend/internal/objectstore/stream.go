package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// HashSource pins every bounded range to the same frozen source. It never
// relies on an ETag, multipart checksum or supplied metadata as the SHA-256.
func HashSource(ctx context.Context, store Store, source Source) (Digest, error) {
	if store == nil || !validKey(source.Key) || !validETag(source.ETag) || source.Size < 0 || source.Size > MaxObjectBytes {
		return Digest{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	hash := sha256.New()
	buffer := make([]byte, 256<<10)
	for offset := int64(0); offset < source.Size; {
		length := DefaultPartBytes
		if source.Size-offset < length {
			length = source.Size - offset
		}
		read, err := store.Read(ctx, source, offset, length)
		if err != nil {
			return Digest{}, err
		}
		if read.Body == nil || read.Size != length || read.ETag != source.ETag || read.VersionID != source.VersionID {
			if read.Body != nil {
				_ = read.Body.Close()
			}
			return Digest{}, invalid("SOURCE_EVIDENCE_MISMATCH")
		}
		count, copyError := io.CopyBuffer(hash, io.LimitReader(contextReader{ctx, read.Body}, length+1), buffer)
		closeError := read.Body.Close()
		if copyError != nil || closeError != nil {
			return Digest{}, &Error{Code: "SOURCE_READ_FAILED", Retryable: ctx.Err() == nil}
		}
		if count != length {
			return Digest{}, invalid("SOURCE_SIZE_MISMATCH")
		}
		offset += length
	}
	if ctx.Err() != nil {
		return Digest{}, invalid("SOURCE_READ_CANCELLED")
	}
	return Digest{SHA256: hex.EncodeToString(hash.Sum(nil)), Size: source.Size}, nil
}

type countedReader struct {
	source io.Reader
	count  int64
}

func (r *countedReader) Read(buffer []byte) (int, error) {
	n, err := r.source.Read(buffer)
	r.count += int64(n)
	return n, err
}

// TransferPart copies one exact source range to a known destination multipart.
// The caller records the returned ETag under its valid job lease and fencing
// token. This primitive does not mark a replica verified or complete a job.
func TransferPart(ctx context.Context, sourceStore, targetStore Store, source Source, target Multipart, layout Layout, number int) (Part, error) {
	if sourceStore == nil || targetStore == nil || layout.Size != source.Size {
		return Part{}, invalid("INVALID_TRANSFER")
	}
	length, err := layout.PartSize(number)
	if err != nil {
		return Part{}, err
	}
	if err = validatePart(target, number, length); err != nil {
		return Part{}, err
	}
	offset := int64(number-1) * layout.PartBytes
	read, err := sourceStore.Read(ctx, source, offset, length)
	if err != nil {
		return Part{}, err
	}
	if read.Body == nil || read.Size != length || read.ETag != source.ETag || read.VersionID != source.VersionID {
		if read.Body != nil {
			_ = read.Body.Close()
		}
		return Part{}, invalid("SOURCE_EVIDENCE_MISMATCH")
	}
	counted := &countedReader{source: contextReader{ctx, io.LimitReader(read.Body, length)}}
	part, err := targetStore.WritePart(ctx, target, number, counted, length)
	if err != nil {
		_ = read.Body.Close()
		return Part{}, err
	}
	var extra [1]byte
	extraN, extraErr := read.Body.Read(extra[:])
	closeErr := read.Body.Close()
	if ctx.Err() != nil {
		return Part{}, &Error{Code: "SOURCE_READ_CANCELLED", Retryable: true}
	}
	if closeErr != nil || extraN != 0 || extraErr != io.EOF || counted.count != length || part.Number != number || part.Size != length || !validETag(part.ETag) {
		return Part{}, invalid("TRANSFER_EVIDENCE_MISMATCH")
	}
	return part, nil
}
