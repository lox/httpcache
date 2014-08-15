package httpcache

import "net/http"

type Cache struct {
	store  Store
	shared bool
}

func NewPrivateCache() *Cache {
	return &Cache{
		store:  NewMapStore(),
		shared: false,
	}
}

func NewPublicCache() *Cache {
	return &Cache{
		store:  NewMapStore(),
		shared: true,
	}
}

func (c *Cache) Lookup(key string) (*http.Response, bool) {
	return nil, false
}

func (c *Cache) Store(key string, resp *http.Response) error {
	return nil
}
