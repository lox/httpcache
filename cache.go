package httpcache

import (
	"fmt"
	"net/http"
	"time"
)

type Cache struct {
	Store
	Private   bool
	Validator *Validator
}

func NewPrivateCache() *Cache {
	return &Cache{
		Store:     NewMapStore(),
		Private:   true,
		Validator: &Validator{http.DefaultTransport},
	}
}

func NewPublicCache() *Cache {
	return &Cache{
		Store:     NewMapStore(),
		Private:   false,
		Validator: &Validator{http.DefaultTransport},
	}
}

func (c *Cache) IsFresh(req *http.Request, res *Resource, t time.Time) (ok bool, msg string, err error) {
	var dur time.Duration

	if c.Private {
		dur, err = res.Freshness(t)
		if err != nil {
			msg = "Failed to determine freshness: " + err.Error()
			return
		}
	} else {
		dur, err = res.SharedFreshness(t)
		if err != nil {
			msg = "Failed to determine freshness: " + err.Error()
			return
		}
	}

	reqCacheControl, err := ParseCacheControl(req.Header.Get(CacheControlHeader))
	if err != nil {
		return false, "Failed to parse Cache-Control: " + err.Error(), err
	}

	if reqCacheControl.MaxAge != nil {
		dur = *reqCacheControl.MaxAge
	}

	age, err := res.Age(t)
	if err != nil {
		msg = "Failed to parse Age: " + err.Error()
		return
	}

	if remaining := int(dur - age); remaining > 0 {
		return true, fmt.Sprintf("%d seconds remaining", remaining), nil
	}

	return false, "No freshness indicators", nil
}

func (c *Cache) IsStoreable(res *Resource) (bool, string, error) {
	if ok, reason, err := c.IsCacheable(res); !ok {
		return ok, reason, err
	}

	cc, err := res.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.NoStore {
		return false, "Response contained no-store", err
	}

	if cc.Private && !c.Private {
		return false, "Response is private", nil
	}

	return true, "", nil
}

func (c *Cache) IsCacheable(res *Resource) (bool, string, error) {
	cc, err := res.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.NoCache {
		return false, "Response contained no-cache", err
	}

	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.4
	allowed := []int{
		http.StatusOK,
		http.StatusFound,
		http.StatusNotModified,
		http.StatusNonAuthoritativeInfo,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusGone,
	}

	for _, a := range allowed {
		if a == res.StatusCode {
			return true, "", nil
		}
	}

	msg := fmt.Sprintf("Response code %d isn't cacheable", res.StatusCode)
	return false, msg, nil
}

func (c *Cache) IsRequestCacheable(req *http.Request) (bool, string, error) {
	cc, err := ParseCacheControl(req.Header.Get(CacheControlHeader))
	if err != nil {
		return false, err.Error(), err
	}

	// BUG(lox) technically anything with explicity freshness headers
	// can be cached, but we don't support it yet
	if req.Method != "GET" && req.Method != "HEAD" {
		return false, "Non-GET/HEAD requests not cached", nil
	}

	if cc.NoCache {
		return false, "Request contains no-cache", nil
	}

	return true, "", nil
}
