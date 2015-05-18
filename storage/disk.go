package storage

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type fileStorable struct {
	path       string
	size       uint64
	header     http.Header
	statusCode int
}

func (fs *fileStorable) Status() int {
	return fs.statusCode
}

func (fs *fileStorable) Size() uint64 {
	return fs.size
}

func (fs *fileStorable) Header() http.Header {
	return fs.header
}

func (fs *fileStorable) Reader() (ReadSeekCloser, error) {
	return os.Open(fs.path)
}

type DiskStorage struct {
	sync.Mutex
	dir   string
	items *CappedLRUList
}

// Create a new disk storage with a maximum size of capacity bytes.
func NewDiskStorage(dir string, perms os.FileMode, capacity uint64) (*DiskStorage, error) {
	if err := os.MkdirAll(dir, perms); err != nil {
		return nil, err
	}
	return &DiskStorage{
		dir:   dir,
		items: NewCappedLRUList(capacity),
	}, nil
}

func (ms *DiskStorage) Len() int {
	return ms.items.Len()
}

func (ms *DiskStorage) Keys() []string {
	return ms.items.Keys()
}

func (ms *DiskStorage) Freshen(key string, statusCode int, header http.Header) error {
	ms.Lock()
	defer ms.Unlock()

	r, exists := ms.items.Get(key)
	if !exists {
		return keyNotFoundError{fmt.Sprintf("Key %q doesn't exist, can't store meta", key), key}
	}

	fs := r.(*fileStorable)
	fs.header = header
	fs.statusCode = statusCode
	return nil
}

func (ms *DiskStorage) Store(key string, s Storable) error {
	if err := ms.Delete(key); err != nil && !IsErrNotFound(err) {
		return err
	}

	path := ms.keyPath(key)
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	n, err := StorableCopy(f, s)
	if err != nil {
		return err
	}

	ms.items.Add(key, &fileStorable{
		path, uint64(n), s.Header(), s.Status(),
	})

	return nil
}

func (ms *DiskStorage) GetMeta(key string) (int, http.Header, error) {
	r, exists := ms.items.Get(key)
	if !exists {
		return 0, nil, keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	fs := r.(*fileStorable)
	return fs.statusCode, fs.Header(), nil
}

func (ms *DiskStorage) Get(key string) (Storable, error) {
	s, exists := ms.items.Get(key)
	if !exists {
		return nil, keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	return s, nil
}

func (ms *DiskStorage) Delete(key string) error {
	_, exists := ms.items.Get(key)
	if !exists {
		return keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	ms.items.Delete(key)
	return nil
}

func (ms *DiskStorage) keyPath(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return filepath.Join(ms.dir, fmt.Sprintf("%x", h.Sum(nil)))
}
