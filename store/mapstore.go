package store

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync"
)

type MapStore struct {
	mutex sync.RWMutex
	data  map[string][]byte
}

func NewMapStore() *MapStore {
	return &MapStore{data: map[string][]byte{}}
}

func (m *MapStore) Has(key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, ok := m.data[key]
	return ok
}

func (m *MapStore) Copy(dest, src string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	b, ok := m.data[src]
	if !ok {
		return ErrNotExists
	}
	m.data[dest] = b
	return nil
}

func (m *MapStore) ReadStream(key string) (io.ReadCloser, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	b, ok := m.data[key]
	if !ok {
		return nil, ErrNotExists
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

func (m *MapStore) WriteStream(key string, r io.Reader) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	m.data[key] = b
	return nil
}

func (m *MapStore) Delete(key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.data, key)
	return nil
}
