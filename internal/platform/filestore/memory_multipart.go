package filestore

// Multipart sessions on the in-process store: parts buffer in memory and
// completion concatenates them into the final blob, so dev mode exercises
// the same client flow production does without an S3 endpoint.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

type memoryMultipart struct {
	key         string
	contentType string
	parts       map[int32][]byte
	initiated   time.Time
}

// CreateMultipart opens an in-memory session.
func (m *Memory) CreateMultipart(_ context.Context, key, contentType string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.multiparts == nil {
		m.multiparts = map[string]*memoryMultipart{}
	}
	uploadID := kernel.NewID()
	m.multiparts[uploadID] = &memoryMultipart{
		key:         key,
		contentType: contentType,
		parts:       map[int32][]byte{},
		initiated:   m.now(),
	}
	return uploadID, nil
}

// PresignPart has no URL to hand out in dev mode; clients PUT the part
// through PutMultipartPart instead (the HTTP layer routes dev clients to
// the streaming variant when presigns are unavailable).
func (m *Memory) PresignPart(context.Context, string, string, int32, time.Duration) (string, error) {
	return "", ErrNotSupported
}

// PutMultipartPart buffers one part (the dev-mode counterpart of a
// presigned part URL) and returns its checksum as the part ETag.
func (m *Memory) PutMultipartPart(_ context.Context, uploadID string, partNumber int32, r io.Reader) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.multiparts[uploadID]
	if !ok {
		return "", ErrNotFound
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	session.parts[partNumber] = data
	return sha256Hex(data), nil
}

// CompleteMultipart concatenates the listed parts into the final blob.
func (m *Memory) CompleteMultipart(_ context.Context, key, uploadID string, parts []CompletedPart) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.multiparts[uploadID]
	if !ok || session.key != key {
		return ErrNotFound
	}
	if len(parts) == 0 {
		return fmt.Errorf("multipart complete: no parts listed")
	}
	ordered := make([]CompletedPart, len(parts))
	copy(ordered, parts)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].PartNumber < ordered[j].PartNumber })
	var buf bytes.Buffer
	for _, part := range ordered {
		data, ok := session.parts[part.PartNumber]
		if !ok {
			return fmt.Errorf("multipart complete: part %d was never uploaded", part.PartNumber)
		}
		buf.Write(data)
	}
	m.blobs[key] = buf.Bytes()
	delete(m.multiparts, uploadID)
	return nil
}

// AbortMultipart discards the session and its buffered parts.
func (m *Memory) AbortMultipart(_ context.Context, _, uploadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.multiparts[uploadID]; !ok {
		return ErrNotFound
	}
	delete(m.multiparts, uploadID)
	return nil
}

// ListMultiparts returns every in-flight session, oldest first.
func (m *Memory) ListMultiparts(context.Context) ([]MultipartUpload, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MultipartUpload, 0, len(m.multiparts))
	for uploadID, session := range m.multiparts {
		out = append(out, MultipartUpload{Key: session.key, UploadID: uploadID, Initiated: session.initiated})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Initiated.Before(out[j].Initiated) })
	return out, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

var _ MultipartStorer = (*Memory)(nil)
