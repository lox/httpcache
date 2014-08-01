package httpcache

type Store interface {
	Get(key string) (*Resource, bool)
	Set(key string, r *Resource) error
}

type MapStore struct {
	resources map[string]*Resource
}

func NewMapStore() *MapStore {
	return &MapStore{map[string]*Resource{}}
}

func (m *MapStore) Get(k string) (*Resource, bool) {
	r, ok := m.resources[k]
	return r, ok
}

func (m *MapStore) Set(k string, r *Resource) error {
	m.resources[k] = r
	return nil
}
