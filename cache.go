package httpcache

import (
	"errors"
	"os"

	"log"
	"net/http"

	"time"

	"github.com/lox/httpcache/storage"
)

// Returned when a resource doesn't exist
var ErrNotFoundInCache = errors.New("Not found in cache")

// Cache provides a storage mechanism for cached Resources
type Cache struct {
	storage storage.Storage
	stale   map[string]time.Time
}

type Header struct {
	http.Header
	StatusCode int
}

// NewCache returns a Cache backed off the provided Storage
func NewCache(s storage.Storage) *Cache {
	return &Cache{storage: s, stale: map[string]time.Time{}}
}

// NewMemoryCache returns an ephemeral cache in memory
func NewMemoryCache(capacity uint64) *Cache {
	return NewCache(storage.NewMemoryStorage(capacity))
}

// NewDiskCache returns a disk-backed cache
func NewDiskCache(dir string, perms os.FileMode, capacity uint64) (*Cache, error) {
	store, err := storage.NewDiskStorage(dir, perms, capacity)
	if err != nil {
		return nil, err
	}
	return NewCache(store), nil
}

// Store a resource against a number of keys
func (c *Cache) Store(res *Resource, keys ...string) error {
	for _, key := range keys {
		if err := c.storage.Store(key, res); err != nil {
			return err
		}
	}

	return nil
}

// Retrieve returns a cached Resource for the given key
func (c *Cache) Retrieve(key string) (*Resource, error) {
	storable, err := c.storage.Get(key)
	if err != nil && storage.IsErrNotFound(err) {
		return nil, ErrNotFoundInCache
	}

	res, err := NewResource(storable)
	if err != nil {
		return nil, err
	}

	if staleTime, exists := c.stale[key]; exists {
		if !res.DateAfter(staleTime) {
			log.Printf("stale marker of %s found", staleTime)
			res.Stale = true
		}
	}
	return res, nil
}

func (c *Cache) Invalidate(keys ...string) {
	log.Printf("invalidating %q", keys)
	for _, key := range keys {
		c.stale[key] = Clock()
	}
}

func (c *Cache) Freshen(res *Resource, keys ...string) error {
	for _, key := range keys {
		status, h, err := c.storage.GetMeta(key)
		if err != nil {
			if storage.IsErrNotFound(err) {
				continue
			}
			return err
		}
		log.Printf("todo: implement freshen: %#v %#v", status, h)

		// if h, err := c.Header(key); err == nil {
		// 	if h.StatusCode == res.Status() && headersEqual(h.Header, res.Header()) {
		// 		debugf("freshening key %s", key)
		// 		if err := c.storeHeader(h.StatusCode, res.Header(), key); err != nil {
		// 			return err
		// 		}
		// 	} else {
		// 		debugf("freshen failed, invalidating %s", key)
		// 		c.Invalidate(key)
		// 	}
		// }
	}
	return nil
}
