package storage

import (
	"bytes"
	"fmt"
	"net/http"
)

type MemoryStorage struct {
	items *CappedLRUList
}

// Create a new memory storage with a maximum size of capacity bytes, or 0 bytes for unbounded.
func NewMemoryStorage(capacity uint64) *MemoryStorage {
	return &MemoryStorage{
		items: NewCappedLRUList(capacity),
	}
}

func (ms *MemoryStorage) Len() int {
	return ms.items.Len()
}

func (ms *MemoryStorage) Keys() []string {
	return ms.items.Keys()
}

func (ms *MemoryStorage) Freshen(key string, statusCode int, header http.Header) error {
	s, exists := ms.items.Get(key)
	if !exists {
		return keyNotFoundError{fmt.Sprintf("Key %q doesn't exist, can't store meta", key), key}
	}

	res := s.(*byteStorable)
	res.header = header
	res.statusCode = statusCode
	return nil
}

func (ms *MemoryStorage) Store(key string, s Storable) error {
	if err := ms.Delete(key); err != nil && !IsErrNotFound(err) {
		return err
	}

	var buf = &bytes.Buffer{}

	_, err := StorableCopy(buf, s)
	if err != nil {
		return err
	}

	ms.items.Add(key, &byteStorable{
		buf.Bytes(), s.Header(), s.Status(),
	})

	return nil
}

func (ms *MemoryStorage) GetMeta(key string) (int, http.Header, error) {
	s, exists := ms.items.Get(key)
	if !exists {
		return 0, nil, keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	bs := s.(*byteStorable)
	return bs.statusCode, bs.header, nil
}

func (ms *MemoryStorage) Get(key string) (Storable, error) {
	s, exists := ms.items.Get(key)
	if !exists {
		return nil, keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	return s, nil
}

func (ms *MemoryStorage) Delete(key string) error {
	_, exists := ms.items.Get(key)
	if !exists {
		return keyNotFoundError{fmt.Sprintf("Key %q doesn't exist", key), key}
	}

	ms.items.Delete(key)
	return nil
}
