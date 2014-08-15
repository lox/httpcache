package httpcache

import (
	"log"
	"net/http"
	"time"
)

// http://httpwg.github.io/specs/rfc7234.html

// A cache MUST NOT store a response to any request, unless the response either:
// contains an Expires header field (see Section 5.3), or
// contains a max-age response directive (see Section 5.2.2.8), or
// contains a s-maxage response directive (see Section 5.2.2.9) and the cache is shared, or
// contains a Cache Control Extension (see Section 5.2.3) that allows it to be cached, or
// has a status code that is defined as cacheable by default (see Section 4.2.2), or
// contains a public response directive (see Section 5.2.2.5).

type Strategy interface {
	IsCacheable(r *http.Request, resp *http.Response) bool
	Freshness(r *http.Request, resp *http.Response) (time.Duration, error)
	Age(r *http.Request, resp *http.Response) (time.Duration, error)
}

type DefaultStrategy struct {
	Shared  bool
	NowFunc func() time.Time
}

// now returns time.Now(), or the value of DefaultStrategy.Now
func (d *DefaultStrategy) Now() time.Time {
	if d.NowFunc != nil {
		return d.NowFunc()
	}

	return time.Now()
}

func (d *DefaultStrategy) IsCacheable(r *http.Request, resp *http.Response) bool {
	if !isRequestMethodCacheable(r) {
		logRequest(r, "Request method not cacheable")
		return false
	}

	if !isStatusCodeCacheable(resp) {
		logRequest(r, "Response status not cacheable")
		return false
	}

	reqCC, err := ParseCacheControl(r.Header.Get(CacheControlHeader))
	if err != nil {
		logRequest(r, "Error parsing cache-control: "+err.Error())
		return false
	}

	respCC, err := ParseCacheControl(resp.Header.Get(CacheControlHeader))
	if err != nil {
		logRequest(r, "Error parsing cache-control: "+err.Error())
		return false
	}

	if reqCC.Has("no-store") || respCC.Has("no-store") {
		logRequest(r, "Has no-store")
		return false
	}

	if respCC.Has("private") && d.Shared {
		logRequest(r, "Has private")
		return false
	}

	if resp.Header.Get("Authorization") != "" && !isAuthorizationAllowed(resp) {
		logRequest(r, "Has Authorization")
		return false
	}

	freshness, err := d.Freshness(r, resp)
	if err != nil {
		logRequest(r, "Error calculating freshness: "+err.Error())
	} else if freshness > 0 {
		logRequest(r, "Has freshness "+freshness.String())
		return true
	}

	logRequest(r, "No freshness indicators")
	return false
}

func (d *DefaultStrategy) Freshness(r *http.Request, resp *http.Response) (time.Duration, error) {
	// reqCC, err := ParseCacheControl(r.Header.Get(CacheControlHeader))
	// if err != nil {
	// 	return time.Duration(0), err
	// }

	respCC, err := ParseCacheControl(resp.Header.Get(CacheControlHeader))
	if err != nil {
		return time.Duration(0), err
	}

	if respCC.Has("s-maxage") && d.Shared {
		if maxAge, err := respCC.Duration("s-maxage"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if respCC.Has("max-age") {
		if maxAge, err := respCC.Duration("max-age"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if expiresVal := resp.Header.Get("Expires"); expiresVal != "" {
		expires, err := http.ParseTime(expiresVal)
		if err != nil {
			return time.Duration(0), err
		}
		return expires.Sub(d.Now()), nil
	}

	return heuristicFreshness(resp), nil
}

// apparent_age = max(0, response_time - date_value);
// response_delay = response_time - request_time;
// corrected_age_value = age_value + response_delay;

func (d *DefaultStrategy) Age(r *http.Request, resp *http.Response) (time.Duration, error) {
	responseTime, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return time.Duration(0), err
	}

	apparentAge := d.Now().Sub(responseTime)
	if apparentAge < 0 {
		apparentAge = time.Duration(0)
	}

	return apparentAge, nil
}

func isAuthorizationAllowed(resp *http.Response) bool {
	return false
}

func isStatusCodeCacheable(resp *http.Response) bool {
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
		if a == resp.StatusCode {
			return true
		}
	}

	return false
}

func isRequestMethodCacheable(r *http.Request) bool {
	return r.Method == "GET" || r.Method == "HEAD"
}

func heuristicFreshness(r *http.Response) time.Duration {
	return time.Duration(0)
}

func logRequest(r *http.Request, msg string) {
	log.Printf("[%s %s] %s", r.Method, r.URL.String(), msg)
}
