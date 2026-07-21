package storage

import (
	"context"
	"errors"

	"github.com/hitel00000/mold/resource"
)

// Common storage errors.
var (
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
)

// Record represents a dynamic row of resource data.
type Record map[string]any

// Query specifies filtering and pagination options for List operations.
type Query struct {
	Filter map[string]any
	Limit  int
	Offset int
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
