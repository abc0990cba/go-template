package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/transport/http"
)

// S3Config selects the endpoint (AWS S3 when Region is set without a custom
// endpoint, MinIO/anything else with Endpoint) and credentials.
type S3Config struct {
	Endpoint     string // e.g. http://minio:9000; empty = real AWS
	Region       string
	Bucket       string
	AccessKeyID  string
	SecretKey    string
	UsePathStyle bool // MinIO wants path-style addressing
}

// S3 stores blobs in an S3-compatible bucket.
type S3 struct {
	client *s3.Client
	bucket string
}

// NewS3 opens the client and verifies the bucket is reachable (a HEAD
// against the bucket); a missing bucket is fatal for the adapter so the
// bootstrap can fall back to the in-process store with a warning.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" || cfg.Region == "" {
		return nil, fmt.Errorf("s3 filestore needs bucket and region")
	}
	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, "")),
	}
	if cfg.Endpoint != "" {
		opts = append(opts, awscfg.WithBaseEndpoint(cfg.Endpoint))
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("s3 config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})
	// Reachability probe: ListObjectsV2 with a page of one is the cheapest
	// call that proves bucket + credentials.
	if _, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(cfg.Bucket),
		MaxKeys: aws.Int32(1),
	}); err != nil {
		return nil, fmt.Errorf("s3 probe bucket %q: %w", cfg.Bucket, err)
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("s3 open %s: %w", key, err)
	}
	return out.Body, nil
}

// isS3NotFound folds the two shapes a missing object can take: the
// NoSuchKey API error and a bare 404 response.
func isS3NotFound(err error) bool {
	var noKey *s3types.NoSuchKey
	if errors.As(err, &noKey) {
		return true
	}
	var respErr *http.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func (s *S3) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	presign := s3.NewPresignClient(s.client)
	req, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String("attachment"),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("s3 presign %s: %w", key, err)
	}
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}
