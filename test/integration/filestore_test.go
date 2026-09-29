//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"strings"
	"testing"
	"time"

	testcontainersminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/your-org/go-template/internal/platform/filestore"
)

// TestS3FileStoreContract runs the same operations the in-memory adapter
// serves against a real MinIO container: put → open → byte equality →
// delete → not-found, plus a presigned URL fetch.
func TestS3FileStoreContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// The module's default image is the tag the testcontainers project
	// itself tests against; restricted environments that cannot pull it
	// skip instead of failing the suite.
	container, err := testcontainersminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z",
		testcontainersminio.WithUsername("filesapp"), testcontainersminio.WithPassword("files-secret"))
	if err != nil {
		msg := err.Error()
		for _, marker := range []string{"pull access denied", "manifest unknown", "No such image", "not found"} {
			if strings.Contains(msg, marker) {
				t.Skipf("minio image unavailable in this environment: %v", err)
			}
		}
		t.Fatalf("start minio: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	endpoint, err := container.Endpoint(ctx, "http")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	store, err := filestore.NewS3(ctx, filestore.S3Config{
		Endpoint: endpoint, Region: "us-east-1", Bucket: "test-bucket",
		AccessKeyID: "filesapp", SecretKey: "files-secret", UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("open s3: %v", err)
	}

	payload := make([]byte, 64<<10)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("payload: %v", err)
	}

	if err := store.Put(ctx, "contract/blob.bin", bytes.NewReader(payload), int64(len(payload)), "application/octet-stream"); err != nil {
		t.Fatalf("put: %v", err)
	}

	stream, err := store.Open(ctx, "contract/blob.bin")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("roundtrip mismatch")
	}

	url, err := store.Presign(ctx, "contract/blob.bin", time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	if url == "" {
		t.Fatal("presign must return a URL")
	}

	if err := store.Delete(ctx, "contract/blob.bin"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Open(ctx, "contract/blob.bin"); err != filestore.ErrNotFound {
		t.Fatalf("deleted blob: want ErrNotFound, got %v", err)
	}
}
