package httpcache

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	lastModDivisor = 10
	viaPseudonym   = "httpcache"
)

var Clock = func() time.Time {
	return time.Now().UTC()
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
	RequestTime, ResponseTime time.Time
	header                    http.Header
	statusCode                int
	cc                        CacheControl
	stale                     bool
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
		cc:             ParseCacheControlHeaders(hdrs),
		header:         hdrs,
		statusCode:     statusCode,
		ReadSeekCloser: &byteReadSeekCloser{bytes.NewReader(b)},
	}
}

func (r *Resource) IsNonErrorStatus() bool {
	return r.statusCode >= 200 && r.statusCode < 400
}

func (r *Resource) Status() int {
	return r.statusCode
}

func (r *Resource) Header() http.Header {
	return r.header
}

func (r *Resource) IsStale() bool {
	return r.stale
}

func (r *Resource) MarkStale() {
	r.stale = true
}

func (r *Resource) SetDefaultCacheControl() {
	r.cc = CacheControl{
		"public":  nil,
		"max-age": []string{"3600"},
	}
	r.header.Set(CacheControlHeader, "public, max-age=3600")
}

func (r *Resource) cacheControl() CacheControl {
	if r.cc != nil {
		return r.cc
	}

	cc := ParseCacheControlHeaders(r.header)
	r.cc = cc
	return cc
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

func (r *Resource) Expires() time.Time {
	if expires := r.header.Get("Expires"); expires != "" {
		exp, err := http.ParseTime(expires)
		if err != nil {
			return Clock().Add(-1 * time.Second)
		}
		return exp
	}

	return time.Time{}
}

func (r *Resource) MustValidate(shared bool) bool {
	cc := r.cacheControl()
	// The s-maxage directive also implies the semantics of proxy-revalidate
	if cc.Has("s-maxage") && shared {
		return true
	}

	if cc.Has("must-revalidate") || (cc.Has("proxy-revalidate") && shared) {
		return true
	}

	return false
}

func (r *Resource) DateAfter(d time.Time) bool {
	if dateHeader := r.header.Get("Date"); dateHeader != "" {
		if t, err := http.ParseTime(dateHeader); err != nil {
			return false
		} else {
			return t.After(d)
		}
	}
	return false
}

// Calculate the age of the resource
func (r *Resource) Age() (time.Duration, error) {
	var age time.Duration

	if ageInt, err := intHeader("Age", r.header); err == nil {
		age = time.Second * time.Duration(ageInt)
	}

	if proxyDate, err := timeHeader(ProxyDateHeader, r.header); err == nil {
		return Clock().Sub(proxyDate) + age, nil
	}

	if date, err := timeHeader("Date", r.header); err == nil {
		return Clock().Sub(date) + age, nil
	}

	return time.Duration(0), errors.New("Unable to calculate age")
}

func (r *Resource) MaxAge(shared bool) (time.Duration, error) {
	cc := r.cacheControl()

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

	return r.Expires().Sub(Clock()), nil
}

func (r *Resource) RemovePrivateHeaders() {
	cc := r.cacheControl()

	for _, p := range cc["private"] {
		debugf("removing private header %q", p)
		r.header.Del(p)
	}
}

func (r *Resource) HasValidators() bool {
	if r.header.Get("Last-Modified") != "" || r.header.Get("Etag") != "" {
		return true
	}

	return false
}

func (r *Resource) HasAnyCacheControls() bool {
	return len(r.cc) > 0 || r.HasValidators() || r.HasExplicitExpiration() || r.header.Get("Expires") != ""
}

func (r *Resource) HasExplicitExpiration() bool {
	cc := r.cacheControl()

	if d, _ := cc.Duration("max-age"); d > time.Duration(0) {
		return true
	}

	if d, _ := cc.Duration("s-maxage"); d > time.Duration(0) {
		return true
	}

	if exp := r.Expires(); exp.After(Clock()) {
		return true
	}

	return false
}

func (r *Resource) HeuristicFreshness() time.Duration {
	if !r.HasExplicitExpiration() && r.header.Get("Last-Modified") != "" {
		return Clock().Sub(r.LastModified()) / time.Duration(lastModDivisor)
	}

	return time.Duration(0)
}

func (r *Resource) Via() string {
	via := []string{}
	via = append(via, fmt.Sprintf("1.1 %s", viaPseudonym))
	return strings.Join(via, ",")
}
