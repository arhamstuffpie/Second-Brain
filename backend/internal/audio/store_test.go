package audio

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStoreObjectLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStore(root, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	object, err := store.Save(ctx, "clip.wav", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if object.Provider != "local" || object.Key == "" || object.SHA256 == "" {
		t.Fatalf("unexpected object: %#v", object)
	}
	if ok, err := store.Exists(ctx, object.Key); err != nil || !ok {
		t.Fatalf("exists = %v, %v", ok, err)
	}
	r, err := store.Open(ctx, object.Key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if string(got) != "hello" {
		t.Fatalf("read %q", got)
	}
	legacy, err := store.Open(ctx, filepath.Join(root, object.Key))
	if err != nil {
		t.Fatalf("open legacy absolute path: %v", err)
	}
	_ = legacy.Close()
	if _, err := store.Open(ctx, filepath.Join(t.TempDir(), object.Key)); err == nil {
		t.Fatal("opened path outside storage root")
	}
	if _, err := store.Checksum(ctx, object.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SignedDownloadURL(ctx, object.Key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, object.Key); err != nil {
		t.Fatal(err)
	}
}

func TestS3StoreIntegration(t *testing.T) {
	bucket := os.Getenv("APP_S3_INTEGRATION_BUCKET")
	if bucket == "" || os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		t.Skip("set APP_S3_INTEGRATION_BUCKET and AWS credentials to run")
	}
	store, err := NewS3Store(context.Background(), bucket, "ai-second-brain-test", os.Getenv("AWS_REGION"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	object, err := store.Save(ctx, "clip.wav", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Delete(ctx, object.Key)
	if ok, err := store.Exists(ctx, object.Key); err != nil || !ok {
		t.Fatalf("head = %v, %v", ok, err)
	}
	r, err := store.Open(ctx, object.Key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if string(got) != "hello" {
		t.Fatalf("read %q", got)
	}
	if checksum, err := store.Checksum(ctx, object.Key); err != nil || checksum == "" {
		t.Fatalf("checksum = %q, %v", checksum, err)
	}
	if err := store.Delete(ctx, object.Key); err != nil {
		t.Fatal(err)
	}
}
