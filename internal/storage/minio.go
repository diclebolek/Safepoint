package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStore is an S3-compatible ObjectStore backed by MinIO (or any S3 API).
type MinIOStore struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewMinIOStore builds a MinIO/S3 client from Config.
func NewMinIOStore(cfg Config) (*MinIOStore, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("storage endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage bucket is required")
	}
	if cfg.Creds.AccessKey == "" || cfg.Creds.SecretKey == "" {
		return nil, fmt.Errorf("storage credentials are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.Creds.AccessKey, cfg.Creds.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &MinIOStore{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

// Upload streams body to the given object key under the configured prefix.
func (s *MinIOStore) Upload(ctx context.Context, key string, body io.Reader, size int64) error {
	objectKey := s.objectKey(key)
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, body, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("upload %q: %w", objectKey, err)
	}
	return nil
}

// List returns objects under prefix (relative to the store prefix).
func (s *MinIOStore) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := s.objectKey(prefix)
	opts := minio.ListObjectsOptions{
		Prefix:    fullPrefix,
		Recursive: true,
	}

	var out []ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, opts) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects: %w", obj.Err)
		}
		out = append(out, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	return out, nil
}

// Delete removes an object by full or relative key.
func (s *MinIOStore) Delete(ctx context.Context, key string) error {
	objectKey := key
	if !strings.HasPrefix(key, s.prefix) && s.prefix != "" {
		objectKey = s.objectKey(key)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %q: %w", objectKey, err)
	}
	return nil
}

func (s *MinIOStore) objectKey(key string) string {
	key = strings.TrimPrefix(key, "/")
	if s.prefix == "" {
		return key
	}
	return path.Join(s.prefix, key)
}
