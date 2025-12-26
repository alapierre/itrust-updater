package backend

import (
	"context"
	"io"
)

type Backend interface {
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	Put(ctx context.Context, path string, openBody func() (io.ReadCloser, error), contentType string) error
	Exists(ctx context.Context, path string) (bool, error)
}
