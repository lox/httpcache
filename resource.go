package httpcache

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	lastModDivisor = 10
)

var Clock = func() time.Time {
	return time.Now()
}

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type byteReadSeekCloser struct {
	*bytes.Reader
}

func (brsc *byteReadSeekCloser) Close() error { return nil }

type Resource struct {
	ReadSeekCloser
	header     http.Header
	statusCode int
	cc         CacheControl
}

func NewResource(statusCode int, body ReadSeekCloser, hdrs http.Header) *Resource {
	return &Resource{
		header:         hdrs,
		ReadSeekCloser: body,
		statusCode:     statusCode,
	}
}

func NewResourceBytes(statusCode int, b []byte, hdrs http.Header) *Resource {
	return &Resource{
		header:         hdrs,
		statusCode:     statusCode,
		ReadSeekCloser: &byteReadSeekCloser{bytes.NewReader(b)},
	}
}

func (r *Resource) Status() int {
	return r.statusCode
}

func (r *Resource) Header() http.Header {
	return r.header
}

func (r *Resource) cacheControl() (CacheControl, error) {
	if r.cc != nil {
		return r.cc, nil
	}
	cc, err := ParseCacheControl(r.header.Get("Cache-Control"))
	if err != nil {
		return cc, err
	}

	r.cc = cc
	return cc, nil
}

func (r *Resource) LastModified() time.Time {
	var modTime time.Time

	if lastModHeader := r.header.Get("Last-Modified"); lastModHeader != "" {
		if t, err := http.ParseTime(lastModHeader); err == nil {
			modTime = t
		}
	}

	return modTime
}

func (r *Resource) Expires() (time.Time, error) {
	if expires := r.header.Get("Expires"); expires != "" {
		return http.ParseTime(expires)
	}

	return time.Time{}, errors.New("No expires header present")
}

func (r *Resource) MustValidate() bool {
	cc, err := r.cacheControl()
	if err != nil {
		log.Printf("Error parsing Cache-Control: ", err.Error())
	}

	if cc.Has("must-validate") {
		return true
	}

	return false
}

func (r *Resource) Age() (time.Duration, error) {
	var age time.Duration

	if ageHeader := r.header.Get("Age"); ageHeader != "" {
		if ageInt, err := strconv.Atoi(ageHeader); err == nil {
			age = time.Second * time.Duration(ageInt)
		}
	}

	if dateHeader := r.header.Get("Date"); dateHeader != "" {
		if t, err := http.ParseTime(dateHeader); err != nil {
			return time.Duration(0), err
		} else {
			return Clock().Sub(t) + age, nil
		}
	}

	return time.Duration(0), errors.New("Unable to calculate age")
}

func (r *Resource) MaxAge(shared bool) (time.Duration, error) {
	cc, err := r.cacheControl()
	if err != nil {
		return time.Duration(0), err
	}

	if cc.Has("s-maxage") && shared {
		if maxAge, err := cc.Duration("s-maxage"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if cc.Has("max-age") {
		if maxAge, err := cc.Duration("max-age"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if expiresVal := r.header.Get("Expires"); expiresVal != "" {
		expires, err := http.ParseTime(expiresVal)
		if err != nil {
			return time.Duration(0), err
		}
		return expires.Sub(Clock()), nil
	}

	return time.Duration(0), nil
}

func (r *Resource) HasValidators() bool {
	if r.header.Get("Last-Modified") != "" || r.header.Get("Etag") != "" {
		return true
	}

	return false
}

func (r *Resource) HasExplicitFreshness() bool {
	cc, err := r.cacheControl()
	if err != nil {
		return false
	}

	return cc.Has("max-age") || cc.Has("public") || r.header.Get("Expires") != ""
}

func (r *Resource) HeuristicFreshness() time.Duration {
	if r.header.Get("Last-Modified") != "" && !r.HasExplicitFreshness() {
		return Clock().Sub(r.LastModified()) / time.Duration(lastModDivisor)
	}

	return time.Duration(0)
}

func isStatusCodeCacheable(status int) bool {
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
		if a == status {
			return true
		}
	}

	return false
}

func (r *Resource) IsUncacheable(shared bool) bool {
	if !isStatusCodeCacheable(r.statusCode) {
		return true
	}

	cc, err := r.cacheControl()
	if err != nil {
		log.Println("Error parsing cache-control: ", err)
		return true
	}

	if cc.Has("no-cache") || cc.Has("no-store") || (cc.Has("private") && shared) {
		return true
	}

	if maxAge, _ := cc.Get("max-age"); maxAge == "0" {
		return true
	}

	if r.header.Get("Authorization") != "" {
		return true
	}

	// malformed expires header is uncacheable
	if _, err = r.Expires(); r.header.Get("Expires") != "" && err != nil {
		return true
	}

	return false
}

func (r *Resource) Warnings() ([]string, error) {
	warns := []string{}

	age, err := r.Age()
	if err != nil {
		return warns, err
	}

	// http://httpwg.github.io/specs/rfc7234.html#warn.113
	if !r.HasExplicitFreshness() {
		if age > (time.Hour*24) && r.HeuristicFreshness() > (time.Hour*24) {
			warns = append(warns, `113 - "Heuristic Expiration"`)
		}
	}

	return warns, nil
}
