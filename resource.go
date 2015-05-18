package httpcache

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lox/httpcache/storage"
)

const (
	lastModDivisor = 10
	viaPseudonym   = "httpcache"
)

var Clock = func() time.Time {
	return time.Now().UTC()
}

type Resource struct {
	storage.Storable
	RequestTime, ResponseTime time.Time
	CacheControl              CacheControl
	Stale                     bool
}

func NewResource(s storage.Storable) (*Resource, error) {
	cc, err := ParseCacheControlHeaders(s.Header())
	if err != nil {
		return nil, err
	}

	return &Resource{
		CacheControl: cc,
		Storable:     s,
	}, nil
}

func NewResourceBytes(status int, body []byte, h http.Header) (*Resource, error) {
	return NewResource(storage.NewByteStorable(body, status, h))
}

func (r *Resource) IsNonErrorStatus() bool {
	return r.Status() >= 200 && r.Status() < 400
}

func (r *Resource) LastModified() time.Time {
	var modTime time.Time

	if lastModHeader := r.Header().Get("Last-Modified"); lastModHeader != "" {
		if t, err := http.ParseTime(lastModHeader); err == nil {
			modTime = t
		}
	}

	return modTime
}

func (r *Resource) Expires() (time.Time, error) {
	if expires := r.Header().Get("Expires"); expires != "" {
		return http.ParseTime(expires)
	}

	return time.Time{}, nil
}

func (r *Resource) MustValidate(shared bool) bool {
	// The s-maxage directive also implies the semantics of proxy-revalidate
	if r.CacheControl.Has("s-maxage") && shared {
		return true
	}

	if r.CacheControl.Has("must-revalidate") || (r.CacheControl.Has("proxy-revalidate") && shared) {
		return true
	}

	return false
}

func (r *Resource) DateAfter(d time.Time) bool {
	if dateHeader := r.Header().Get("Date"); dateHeader != "" {
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

	if ageInt, err := intHeader("Age", r.Header()); err == nil {
		age = time.Second * time.Duration(ageInt)
	}

	if proxyDate, err := timeHeader(ProxyDateHeader, r.Header()); err == nil {
		return Clock().Sub(proxyDate) + age, nil
	}

	if date, err := timeHeader("Date", r.Header()); err == nil {
		return Clock().Sub(date) + age, nil
	}

	return time.Duration(0), errors.New("Unable to calculate age")
}

func (r *Resource) MaxAge(shared bool) (time.Duration, error) {
	if r.CacheControl.Has("s-maxage") && shared {
		if maxAge, err := r.CacheControl.Duration("s-maxage"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if r.CacheControl.Has("max-age") {
		if maxAge, err := r.CacheControl.Duration("max-age"); err != nil {
			return time.Duration(0), err
		} else if maxAge > 0 {
			return maxAge, nil
		}
	}

	if expiresVal := r.Header().Get("Expires"); expiresVal != "" {
		expires, err := http.ParseTime(expiresVal)
		if err != nil {
			return time.Duration(0), err
		}
		return expires.Sub(Clock()), nil
	}

	return time.Duration(0), nil
}

func (r *Resource) RemovePrivateHeaders() {
	for _, p := range r.CacheControl["private"] {
		debugf("removing private header %q", p)
		r.Header().Del(p)
	}
}

func (r *Resource) HasValidators() bool {
	if r.Header().Get("Last-Modified") != "" || r.Header().Get("Etag") != "" {
		return true
	}

	return false
}

func (r *Resource) HasExplicitExpiration() bool {
	if d, _ := r.CacheControl.Duration("max-age"); d > time.Duration(0) {
		return true
	}

	if d, _ := r.CacheControl.Duration("s-maxage"); d > time.Duration(0) {
		return true
	}

	if exp, _ := r.Expires(); !exp.IsZero() {
		return true
	}

	return false
}

func (r *Resource) HeuristicFreshness() time.Duration {
	if !r.HasExplicitExpiration() && r.Header().Get("Last-Modified") != "" {
		return Clock().Sub(r.LastModified()) / time.Duration(lastModDivisor)
	}

	return time.Duration(0)
}

func (r *Resource) Via() string {
	via := []string{}
	via = append(via, fmt.Sprintf("1.1 %s", viaPseudonym))
	return strings.Join(via, ",")
}
