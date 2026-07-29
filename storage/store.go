package storage

import (
	"context"
	"errors"
	"io"

	"github.com/hitel00000/mold/resource"
)

// Common storage errors.
var (
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
)

// Record represents a dynamic row of resource data.
type Record map[string]any

// OwnerFilterMode specifies how owner-based record filtering should be applied in Query.
type OwnerFilterMode int

const (
	OwnerFilterNone OwnerFilterMode = iota
	OwnerFilterUserAndNull
	OwnerFilterNullOnly
)

// OwnerFilter specifies owner-level row filtering options.
type OwnerFilter struct {
	Mode           OwnerFilterMode
	OwnershipField string
	UserID         any
}

// Query specifies filtering and pagination options for List operations.
type Query struct {
	IDs         []any
	Filter      map[string]any
	Limit       int
	Offset      int
	OwnerFilter *OwnerFilter
}

// Store defines the storage engine interface for Mold resources.
// It remains completely agnostic of the underlying database implementation.
type Store interface {
	EnsureSchema(ctx context.Context, res *resource.Resource) error
	Create(ctx context.Context, res *resource.Resource, record Record) (Record, error)
	Get(ctx context.Context, res *resource.Resource, id any) (Record, error)
	List(ctx context.Context, res *resource.Resource, query Query) ([]Record, error)
	Update(ctx context.Context, res *resource.Resource, id any, record Record) (Record, error)
	SoftDelete(ctx context.Context, res *resource.Resource, id any) error
}

// BlobStore defines the binary data storage interface for Mold resources (e.g. image/file upload).
// Store (relational record CRUD) and BlobStore (binary byte stream storage) are kept as strictly separated responsibilities.
type BlobStore interface {
	Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}
