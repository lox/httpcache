package httpcache

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoringSingleResource(t *testing.T) {
	s := NewMapStore()
	r1 := &Resource{
		Header: make(http.Header),
		Body:   strings.NewReader("tests"),
	}

	s.Set("test", r1)

	retR1, ok := s.Get("test")
	if !ok {
		t.Fatal("Failed to find resource by key")
	}

	assert.Equal(t, retR1, r1)

	b, err := ioutil.ReadAll(retR1.Body)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "tests", string(b))

	_, notok := s.Get("not there")
	if notok {
		t.Fatal("Should have failed to find resource")
	}
}
