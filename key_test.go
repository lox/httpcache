package httpcache

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeysDiffer(t *testing.T) {
	k1, err := Key("GET", "http://example.org/test")
	if err != nil {
		t.Fatal(err)
	}

	k2, err := Key("GET", "http://example.org/test/llamas")
	if err != nil {
		t.Fatal(err)
	}

	assert.NotEqual(t, k1, k2)
}

func TestSecondaryKeysDiffer(t *testing.T) {
	k1, err := Key("GET", "http://example.org/test")
	if err != nil {
		t.Fatal(err)
	}

	k2, err := SecondaryKey("GET", "http://example.org/test/", http.Header{
		"X-Test": []string{"Test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.NotEqual(t, k1, k2)
}
