package store

import (
	"errors"
	"io"
)

var ErrNotExists = errors.New("key does not exist in store")

type Store interface {
	Has(key string) bool
	Delete(key string) error
	Writer(key string) (io.WriteCloser, error)
	Reader(key string) (io.ReadCloser, error)
}

func IsNotExists(e error) bool {
	return e == ErrNotExists
}

func Copy(destKey, srcKey string, s Store) (int64, error) {
	r, err := s.Reader(srcKey)
	if err != nil {
		return 0, err
	}
	w, err := s.Writer(destKey)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, r)
	if err != nil {
		return n, err
	}
	if err = r.Close(); err != nil {
		return n, err
	}
	if err = w.Close(); err != nil {
		return n, err
	}
	return n, nil
}
