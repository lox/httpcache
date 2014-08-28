package store

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"

	"github.com/peterbourgon/diskv"
)

type FileStore struct {
	d *diskv.Diskv
}

func NewFileStore(dir string, maxSize uint64) (*FileStore, error) {
	flatTransform := func(s string) []string {
		return []string{}
	}

	d := diskv.New(diskv.Options{
		BasePath:     dir,
		Transform:    flatTransform,
		CacheSizeMax: maxSize,
	})

	return &FileStore{d}, nil
}

func (fs *FileStore) Has(key string) bool {
	return fs.d.Has(hashKey(key))
}

func (fs *FileStore) Delete(key string) error {
	return fs.d.Erase(hashKey(key))
}

func (fs *FileStore) Read(key string) (io.ReadCloser, error) {
	r, err := fs.d.ReadStream(hashKey(key), false)

	if os.IsNotExist(err) {
		return nil, ErrNotExists
	}

	return r, err
}

func (fs *FileStore) WriteFrom(key string, r io.Reader) error {
	return fs.d.WriteStream(hashKey(key), r, false)
}

func hashKey(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return fmt.Sprintf("%x", h.Sum(nil))
}
