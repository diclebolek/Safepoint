// Package storage defines object-storage abstractions used by the operator.
package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo describes a stored backup object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Credentials holds S3-compatible access credentials.
type Credentials struct {
	AccessKey string
	SecretKey string
}

// Config configures an S3-compatible client.
type Config struct {
	Endpoint  string
	Bucket    string
	Prefix    string
	Region    string
	UseSSL    bool
	Creds     Credentials
}

// ObjectStore uploads, lists, and deletes backup objects.
// Implementations must be safe for concurrent use.
type ObjectStore interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64) error
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}
