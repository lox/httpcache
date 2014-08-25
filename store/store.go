package store

import (
	"errors"
	"io"
)

var ErrNotExists = errors.New("key does not exist in store")

type Store interface {
	Has(key string) bool
	Delete(key string) error
	Copy(dest, src string) error
	WriteStream(key string, r io.Reader) error
	ReadStream(key string) (io.ReadCloser, error)
}

func IsNotExists(e error) bool {
	return e == ErrNotExists
}
