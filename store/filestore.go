package store

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type FileStore struct {
	dir       string
	mutex     sync.RWMutex
	FileNamer func(k string) string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	return &FileStore{dir: dir, FileNamer: func(k string) string {
		h := md5.New()
		io.WriteString(h, k)
		return fmt.Sprintf("%x", h.Sum(nil))
	}}, nil
}

func (fs *FileStore) Has(key string) bool {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	if _, err := fs.file(key, os.O_RDONLY); err != nil {
		return false
	}
	return true
}

func (fs *FileStore) Copy(dest, src string) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	s, err := fs.file(src, os.O_RDONLY)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := fs.file(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}

	return d.Close()
}

func (fs *FileStore) Delete(key string) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	err := os.Remove(fs.filePath(key))
	if err != nil && os.IsNotExist(err) {
		return ErrNotExists
	}
	return err
}

func (fs *FileStore) ReadStream(key string) (io.ReadCloser, error) {
	fs.mutex.RLock()

	f, err := fs.file(key, os.O_RDONLY)
	if err != nil {
		fs.mutex.RUnlock()
		return nil, err
	}
	return &fileCloser{f, &fs.mutex}, nil
}

func (fs *FileStore) WriteStream(key string, r io.Reader) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	f, err := fs.file(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, r); err != nil {
		return err
	}

	return nil
}

func (fs *FileStore) filePath(key string) string {
	return filepath.Join(fs.dir, fs.FileNamer(key))
}

func (fs *FileStore) file(key string, flags int) (*os.File, error) {
	f, err := os.OpenFile(fs.filePath(key), flags, 0777)
	if err != nil && os.IsNotExist(err) {
		return f, ErrNotExists
	} else if err != nil {
		return f, err
	}

	return f, nil
}

type fileCloser struct {
	*os.File
	readMutex *sync.RWMutex
}

func (fc *fileCloser) Close() error {
	defer fc.readMutex.RUnlock()
	if err := fc.File.Close(); err != nil {
		return err
	}
	return nil
}
