package store

import (
	"bytes"

	"io"
	"io/ioutil"
	"sync"
)

type MapStore struct {
	*sync.RWMutex
	data map[string][]byte
}

func NewMapStore() *MapStore {
	return &MapStore{
		RWMutex: new(sync.RWMutex),
		data:    map[string][]byte{},
	}
}

func (m *MapStore) Has(key string) bool {
	m.RLock()
	defer m.RUnlock()

	_, ok := m.data[key]
	return ok
}

func (m *MapStore) Read(key string) (io.ReadCloser, error) {
	m.RLock()
	defer m.RUnlock()
	b, ok := m.data[key]
	if !ok {
		return nil, ErrNotExists
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

func (m *MapStore) WriteFrom(key string, r io.Reader) error {
	m.Lock()
	defer m.Unlock()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	m.data[key] = b
	return nil
}

func (m *MapStore) Delete(key string) error {
	m.Lock()
	defer m.Unlock()
	delete(m.data, key)
	return nil
}
