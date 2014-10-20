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
	var cache = httpcache.NewMemoryCache()

	res := httpcache.NewResourceBytes(http.StatusOK, []byte(body), http.Header{
		"Llamas": []string{"true"},
	})

	if err := cache.Store(res, "testkey"); err != nil {
		t.Fatal(err)
	}

	resOut, err := cache.Retrieve("testkey")
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, resOut)
	require.Equal(t, res.Header(), resOut.Header())
	require.Equal(t, body, readAllString(resOut))
}
