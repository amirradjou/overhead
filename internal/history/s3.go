package history

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store keeps batches in an S3 bucket under a prefix, where Snowpipe picks them up (README: History). The keys
// are the same as DirStore's, so a redelivered batch overwrites its own object.
type S3Store struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

// NewS3Store connects to the bucket in an s3://bucket/prefix URL with the standard AWS settings (AWS_REGION,
// AWS_PROFILE or AWS_ACCESS_KEY_ID ...). endpoint, when set, points at an S3-compatible server such as MinIO.
func NewS3Store(ctx context.Context, rawURL, endpoint string) (*S3Store, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return nil, fmt.Errorf("want s3://bucket/prefix, got %q", rawURL)
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // MinIO and most S3 look-alikes want path-style addressing
		}
	})
	return &S3Store{Client: client, Bucket: u.Host, Prefix: strings.Trim(u.Path, "/")}, nil
}

// Put uploads body to key. A batch is acknowledged only after this returns, so a failed upload is retried.
func (s *S3Store) Put(ctx context.Context, key string, body []byte) error {
	if strings.Contains(key, "..") {
		return fmt.Errorf("refusing key %q", key)
	}
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(s.Bucket),
		Key:             aws.String(path.Join(s.Prefix, key)),
		Body:            bytes.NewReader(body),
		ContentType:     aws.String("application/x-ndjson"),
		ContentEncoding: aws.String("gzip"),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}
