package httpcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheControl(t *testing.T) {
	cc, err := ParseCacheControl(`public, private="set-cookie", max-age=100`)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, cc.Public)
	assert.Equal(t, false, cc.Private)
	assert.Equal(t, time.Second*100, *cc.MaxAge)
	assert.Equal(t, []string{"Set-Cookie"}, cc.PrivateFields)
}

func TestCacheControlParsingQuotes(t *testing.T) {
	cc, err := ParseCacheControl(` foo="max-age=8",  public`)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"max-age=8"}, cc.Extension["foo"])
	assert.Equal(t, true, cc.Public)
}
