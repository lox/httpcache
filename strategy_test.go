package httpcache_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lox/httpcache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestMethodCacheability(t *testing.T) {
	reqs := []struct {
		method    string
		cc        string
		cacheable bool
	}{
		{"GET", "max-age=60", true},
		{"HEAD", "max-age=60", true},
		{"POST", "max-age=60", false},
		{"OPTIONS", "max-age=60", false},
		{"PUT", "max-age=60", false},
		{"GETX", "max-age=60", false},
	}

	for _, req := range reqs {
		s := &httpcache.DefaultStrategy{}
		resp := newResponse(
			http.StatusOK, "llamas", fmt.Sprintf("Cache-Control: %s", req.cc),
		)

		assert.Equal(t,
			s.IsCacheable(newRequest(req.method, "http://example.org"), resp),
			req.cacheable,
			fmt.Sprintf("IsCacheable(%q) should equal %v", req.method, req.cacheable),
		)
	}
}

func TestStatusCodeCacheability(t *testing.T) {
	reqs := []struct {
		statusCode int
		cc         string
		cacheable  bool
	}{
		{http.StatusOK, "max-age=60", true},
		{http.StatusNotModified, "max-age=60", true},
		{http.StatusNotImplemented, "max-age=60", false},
		{http.StatusUnauthorized, "max-age=60", false},
	}

	for _, req := range reqs {
		s := &httpcache.DefaultStrategy{}
		resp := newResponse(
			req.statusCode, "llamas", fmt.Sprintf("Cache-Control: %s", req.cc),
		)

		assert.Equal(t,
			s.IsCacheable(newRequest("GET", "http://example.org"), resp),
			req.cacheable,
			fmt.Sprintf("IsCacheable(%q) should equal %v", http.StatusText(req.statusCode), req.cacheable),
		)
	}
}

func TestAge(t *testing.T) {
	ts := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	s := &httpcache.DefaultStrategy{
		NowFunc: func() time.Time { return ts },
	}

	req := newRequest("GET", "http://example.org")
	resp := newResponse(
		http.StatusOK, "llamas",
		"Date: "+ts.Add(time.Hour*-2).Format(http.TimeFormat),
	)

	age, err := s.Age(req, resp)

	require.NoError(t, err)
	assert.Equal(t, age.String(), time.Duration(time.Hour*2).String())
}

func TestFreshness(t *testing.T) {
	ts := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	s := &httpcache.DefaultStrategy{
		NowFunc: func() time.Time { return ts },
		Shared:  true,
	}

	resp := newResponse(
		http.StatusOK, "llamas",
		"Date: "+ts.Add(time.Hour*-2).Format(http.TimeFormat),
		"Expires: "+ts.Add(time.Hour*5).Format(http.TimeFormat),
		"Cache-Control: max-age=60, s-maxage=40",
	)

	f1, err := s.Freshness(newRequest("GET", "http://example.org"), resp)

	require.NoError(t, err)
	assert.Equal(t, f1.String(), time.Duration(time.Second*40).String())

	s.Shared = false
	f2, err := s.Freshness(newRequest("GET", "http://example.org"), resp)

	require.NoError(t, err)
	assert.Equal(t, f2.String(), time.Duration(time.Second*60).String())
}
