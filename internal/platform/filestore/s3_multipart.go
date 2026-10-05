package filestore

// Multipart sessions on S3: the client uploads parts straight to presigned
// part URLs, so large files never stream through the API process.

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// CreateMultipart opens a session for the destination key.
func (s *S3) CreateMultipart(ctx context.Context, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3 create multipart %s: %w", key, err)
	}
	return *out.UploadId, nil
}

// PresignPart returns a PUT URL for one part; the client uploads the raw
// bytes there and reports the ETag from the response headers back.
func (s *S3) PresignPart(ctx context.Context, key, uploadID string, partNumber int32, ttl time.Duration) (string, error) {
	presign := s3.NewPresignClient(s.client)
	req, err := presign.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign part %d of %s: %w", partNumber, key, err)
	}
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// CompleteMultipart assembles the listed parts; the ETags are the server's
// integrity proof that exactly the uploaded bytes land in the object.
func (s *S3) CompleteMultipart(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	if len(parts) == 0 {
		return fmt.Errorf("s3 complete multipart %s: no parts listed", key)
	}
	ordered := make([]s3types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		ordered = append(ordered, s3types.CompletedPart{
			PartNumber: aws.Int32(part.PartNumber),
			ETag:       aws.String(part.ETag),
		})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return *ordered[i].PartNumber < *ordered[j].PartNumber
	})
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: ordered,
		},
	})
	if err != nil {
		return fmt.Errorf("s3 complete multipart %s: %w", key, err)
	}
	return nil
}

// AbortMultipart discards the session; S3 bills stored parts until an
// upload is completed or aborted, which is why the stale sweep exists.
func (s *S3) AbortMultipart(ctx context.Context, key, uploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return fmt.Errorf("s3 abort multipart %s/%s: %w", key, uploadID, err)
	}
	return nil
}

// ListMultiparts returns every in-flight session.
func (s *S3) ListMultiparts(ctx context.Context) ([]MultipartUpload, error) {
	var out []MultipartUpload
	var keyMarker, uploadIDMarker *string
	for {
		page, err := s.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
			Bucket:         aws.String(s.bucket),
			KeyMarker:      keyMarker,
			UploadIdMarker: uploadIDMarker,
		})
		if err != nil {
			return nil, fmt.Errorf("s3 list multiparts: %w", err)
		}
		for _, upload := range page.Uploads {
			out = append(out, MultipartUpload{
				Key:       *upload.Key,
				UploadID:  *upload.UploadId,
				Initiated: aws.ToTime(upload.Initiated),
			})
		}
		if !aws.ToBool(page.IsTruncated) {
			return out, nil
		}
		keyMarker, uploadIDMarker = page.NextKeyMarker, page.NextUploadIdMarker
	}
}

var _ MultipartStorer = (*S3)(nil)
