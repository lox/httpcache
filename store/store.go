package store

import (
	"errors"
	"io"
)

var ErrNotExists = errors.New("key does not exist in store")

type Store interface {
	Has(key string) bool
	Delete(key string) error
	WriteFrom(key string, r io.Reader) error
	Read(key string) (io.ReadCloser, error)
}

func IsNotExists(e error) bool {
	return e == ErrNotExists
}

func Copy(destKey, srcKey string, s Store) error {
	r, err := s.Read(srcKey)
	if err != nil {
		return err
	}
	err = s.WriteFrom(destKey, r)
	if err != nil {
		return err
	}
	if err = r.Close(); err != nil {
		return err
	}
	return nil
}
