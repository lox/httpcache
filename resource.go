package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

var Clock = func() time.Time {
	return time.Now()
}

type Resource struct {
	ContentLength int64
	StatusCode    int
	Header        http.Header
	Body          io.ReadCloser
	cc            CacheControl
}

func NewResource() *Resource {
	return &Resource{
		Body:   ioutil.NopCloser(bytes.NewReader([]byte{})),
		Header: make(http.Header),
	}
}

func NewResourceString(s string) *Resource {
	return &Resource{
		Header: make(http.Header),
		Body:   ioutil.NopCloser(bytes.NewReader([]byte(s))),
	}
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
	var d time.Duration

	if ageHeader := r.Header.Get("Age"); ageHeader != "" {
		if ageInt, err := strconv.Atoi(ageHeader); err == nil {
			d = time.Second * time.Duration(ageInt)
		}
	}

	if dateHeader := r.Header.Get("Date"); dateHeader != "" {
		if t, err := http.ParseTime(dateHeader); err != nil {
			return d, err
		} else {
			d = Clock().Sub(t)
		}
	}

	return d, nil
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

	return r.HeuristicFreshness()
}

func (r *Resource) HeuristicFreshness() (time.Duration, error) {
	return time.Duration(0), nil
}

func (r *Resource) IsCacheable(shared bool) bool {
	cc, err := r.cacheControl()
	if err != nil {
		log.Println("Error parsing cache-control: ", err)
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

	return true
}

func (r *Resource) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	age, err := r.Age()
	if err != nil {
		http.Error(w, "Error calculating age: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading resource: "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	r.Body.Close()
	http.ServeContent(w, req, "", r.LastModified(), bytes.NewReader(b))
}

func NewResourceWriter(rw http.ResponseWriter) *responseWriter {
	r, w := io.Pipe()

	return &responseWriter{
		ResponseWriter: rw,
		Resource:       make(chan *Resource),
		pr:             r,
		pw:             w,
	}
}

type responseWriter struct {
	http.ResponseWriter
	Resource chan *Resource
	pr       io.ReadCloser
	pw       io.WriteCloser
}

func (rw *responseWriter) WriteHeader(status int) {
	res := NewResource()
	res.Header = rw.Header()
	res.Body = rw.pr
	res.StatusCode = status

	if clenHeader := res.Header.Get("Content-Length"); clenHeader != "" {
		clenInt, err := strconv.ParseInt(clenHeader, 10, 64)
		if err == nil {
			res.ContentLength = clenInt
		}
	}

	rw.Resource <- res
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if n, err := rw.pw.Write(b); err != nil {
		return n, err
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Close() error {
	return rw.pw.Close()
}
