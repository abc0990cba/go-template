//go:build integration

package integration

// The S3 multipart contract against a real MinIO container: open a
// session, presign a part, complete without the part (must fail), upload
// through the SDK directly (the presigned-PUT equivalent), complete again,
// and read the assembled object back.

import (
	"bytes"
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	testcontainersminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/your-org/go-template/internal/platform/filestore"
)

func TestS3MultipartContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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
	// The optional capabilities are asserted through the port, the way the
	// files feature type-asserts its blob store.
	var blobStore filestore.Store = store
	storer, ok := blobStore.(filestore.MultipartStorer)
	if !ok {
		t.Fatal("s3 adapter must implement MultipartStorer")
	}

	// Open the session; MinIO tracks it.
	uploadID, err := storer.CreateMultipart(ctx, "contract/multi.bin", "application/octet-stream")
	if err != nil {
		t.Fatalf("create multipart: %v", err)
	}
	sessions, err := storer.ListMultiparts(ctx)
	if err != nil || len(sessions) != 1 || sessions[0].UploadID != uploadID {
		t.Fatalf("list: %d sessions, err %v", len(sessions), err)
	}

	// Completing before any part exists must fail (the server's ETag check).
	err = storer.CompleteMultipart(ctx, "contract/multi.bin", uploadID,
		[]filestore.CompletedPart{{PartNumber: 1, ETag: "bogus"}})
	if err == nil {
		t.Fatal("complete without an uploaded part must fail")
	}

	// Upload two parts the way a client would through the presigned URL:
	// a raw UploadPart against the session.
	part1 := make([]byte, 5<<20)
	part2 := []byte("tail")
	if _, err := rand.Read(part1); err != nil {
		t.Fatalf("payload: %v", err)
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("filesapp", "files-secret", "")),
		awscfg.WithBaseEndpoint(endpoint),
	)
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	s3client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	etags := make([]filestore.CompletedPart, 0, 2)
	for i, payload := range [][]byte{part1, part2} {
		out, err := s3client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String("test-bucket"),
			Key:        aws.String("contract/multi.bin"),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(int32(i + 1)),
			Body:       bytes.NewReader(payload),
		})
		if err != nil {
			t.Fatalf("upload part %d: %v", i+1, err)
		}
		etags = append(etags, filestore.CompletedPart{PartNumber: int32(i + 1), ETag: *out.ETag})
	}

	// Presign a part URL (the capability the initiate endpoint serves).
	partURL, err := storer.PresignPart(ctx, "contract/multi.bin", uploadID, 3, time.Minute)
	if err != nil || !strings.Contains(partURL, "uploadId="+uploadID) && !strings.Contains(partURL, "uploadId%3D") {
		t.Fatalf("presign part: url=%q err=%v", partURL, err)
	}

	// Complete assembles the object.
	if err := storer.CompleteMultipart(ctx, "contract/multi.bin", uploadID, etags); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Size comes from storage, and the bytes are the ordered concatenation.
	sizer, ok := blobStore.(filestore.Sizer)
	if !ok {
		t.Fatal("s3 adapter must implement Sizer")
	}
	size, err := sizer.Size(ctx, "contract/multi.bin")
	if err != nil || size != int64(len(part1)+len(part2)) {
		t.Fatalf("size: %d err %v", size, err)
	}
	stream, err := store.Open(ctx, "contract/multi.bin")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	head := make([]byte, 16)
	if _, err := stream.Read(head); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = stream.Close()
	if !bytes.Equal(head, part1[:16]) {
		t.Fatal("assembled object does not start with part 1")
	}

	// The abort path: sessions vanish from the listing.
	abortID, err := storer.CreateMultipart(ctx, "contract/abort.bin", "text/plain")
	if err != nil {
		t.Fatalf("create abort session: %v", err)
	}
	if err := storer.AbortMultipart(ctx, "contract/abort.bin", abortID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	sessions, err = storer.ListMultiparts(ctx)
	if err != nil {
		t.Fatalf("list after abort: %v", err)
	}
	for _, session := range sessions {
		if session.UploadID == abortID {
			t.Fatal("aborted session still listed")
		}
	}
}
