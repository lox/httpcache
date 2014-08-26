package store

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type FileStore struct {
	dir       string
	mutex     *storeMutex
	FileNamer func(k string) string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	return &FileStore{
		dir: dir, FileNamer: md5Namer, mutex: newStoreMutex(),
	}, nil
}

func (fs *FileStore) Has(key string) bool {
	mutex := fs.mutex.ForKey(key)
	mutex.RLock()
	defer mutex.RUnlock()

	if _, err := fs.file(key, os.O_RDONLY); err != nil {
		return false
	}
	return true
}

func (fs *FileStore) Delete(key string) error {
	mutex := fs.mutex.ForKey(key)
	mutex.Lock()
	defer mutex.Unlock()

	err := os.Remove(fs.filePath(key))
	if err != nil && os.IsNotExist(err) {
		return ErrNotExists
	}
	return err
}

func (fs *FileStore) Reader(key string) (io.ReadCloser, error) {
	mutex := fs.mutex.ForKey(key)
	mutex.RLock()

	f, err := fs.file(key, os.O_RDONLY)
	if err != nil {
		mutex.RUnlock()
		return nil, err
	}

	return mutex.ReadCloser(f), nil
}

func (fs *FileStore) Writer(key string) (io.WriteCloser, error) {
	mutex := fs.mutex.ForKey(key)
	mutex.Lock()

	f, err := fs.file(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, err
	}

	return mutex.WriteCloser(f), nil
}

func (fs *FileStore) filePath(key string) string {
	return filepath.Join(fs.dir, fs.FileNamer(key))
}

func (fs *FileStore) file(key string, flags int) (*os.File, error) {
	f, err := os.OpenFile(fs.filePath(key), flags, 0777)

	if os.IsNotExist(err) {
		return nil, ErrNotExists
	}

	if err != nil {
		return nil, err
	}

	return f, nil
}

func md5Namer(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return fmt.Sprintf("%x", h.Sum(nil))
}
