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
	r1 := &Resource{
		Header: make(http.Header),
		Body:   strings.NewReader("tests"),
	}

	if err := cache.Set("test", r1); err != nil {
		t.Fatal(err)
	}

	r2, ok := cache.Get("test")
	if !ok {
		t.Fatal("Failed to find resource by key")
	} else if r1 != r2 {
		t.Fatal("Retrieved resource isn't the same as stored")
	}

	b, err := ioutil.ReadAll(r2.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(b) != "tests" {
		log.Printf("Expected a body of %q, got %q", "tests", string(b))
	}

}
