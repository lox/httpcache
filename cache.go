package httpcache

import "errors"

var ErrKeyMissing error = errors.New("key missing from cache")

type Cache struct {
	store   map[string]*Entity
	private bool
}

func (c *Cache) Store(key string, ent *Entity) error {
	c.store[key] = ent
	return nil
}

func (c *Cache) Has(key string) bool {
	_, exists := c.store[key]
	return exists
}

func (c *Cache) Retrieve(key string) (*Entity, error) {
	ent, exists := c.store[key]
	if !exists {
		return nil, ErrKeyMissing
	}

	return ent, nil
}

func (c *Cache) IsStoreable(ent *Entity) (bool, error) {
	if ok, err := ent.IsCacheable(); !ok {
		return ok, err
	}

	cc, err := ent.CacheControl()
	if err != nil {
		return false, err
	}

	if cc.NoStore || cc.NoCache {
		return false, err
	}

	if cc.Private && c.private {
		return false, nil
	}

	return true, nil
}

func NewPrivateCache() *Cache {
	return &Cache{
		store:   map[string]*Entity{},
		private: true,
	}
}

func NewPublicCache() *Cache {
	return &Cache{
		store:   map[string]*Entity{},
		private: false,
	}
}
