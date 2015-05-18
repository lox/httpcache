package httpcache_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lox/httpcache"
	"github.com/stretchr/testify/require"
)

func TestSaveResource(t *testing.T) {
	var body = strings.Repeat("llamas", 5000)
	var cache = httpcache.NewMemoryCache(uint64(len(body) * 2))

	res, err := httpcache.NewResourceBytes(http.StatusOK, []byte(body), http.Header{
		"Llamas": []string{"true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := cache.Store(res, "testkey"); err != nil {
		t.Fatal(err)
	}

	resOut, err := cache.Retrieve("testkey")
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, resOut)
	require.Equal(t, res.Header(), resOut.Header())

	reader, err := resOut.Reader()
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, body, readAllString(reader))
}

func TestSaveResourceWithIncorrectContentLength(t *testing.T) {
	var body = "llamas"
	var cache = httpcache.NewMemoryCache(100)

	res, err := httpcache.NewResourceBytes(http.StatusOK, []byte(body), http.Header{
		"Llamas":         []string{"true"},
		"Content-Length": []string{"10"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := cache.Store(res, "testkey"); err == nil {
		t.Fatal("Entry should have generated an error")
	}

	_, err = cache.Retrieve("testkey")
	if err != httpcache.ErrNotFoundInCache {
		t.Fatal("Entry shouldn't have been cached")
	}
}
