package httpcache_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/lox/httpcache"
	"github.com/stretchr/testify/assert"
)

func TestStoringSingleResource(t *testing.T) {
	s := httpcache.NewMapStore()
	resp := newResponse(http.StatusOK, "tests")

	s.Set("test", resp)

	respRet, ok, err := s.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Failed to find resource by key")
	}

	assert.Equal(t, respRet, resp)

	b, err := ioutil.ReadAll(respRet.Body)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "tests", string(b))

	_, notok, _ := s.Get("not there")
	if notok {
		t.Fatal("Should have failed to find resource")
	}
}
