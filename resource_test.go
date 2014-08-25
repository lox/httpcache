package httpcache_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lox/httpcache"
	"github.com/lox/httpcache/store"
	"github.com/stretchr/testify/require"
)

func TestSaveResource(t *testing.T) {
	var body string = strings.Repeat("llamas", 5000)
	var store = store.NewMapStore()

	res := httpcache.NewResourceResponse(
		newResponse(http.StatusOK, []byte(body), "Llamas: true"))

	res.Header.Add("Content-Length", "30000")
	res.ContentLength = 30000

	if err := res.Save("test", store); err != nil {
		t.Fatal(err)
	}

	resOut, err := httpcache.LoadResource("test", nil, store)
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, resOut)
	require.Equal(t, res.Header, resOut.Header)
	require.Equal(t, body, readAllString(resOut.Body))
}
