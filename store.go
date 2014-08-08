package httpcache

import "sync"

type Store interface {
	Get(key string) (*Resource, bool)
	Set(key string, r *Resource)
}

type MapStore struct {
	mutex     sync.RWMutex
	resources map[string]*Resource
}

func NewMapStore() *MapStore {
	return &MapStore{resources: map[string]*Resource{}}
}

func (m *MapStore) Get(k string) (*Resource, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	r, ok := m.resources[k]
	return r, ok
}

func (m *MapStore) Set(k string, r *Resource) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.resources[k] = r
}
