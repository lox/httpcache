package httpcache

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type Transport struct {
	Transport http.RoundTripper
	Strategy  Strategy
	Cache     *Cache
}

func NewTransport(cache *Cache) *Transport {
	return &Transport{
		Cache:     cache,
		Transport: http.DefaultTransport,
		Strategy:  &DefaultStrategy{Shared: cache.shared},
	}
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	key := Key(r.Method, r.URL)
	resp, found := t.Cache.Lookup(key)
	if found {
		freshness, err := t.Strategy.Freshness(r, resp)
		if err != nil {
			return transportError(http.StatusGatewayTimeout,
				"Error calculating freshness: "+err.Error(),
			), nil
		}

		if freshness > 0 {
			resp.Header.Set(CacheHeader, "HIT")
			return resp, nil
		}

		resp, err := t.Validate(resp)
		if err != nil {
			return resp, err
		} else {
			t.Cache.Store(key, resp)
			return resp, nil
		}
	}

	resp, err := t.Transport.RoundTrip(r)
	if err != nil {
		return resp, err
	}

	t.Cache.Store(key, resp)
	resp.Header.Set(CacheHeader, "MISS")
	return resp, nil
}

func (t *Transport) Validate(resp *http.Response) (*http.Response, error) {
	return resp, nil
}

func transportError(statusCode int, msg string) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: int64(len(msg)),
		Body:          ioutil.NopCloser(strings.NewReader(msg)),
		Header:        http.Header{},
	}
}
