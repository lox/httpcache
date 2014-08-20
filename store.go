package httpcache

import (
	"bytes"
	"io/ioutil"
	"sync"
)

type Store interface {
	Has(key string) bool
	Set(key string, res *Resource) error
	Get(key string) (*Resource, bool, error)
	Delete(key string) error
}

type MapStore struct {
	mutex     sync.RWMutex
	resources map[string]*Resource
}

func NewMapStore() *MapStore {
	return &MapStore{resources: map[string]*Resource{}}
}

func (m *MapStore) Has(key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, ok := m.resources[key]
	return ok
}

func (m *MapStore) Get(key string) (*Resource, bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	r, ok := m.resources[key]
	return r, ok, nil
}

func (m *MapStore) Set(key string, res *Resource) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	res.Body.Close()
	res.Body = ioutil.NopCloser(bytes.NewReader(b))
	m.resources[key] = res
	return nil
}

func (m *MapStore) Delete(key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.resources, key)
	return nil
}
