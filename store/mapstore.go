package store

import (
	"bytes"
	"io"
	"io/ioutil"
)

type MapStore struct {
	mutex *storeMutex
	data  map[string][]byte
}

func NewMapStore() *MapStore {
	return &MapStore{
		data: map[string][]byte{}, mutex: newStoreMutex(),
	}
}

func (m *MapStore) Has(key string) bool {
	mutex := m.mutex.ForKey(key)
	mutex.RLock()
	defer mutex.RUnlock()
	_, ok := m.data[key]
	return ok
}

func (m *MapStore) Reader(key string) (io.ReadCloser, error) {
	mutex := m.mutex.ForKey(key)
	mutex.RLock()
	defer mutex.RUnlock()
	b, ok := m.data[key]
	if !ok {
		return nil, ErrNotExists
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

func (m *MapStore) Writer(key string) (io.WriteCloser, error) {
	return &mapStoreWriter{Buffer: &bytes.Buffer{}, ms: m, key: key}, nil
}

func (m *MapStore) Delete(key string) error {
	mutex := m.mutex.ForKey(key)
	mutex.Lock()
	defer mutex.Unlock()
	delete(m.data, key)
	return nil
}

type mapStoreWriter struct {
	*bytes.Buffer
	ms  *MapStore
	key string
}

func (mw *mapStoreWriter) Close() error {
	mutex := mw.ms.mutex.ForKey(mw.key)
	mutex.Lock()
	defer mutex.Unlock()

	mw.ms.data[mw.key] = mw.Buffer.Bytes()
	mw.Buffer = nil
	return nil
}
