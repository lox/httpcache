package httpcache

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/lox/httpcache/store"
)

const (
	lastModDivisor = 10
)

var Clock = func() time.Time {
	return time.Now()
}

type Resource struct {
	*http.Response
	cc     CacheControl
	closer io.Closer
}

func NewResource() *Resource {
	return &Resource{
		Response: &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte{})),
			Header:     make(http.Header),
		},
	}
}

func NewResourceString(s string) *Resource {
	res := NewResource()
	res.Body = ioutil.NopCloser(bytes.NewReader([]byte(s)))
	return res
}

func NewResourceResponse(resp *http.Response) *Resource {
	return &Resource{
		Response: resp,
	}
}

func LoadResource(key string, r *http.Request, s store.Store) (*Resource, error) {
	rc, err := s.Read(key)

	if store.IsNotExists(err) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(rc), r)
	if err != nil {
		return nil, err
	}

	return &Resource{Response: resp, closer: rc}, nil
}

func (r *Resource) Save(key string, s store.Store) error {
	errorCh := make(chan error)
	rd, wr := io.Pipe()

	go func() {
		if err := r.Response.Write(wr); err != nil {
			errorCh <- err
		}
		wr.Close()
		close(errorCh)
	}()

	if err := s.WriteFrom(key, rd); err != nil {
		return err
	}

	return <-errorCh
}

func (r *Resource) cacheControl() (CacheControl, error) {
	if r.cc != nil {
		return r.cc, nil
	}

	cc, err := ParseCacheControl(r.Header.Get("Cache-Control"))
	if err != nil {
		return cc, err
	}

	r.cc = cc
	return cc, nil
}

func (r *Resource) LastModified() time.Time {
	var modTime time.Time

	if lastModHeader := r.Header.Get("Last-Modified"); lastModHeader != "" {
		if t, err := http.ParseTime(lastModHeader); err == nil {
			modTime = t
		}
	}

	return modTime
}

func (r *Resource) Expires() (time.Time, error) {
	if expires := r.Header.Get("Expires"); expires != "" {
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

	if ageHeader := r.Header.Get("Age"); ageHeader != "" {
		if ageInt, err := strconv.Atoi(ageHeader); err == nil {
			age = time.Second * time.Duration(ageInt)
		}
	}

	if dateHeader := r.Header.Get("Date"); dateHeader != "" {
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

	if expiresVal := r.Header.Get("Expires"); expiresVal != "" {
		expires, err := http.ParseTime(expiresVal)
		if err != nil {
			return time.Duration(0), err
		}
		return expires.Sub(Clock()), nil
	}

	return time.Duration(0), nil
}

func (r *Resource) HasExplicitFreshness() bool {
	cc, err := r.cacheControl()
	if err != nil {
		return false
	}

	return cc.Has("max-age") || cc.Has("public") || r.Header.Get("Expires") != ""
}

func (r *Resource) HeuristicFreshness() time.Duration {
	if r.Header.Get("Last-Modified") != "" && !r.HasExplicitFreshness() {
		return Clock().Sub(r.LastModified()) / time.Duration(lastModDivisor)
	}

	return time.Duration(0)
}

func (r *Resource) IsCacheable(shared bool) bool {
	if !isStatusCodeCacheable(r.StatusCode) {
		return false
	}

	cc, err := r.cacheControl()
	if err != nil {
		log.Println("Error parsing cache-control: ", err)
		return false
	}

	if cc.Has("no-cache") {
		return false
	}

	if cc.Has("no-store") {
		return false
	}

	if cc.Has("private") && shared {
		return false
	}

	if r.Header.Get("Authorization") != "" {
		return false
	}

	if r.Header.Get("Last-Modified") != "" || r.Header.Get("Etag") != "" {
		return true
	}

	if r.Header.Get("Expires") != "" {
		if _, err := r.Expires(); err != nil {
			return false
		}
	}

	if r.HasExplicitFreshness() {
		return true
	}

	return false
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

func (r *Resource) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for key, headers := range r.Header {
		for _, header := range headers {
			w.Header().Add(key, header)
		}
	}

	age, err := r.Age()
	if err != nil {
		http.Error(w, "Error calculating age: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	warnings, _ := r.Warnings()
	for _, warn := range warnings {
		w.Header().Add("Warning", warn)
	}

	w.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error reading resource", err)
		http.Error(w, "Error reading resource: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, req, "", r.LastModified(), bytes.NewReader(b))

	if err := r.Close(); err != nil {
		log.Println("Error closing resource", err)
	}
}

func (r *Resource) Close() error {
	return r.closer.Close()
}
