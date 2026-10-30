package history

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// TestS3Store runs against an S3-compatible server named by OVERHEAD_TEST_S3 (s3://bucket/prefix, the bucket
// already made) and OVERHEAD_TEST_S3_ENDPOINT; with compose.yaml's s3 profile (docker compose --profile s3 up -d s3):
//
//	OVERHEAD_TEST_S3=s3://overhead-test/history OVERHEAD_TEST_S3_ENDPOINT=http://localhost:9010 \
//	AWS_ACCESS_KEY_ID=overhead AWS_SECRET_ACCESS_KEY=overhead-s3-local AWS_REGION=us-east-1 go test ./internal/history
func TestS3Store(t *testing.T) {
	target := os.Getenv("OVERHEAD_TEST_S3")
	if target == "" {
		t.Skip("OVERHEAD_TEST_S3 not set")
	}
	ctx := context.Background()
	store, err := NewS3Store(ctx, target, os.Getenv("OVERHEAD_TEST_S3_ENDPOINT"))
	if err != nil {
		t.Fatal(err)
	}
	bt := &Batch{Minute: t0, Run: "20261030T220000Z-abc123", FirstSeq: 7}
	_, line := ev(bt.Run, 7, t0)
	bt.Lines = [][]byte{line}
	body, _ := bt.Encode()
	// Twice: the second put overwrites the first, as a redelivered batch must.
	for range 2 {
		if err := store.Put(ctx, bt.Key(), body); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(store.Bucket), Prefix: aws.String(store.Prefix + "/positions/dt=2026-10-29/"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) != 1 || !strings.HasSuffix(*list.Contents[0].Key, "_000000000007.jsonl.gz") {
		t.Fatalf("objects = %+v", list.Contents)
	}
	got, err := store.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(store.Bucket), Key: list.Contents[0].Key})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	raw, _ := io.ReadAll(got.Body)
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("stored object is not gzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if !bytes.Contains(plain, []byte(`"seq":7`)) {
		t.Fatalf("stored %q", plain)
	}
}

func TestNewS3StoreRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "bucket/prefix", "https://bucket/prefix", "s3:///prefix"} {
		if _, err := NewS3Store(context.Background(), u, ""); err == nil {
			t.Errorf("NewS3Store(%q) should fail", u)
		}
	}
}
