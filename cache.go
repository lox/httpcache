package httpcache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"os"
	pathutil "path"

	vfs "gopkgs.com/vfs.v1"
)

const (
	headerPrefix = "header/"
	bodyPrefix   = "body/"
)

// Returned when a resource doesn't exist
var ErrNotFoundInCache = errors.New("Not found in cache")

// Cache provides a storage mechanism for cached Resources
type Cache struct {
	fs vfs.VFS
}

// NewCache returns a Cache backed off the provided VFS
func NewCache(fs vfs.VFS) *Cache {
	return &Cache{fs}
}

// NewMemoryCache returns an ephemeral cache in memory
func NewMemoryCache() *Cache {
	return &Cache{vfs.Memory()}
}

func (c *Cache) vfsWrite(path string, r io.Reader) error {
	if err := vfs.MkdirAll(c.fs, pathutil.Dir(path), 0700); err != nil {
		return err
	}
	f, err := c.fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return nil
}

// Retrieve the Headers for a given key path
func (c *Cache) Header(key string) (http.Header, error) {
	path := headerPrefix + hashKey(key)
	f, err := c.fs.Open(path)
	if err != nil {
		if vfs.IsNotExist(err) {
			return nil, ErrNotFoundInCache
		}
		return nil, err
	}

	tp := textproto.NewReader(bufio.NewReader(f))
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	return http.Header(mimeHeader), nil
}

// Store a resource against a number of keys
func (c *Cache) Store(res *Resource, keys ...string) error {
	b, err := ioutil.ReadAll(res)
	if err != nil {
		return err
	}
	h := &bytes.Buffer{}
	if err = res.Header().Write(h); err != nil {
		return err
	}
	// ReadMIMEHeader expects a trailing newline
	h.Write([]byte("\r\n"))

	for _, key := range keys {
		if err := c.vfsWrite(bodyPrefix+hashKey(key), bytes.NewReader(b)); err != nil {
			return err
		}
		if err := c.vfsWrite(headerPrefix+hashKey(key), bytes.NewReader(h.Bytes())); err != nil {
			return err
		}
	}

	return nil
}

// Retrieve returns a cached Resource for the given key
func (c *Cache) Retrieve(key string) (*Resource, error) {
	f, err := c.fs.Open(bodyPrefix + hashKey(key))
	if err != nil {
		if vfs.IsNotExist(err) {
			return nil, ErrNotFoundInCache
		}
		return nil, err
	}
	h, err := c.Header(key)
	if err != nil {
		if vfs.IsNotExist(err) {
			return nil, ErrNotFoundInCache
		}
		return nil, err
	}
	return NewResource(http.StatusOK, f, h), nil
}

// Purge removes the Resources identified by the provided keys from the cache
func (c *Cache) Purge(keys ...string) (int, error) {
	return 0, errors.New("Purge not implemented yet")
}

func hashKey(key string) string {
	h := sha256.New()
	io.WriteString(h, key)
	return fmt.Sprintf("%x", h.Sum(nil))
}
