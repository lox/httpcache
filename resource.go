package httpcache

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Resource struct {
	Header     http.Header
	Body       io.ReadSeeker
	StatusCode int
	Method     string
	URL        *url.URL
}

func (e *Resource) Key() string {
	return Key(e.Method, e.URL)
}

func (e *Resource) CacheControl() (CacheControl, error) {
	return ParseCacheControl(e.Header.Get(CacheControlHeader))
}

func (e *Resource) BodyString() (string, error) {
	b, err := ioutil.ReadAll(e.Body)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (e *Resource) Dump(body bool) {
	fmt.Printf("HTTP/1.1 %d %s\n", e.StatusCode, http.StatusText(e.StatusCode))
	e.Header.Write(os.Stdout)
	if body {
		rb, err := ioutil.ReadAll(e.Body)
		if err != nil {
			panic(err)
		}
		fmt.Printf("\n%s\n", rb)
	}
}

func (e *Resource) hasHeader(key string) bool {
	_, exists := e.Header[key]
	return exists
}

func (e *Resource) LastModified() (time.Time, bool, error) {
	if !e.hasHeader("Last-Modified") {
		return time.Time{}, false, nil
	}

	date, err := http.ParseTime(e.Header.Get("Last-Modified"))
	if err != nil {
		return time.Time{}, false, err
	}

	return date, true, nil
}

// Age returns the time difference between when a requests
// was created and an abitrary time
func (e *Resource) Age(now time.Time) (time.Duration, error) {
	dateHeader := e.Header.Get("Date")
	if dateHeader == "" {
		return time.Duration(0), nil
	}

	date, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Duration(0), err
	}
	return now.Sub(date), nil
}

// Expires parses an Expires header
func (e *Resource) Expires() (time.Time, error) {
	expires := e.Header.Get("Expires")
	if expires == "" {
		return time.Time{}, nil
	}

	expTime, err := http.ParseTime(expires)
	if err != nil {
		return time.Time{}, err
	}

	return expTime, nil
}

// Freshness returns how long the entity is fresh for
func (e *Resource) Freshness(now time.Time) (time.Duration, error) {
	cc, err := e.CacheControl()
	if err != nil {
		log.Printf("Failed parsing cache-control: %s", err)
		return time.Duration(0), err
	}

	if cc.Has("max-age") {
		return cc.Duration("max-age")
	}

	expires, err := e.Expires()
	if err != nil || expires.IsZero() {
		return time.Duration(0), err
	}

	return expires.Sub(now), nil
}

// SharedFreshness returns the freshness lifetime of an entity for a shared cache
func (e *Resource) SharedFreshness(now time.Time) (time.Duration, error) {
	freshness, err := e.Freshness(now)
	if err != nil {
		return time.Duration(0), err
	}

	cc, err := e.CacheControl()
	if err != nil {
		return time.Duration(0), err
	}

	if cc.Has("s-maxage") {
		return cc.Duration("s-maxage")
	}

	return freshness, nil
}
