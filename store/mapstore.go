package store

import (
	"bytes"
	"io"
	"io/ioutil"
)

type MapStore struct {
	mutex *KeyMutex
	data  map[string][]byte
}

func NewMapStore() *MapStore {
	return &MapStore{data: map[string][]byte{}, mutex: NewKeyMutex()}
}

func (m *MapStore) Has(key string) bool {
	m.mutex.KeyRLock(key)
	defer m.mutex.KeyRUnlock(key)
	_, ok := m.data[key]
	return ok
}

func (m *MapStore) Reader(key string) (io.ReadCloser, error) {
	m.mutex.KeyRLock(key)
	defer m.mutex.KeyRUnlock(key)
	b, ok := m.data[key]
	if !ok {
		return nil, ErrNotExists
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

func (m *MapStore) Writer(key string) (io.WriteCloser, error) {
	m.mutex.KeyLock(key)
	return &mapStoreWriter{Buffer: &bytes.Buffer{}, ms: m, key: key}, nil
}

func (m *MapStore) Delete(key string) error {
	m.mutex.KeyLock(key)
	defer m.mutex.KeyUnlock(key)
	delete(m.data, key)
	return nil
}

type mapStoreWriter struct {
	*bytes.Buffer
	ms  *MapStore
	key string
}

func (mw *mapStoreWriter) Close() error {
	defer mw.ms.mutex.KeyUnlock(mw.key)
	mw.ms.data[mw.key] = mw.Buffer.Bytes()
	mw.Buffer = nil
	return nil
}
