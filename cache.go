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

func (c *Cache) IsFresh(res *Resource, t time.Time) (ok bool, err error) {
	var dur time.Duration

	if c.Private {
		dur, err = res.Freshness(t)
		if err != nil {
			return
		}
	} else {
		dur, err = res.SharedFreshness(t)
		if err != nil {
			return
		}
	}

	age, err := res.Age(t)
	if err != nil {
		return
	}

	if int(dur-age) > 0 {
		return true, nil
	}

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

	if cc.NoCache {
		return false, "Request contains no-cache", nil
	}

	return true, "", nil
}
