package audio

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Store struct {
	client         *s3.Client
	presign        *s3.PresignClient
	bucket, prefix string
	maxBytes       int64
}

func NewS3Store(ctx context.Context, bucket, prefix, region string, maxBytes int64) (*S3Store, error) {
	if strings.TrimSpace(bucket) == "" || maxBytes < 1 {
		return nil, fmt.Errorf("valid S3 storage configuration is required")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	client := s3.NewFromConfig(cfg)
	return &S3Store{client: client, presign: s3.NewPresignClient(client), bucket: bucket, prefix: strings.Trim(prefix, "/"), maxBytes: maxBytes}, nil
}

func (s *S3Store) key(filename string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := hex.EncodeToString(b) + path.Ext(path.Base(filename))
	if s.prefix != "" {
		return s.prefix + "/" + name, nil
	}
	return name, nil
}
func (s *S3Store) Save(ctx context.Context, filename string, content io.Reader) (service.StoredObject, error) {
	if content == nil {
		return service.StoredObject{}, fmt.Errorf("media content is required")
	}
	key, err := s.key(filename)
	if err != nil {
		return service.StoredObject{}, err
	}
	limited := io.LimitReader(content, s.maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return service.StoredObject{}, err
	}
	if len(data) == 0 || int64(len(data)) > s.maxBytes {
		return service.StoredObject{}, fmt.Errorf("media file exceeds %d bytes or is empty", s.maxBytes)
	}
	h := sha256.Sum256(data)
	checksum := hex.EncodeToString(h[:])
	if _, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(data), ChecksumSHA256: awsString(base64SHA256(h[:]))}); err != nil {
		return service.StoredObject{}, fmt.Errorf("put S3 object: %w", err)
	}
	return service.StoredObject{Provider: "s3", Bucket: s.bucket, Key: key, Path: key, SizeBytes: int64(len(data)), SHA256: checksum}, nil
}
func base64SHA256(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}
func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return false, err
	}
	return true, nil
}
func (s *S3Store) Checksum(ctx context.Context, key string) (string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key, ChecksumMode: types.ChecksumModeEnabled})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(awsStringValue(out.ChecksumSHA256)), nil
}
func (s *S3Store) SignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	out, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
func (s *S3Store) RestoreStatus(ctx context.Context, key string) (string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return "", err
	}
	if out.Restore != nil {
		return *out.Restore, nil
	}
	return "available", nil
}
func awsString(v string) *string { return &v }
func awsStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

var _ service.ObjectStorage = (*S3Store)(nil)
