package sqlite

import (
	"context"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

func (s *Store) Create(ctx context.Context, res *resource.Resource, record storage.Record) (storage.Record, error) {
	panic("not implemented")
}

func (s *Store) Get(ctx context.Context, res *resource.Resource, id any) (storage.Record, error) {
	panic("not implemented")
}

func (s *Store) List(ctx context.Context, res *resource.Resource, query storage.Query) ([]storage.Record, error) {
	panic("not implemented")
}

func (s *Store) Update(ctx context.Context, res *resource.Resource, id any, record storage.Record) (storage.Record, error) {
	panic("not implemented")
}

func (s *Store) SoftDelete(ctx context.Context, res *resource.Resource, id any) error {
	panic("not implemented")
}
