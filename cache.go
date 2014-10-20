package httpcache

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	vfs "gopkgs.com/vfs.v1"
)

// Returned when a resource doesn't exist
var ErrNotFoundInCache = errors.New("Not found in cache")

// Cache provides a storage mechanism for cached Resources
type Cache struct {
	fs      vfs.VFS
	headers map[string]http.Header
}

// Store a resource against a number of keys
func (c *Cache) Store(res *Resource, keys ...string) error {
	b, err := ioutil.ReadAll(res)
	if err != nil {
		return err
	}

	for _, key := range keys {
		path := keyToPath(key)
		log.Printf("writing %q => %q body (%d bytes)", key, path, len(b))
		if err = vfs.WriteFile(c.fs, path, b, 0777); err != nil {
			return err
		}
		c.headers[key] = res.Header()
	}

	return nil
}

// Retrieve returns a cached Resource for the given key
func (c *Cache) Retrieve(key string) (*Resource, error) {
	f, err := c.fs.Open(keyToPath(key))
	if err != nil {
		if vfs.IsNotExist(err) {
			return nil, ErrNotFoundInCache
		}
		return nil, err
	}
	return NewResource(http.StatusOK, f.(ReadSeekCloser), c.headers[key]), nil
}

// Purge removes the Resources identified by the provided keys from the cache
func (c *Cache) Purge(keys ...string) (int, error) {
	return 0, nil
}

func keyToPath(key string) string {
	h := sha256.New()
	io.WriteString(h, key)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// NewMapCache returns a Cache based off an empty map
func NewMapCache() *Cache {
	return &Cache{vfs.Memory(), map[string]http.Header{}}
}
