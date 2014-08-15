package httpcache

import (
	"net/http"
	"sync"
)

type Store interface {
	Get(key string) (*http.Response, bool, error)
	Set(key string, r *http.Response) error
	Delete(key string) error
}

type MapStore struct {
	mutex     sync.RWMutex
	resources map[string]*http.Response
}

func NewMapStore() *MapStore {
	return &MapStore{resources: map[string]*http.Response{}}
}

func (m *MapStore) Get(k string) (*http.Response, bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	r, ok := m.resources[k]
	return r, ok, nil
}

func (m *MapStore) Set(k string, r *http.Response) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.resources[k] = r
	return nil
}

func (m *MapStore) Delete(k string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.resources, k)
	return nil
}
