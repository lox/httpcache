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
	mutex     *KeyMutex
	FileNamer func(k string) string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	return &FileStore{
		dir: dir, FileNamer: md5Namer, mutex: NewKeyMutex(),
	}, nil
}

func (fs *FileStore) Has(key string) bool {
	fs.mutex.KeyRLock(key)
	defer fs.mutex.KeyRUnlock(key)

	if _, err := fs.file(key, os.O_RDONLY); err != nil {
		return false
	}
	return true
}

func (fs *FileStore) Delete(key string) error {
	fs.mutex.KeyLock(key)
	defer fs.mutex.KeyUnlock(key)

	err := os.Remove(fs.filePath(key))
	if err != nil && os.IsNotExist(err) {
		return ErrNotExists
	}
	return err
}

func (fs *FileStore) Reader(key string) (io.ReadCloser, error) {
	fs.mutex.KeyRLock(key)

	f, err := fs.file(key, os.O_RDONLY)
	if err != nil {
		fs.mutex.KeyRUnlock(key)
		return nil, err
	}

	return fs.mutex.KeyRCloser(key, f), nil
}

func (fs *FileStore) Writer(key string) (io.WriteCloser, error) {
	fs.mutex.KeyLock(key)

	f, err := fs.file(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, err
	}

	return fs.mutex.KeyWCloser(key, f), nil
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
