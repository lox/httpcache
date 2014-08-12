package httpcache

import (
	"fmt"
	"net/http"
	"time"
)

type Cache struct {
	Store
	Private bool
}

func NewPrivateCache() *Cache {
	return &Cache{
		Store:   NewMapStore(),
		Private: true,
	}
}

func NewPublicCache() *Cache {
	return &Cache{
		Store:   NewMapStore(),
		Private: false,
	}
}

func (c *Cache) IsFresh(req *http.Request, res *Resource, t time.Time) (ok bool, stale time.Duration, msg string, err error) {
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
		return false, 0, "Failed to parse Cache-Control: " + err.Error(), err
	}

	if reqCacheControl.Has("max-age") {
		dur, _ = reqCacheControl.Duration("max-age")
	}

	age, err := res.Age(t)
	if err != nil {
		msg = "Failed to parse Age: " + err.Error()
		return
	}

	stale = age - dur
	ok = stale <= 0
	msg = fmt.Sprintf("Stale by %.fs", stale.Seconds())

	return
}

func (c *Cache) IsStoreable(res *Resource) (bool, string, error) {
	if ok, reason, err := c.IsCacheable(res); !ok {
		return ok, reason, err
	}

	cc, err := res.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.Has("no-store") {
		return false, "Response contained no-store", err
	}

	if cc.Has("private") && !c.Private {
		return false, "Response is private", nil
	}

	return true, "", nil
}

func (c *Cache) IsCacheable(res *Resource) (bool, string, error) {
	cc, err := res.CacheControl()
	if err != nil {
		return false, err.Error(), err
	}

	if cc.Has("no-cache") {
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

	if cc.Has("no-cache") {
		return false, "Request contains no-cache", nil
	}

	return true, "", nil
}
