package httpcache

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

type Entity struct {
	Header     http.Header
	Body       io.ReadSeeker
	StatusCode int
	Method     string
}

func NewEntity(method string, code int, h http.Header, body io.ReadSeeker) *Entity {
	return &Entity{
		Header:     h,
		Body:       body,
		Method:     method,
		StatusCode: code,
	}
}

func (e *Entity) CacheControl() (CacheControl, error) {
	return ParseCacheControl(e.Header.Get(CacheControlHeader))
}

func (e *Entity) BodyString() (string, error) {
	b, err := ioutil.ReadAll(e.Body)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (e *Entity) IsCacheable() (bool, error) {
	cc, err := e.CacheControl()
	if err != nil {
		return false, err
	}

	if cc.NoCache {
		return false, err
	}

	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.4
	allowed := []int{
		http.StatusOK,
		http.StatusFound,
		http.StatusNonAuthoritativeInfo,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusGone,
	}

	for _, a := range allowed {
		if a == e.StatusCode {
			return true, nil
		}
	}

	return false, nil
}

func (e *Entity) Dump(body bool) {
	fmt.Printf("HTTP/1.1 %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	e.Header.Write(os.Stdout)
	if body {
		rb, err := ioutil.ReadAll(e.Body)
		if err != nil {
			panic(err)
		}
		fmt.Printf("\n%s\n", rb)
	}
}

func (e *Entity) hasHeader(key string) bool {
	_, exists := e.Header[key]
	return exists
}

func (e *Entity) LastModified() (time.Time, bool, error) {
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
func (e *Entity) Age(now time.Time) (time.Duration, error) {
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
func (e *Entity) Expires() (time.Time, error) {
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
func (e *Entity) Freshness(now time.Time) (time.Duration, error) {
	cc, err := e.CacheControl()
	if err != nil {
		log.Printf("Failed parsing cache-control: %s", err)
		return time.Duration(0), err
	}

	if cc.MaxAge != nil {
		return *cc.MaxAge, err
	}

	expires, err := e.Expires()
	if err != nil || expires.IsZero() {
		return time.Duration(0), err
	}

	return expires.Sub(now), nil
}

// SharedFreshness returns the freshness lifetime of an entity for a shared cache
func (e *Entity) SharedFreshness(now time.Time) (time.Duration, error) {
	freshness, err := e.Freshness(now)
	if err != nil {
		return time.Duration(0), err
	}

	cc, err := e.CacheControl()
	if err != nil {
		return time.Duration(0), err
	}

	if cc.SMaxAge != nil && *cc.SMaxAge > freshness {
		return *cc.SMaxAge, nil
	}

	return freshness, nil
}
