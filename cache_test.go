package httpcache

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestStoringInCache(t *testing.T) {
	cache := NewPublicCache()
	ent := &Entity{
		Header: make(http.Header),
		Body:   strings.NewReader("tests"),
	}

	if err := cache.Store("test", ent); err != nil {
		t.Fatal(err)
	}

	entRet, err := cache.Retrieve("test")
	if err != nil {
		t.Fatal(err)
	} else if entRet != ent {
		t.Fatal("Retrieved entity isn't the same as stored")
	}

	b, err := ioutil.ReadAll(entRet.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(b) != "tests" {
		log.Printf("Expected a body of %q, got %q", "tests", string(b))
	}

}
