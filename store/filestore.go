package store

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

type FileStore struct {
	sync.RWMutex
	dir string
}

func NewFileStore(dir string, maxSize uint64) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir}, nil
}

func (fs *FileStore) Has(key string) bool {
	fs.RLock()
	defer fs.RUnlock()

	if _, err := os.Stat(fs.path(key)); err == nil {
		return true
	}

	return false
}

func (fs *FileStore) Delete(key string) error {
	fs.Lock()
	defer fs.Unlock()

	os.Remove(fs.path(key))
	return nil
}

func (fs *FileStore) Read(key string) (io.ReadCloser, error) {
	fs.RLock()
	defer fs.RUnlock()

	f, err := os.Open(fs.path(key))
	if os.IsNotExist(err) {
		return nil, ErrNotExists
	} else if err != nil {
		return nil, err
	}

	return f, nil
}

func (fs *FileStore) WriteFrom(key string, r io.Reader) error {
	fs.RLock()
	defer fs.RUnlock()

	tempFile, err := ioutil.TempFile(fs.dir, "tmp")
	if err != nil {
		return err
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, r)
	if err != nil {
		return err
	}

	if _, err := os.Stat(fs.path(key)); err == nil {
		os.Remove(fs.path(key))
	}

	if err = os.Link(tempFile.Name(), fs.path(key)); err != nil {
		return err
	}

	return nil
}

func (fs *FileStore) path(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return filepath.Join(fs.dir, fmt.Sprintf("%x", h.Sum(nil)))
}
