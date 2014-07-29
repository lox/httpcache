package httpcache

import (
	"errors"
	"fmt"
	"net/http"
)

type Cache struct {
	store   map[string]*Entity
	private bool
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

func (c *Cache) Store(key string, ent *Entity) error {
	c.store[key] = ent
	return nil
}

func (c *Cache) Has(key string) bool {
	_, exists := c.store[key]
	return exists
}

var ErrKeyMissing error = errors.New("key missing from cache")

func (c *Cache) Retrieve(key string) (*Entity, error) {
	ent, exists := c.store[key]
	if !exists {
		return nil, ErrKeyMissing
	}

	return ent, nil
}

func (c *Cache) IsStoreable(ent *Entity) (bool, string, error) {
	if ok, reason, err := c.IsCacheable(ent); !ok {
		return ok, reason, err
	}

	cc, err := ent.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.NoStore {
		return false, "Response contained no-store", err
	}

	if cc.Private && !c.private {
		return false, "Response is private", nil
	}

	return true, "", nil
}

func (c *Cache) IsCacheable(ent *Entity) (bool, string, error) {
	cc, err := ent.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.NoCache {
		return false, "Response contained no-cache", err
	}

	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.4
	allowed := []int{
		http.StatusOK,
		http.StatusFound,
		http.StatusNonAuthoritativeInfo,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusGone,
	}

	for _, a := range allowed {
		if a == ent.StatusCode {
			return true, "", nil
		}
	}

	msg := fmt.Sprintf("Response code %d isn't cacheable", ent.StatusCode)
	return false, msg, nil
}

func (c *Cache) IsRequestCacheable(r *http.Request) (bool, string, error) {
	cc, err := ParseCacheControl(r.Header.Get(CacheControlHeader))
	if err != nil {
		return false, err.Error(), err
	}

	if cc.NoCache {
		return false, "Request contains no-cache", nil
	}

	return true, "", nil
}
