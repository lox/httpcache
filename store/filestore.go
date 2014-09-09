package store

import (
	"crypto/md5"
	"fmt"
	"io"

	"github.com/peterbourgon/diskv"
)

type FileStore struct {
	d *diskv.Diskv
}

func NewFileStore(dir string) *FileStore {
	flatTransform := func(s string) []string { return []string{} }

	return &FileStore{diskv.New(diskv.Options{
		BasePath:  dir,
		Transform: flatTransform,
	})}
}

func (fs *FileStore) Has(key string) bool {
	return fs.d.Has(fs.keyHash(key))
}

func (fs *FileStore) Delete(key string) error {
	return fs.d.Erase(fs.keyHash(key))
}

func (fs *FileStore) Read(key string) (io.ReadCloser, error) {
	return fs.d.ReadStream(fs.keyHash(key), false)
}

func (fs *FileStore) WriteFrom(key string, r io.Reader) error {
	return fs.d.WriteStream(fs.keyHash(key), r, false)
}

func (fs *FileStore) keyHash(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return fmt.Sprintf("%x", h.Sum(nil))
}
