package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

const (
	CacheHeader = "X-Cache"
)

var Writes sync.WaitGroup

type Handler struct {
	Shared   bool
	upstream http.Handler
	cache    *Cache
}

func NewHandler(cache *Cache, upstream http.Handler) *Handler {
	return &Handler{
		upstream: upstream,
		cache:    cache,
		Shared:   false,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req := &request{Request: r}

	if bad, reason := req.isBadRequest(); bad {
		http.Error(rw, "invalid request: "+reason,
			http.StatusBadRequest)
		return
	}

	if !req.isCacheable() {
		Debugf("request not cacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		h.pipeUpstream(rw, r)
		return
	}

	res, err := h.lookup(req)
	if err != nil && err != ErrNotFoundInCache {
		http.Error(rw, "lookup error: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	if err == ErrNotFoundInCache {
		Debugf("%s %s not in cache", r.Method, r.URL.String())
		h.passUpstream(rw, r)
		return
	} else {
		Debugf("%s %s found in cache", r.Method, r.URL.String())
	}

	if h.needsValidation(res, req) {
		Debugf("validating cached response")

		if !h.validate(req, res) {
			Debugf("response is changed")
			h.passUpstream(rw, r)
			return
		} else {
			Debugf("response is valid")
			h.cache.Freshen(res, NewRequestKey(r).String())
		}
	}

	h.serveResource(res, rw, r)
	rw.Header().Set(CacheHeader, "HIT")

	if err := res.Close(); err != nil {
		Errorf("Error closing resource: %s", err.Error())
	}
}

func (h *Handler) needsValidation(res *Resource, req *request) bool {
	if req.mustValidate() || res.MustValidate() {
		return true
	}

	if res.IsStale() {
		return true
	}

	maxAge, err := res.MaxAge(h.Shared)
	if err != nil {
		Debugf("error calculating max-age: %s", err.Error())
		return true
	}

	age, err := res.Age()
	if err != nil {
		Debugf("error calculating age: %s", err.Error())
		return true
	}

	if hFresh := res.HeuristicFreshness(); hFresh > age {
		Debugf("heuristic freshness of %q", hFresh)
		return false
	}

	if age > maxAge {
		Debugf("age %q > max-age %q", age, maxAge)
		maxStale, _ := req.maxStale()
		if maxStale > (age - maxAge) {
			log.Printf("stale, but within allowed max-stale period of %s", maxStale)
			return false
		}
		return true
	}

	return false
}

// pipeUpstream makes the request via the upstream handler, the response is not stored or modified
func (h *Handler) pipeUpstream(w http.ResponseWriter, r *http.Request) {
	rw := newResponseWriter(w)

	Debugf("piping request upstream")
	h.upstream.ServeHTTP(rw, r)

	if r.Method == "HEAD" {
		res := rw.Resource()
		h.cache.Freshen(res, NewRequestKey(r).ForMethod("GET").String())
	}
}

// passUpstream makes the request via the upstream handler and stores the result
func (h *Handler) passUpstream(w http.ResponseWriter, r *http.Request) {
	rw := newResponseWriter(w)

	t := time.Now()
	Debugf("passing request upstream")
	h.upstream.ServeHTTP(rw, r)
	res := rw.Resource()

	Debugf("upstream responded in %s", time.Now().Sub(t))

	if !h.isCacheable(r, res) {
		Debugf("resource is uncacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		return
	}

	rw.Header().Set(CacheHeader, "MISS")
	h.storeResource(r, res)
}

func isStatusCacheableByDefault(status int) bool {
	allowed := []int{
		http.StatusOK,
		http.StatusFound,
		http.StatusNotModified,
		http.StatusNonAuthoritativeInfo,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusGone,
		http.StatusPartialContent,
	}

	for _, a := range allowed {
		if a == status {
			return true
		}
	}

	return false
}

func (h *Handler) isCacheable(r *http.Request, res *Resource) bool {
	cc, err := res.cacheControl()
	if err != nil {
		Errorf("Error parsing cache-control: %s", err.Error())
		return false
	}

	if cc.Has("no-cache") || cc.Has("no-store") || (cc.Has("private") && h.Shared) {
		return false
	}

	if r.Header.Get("Authorization") != "" && h.Shared {
		return false
	}

	if res.HasExplicitExpiration() {
		return true
	}

	if isStatusCacheableByDefault(res.Status()) {
		if cc.Has("public") {
			return true
		} else if res.HasValidators() {
			return true
		} else if res.HeuristicFreshness() > time.Duration(0) {
			return true
		}
	}

	return false
}

func (h *Handler) serveResource(res *Resource, w http.ResponseWriter, req *http.Request) {
	for key, headers := range res.Header() {
		for _, header := range headers {
			w.Header().Add(key, header)
		}
	}

	age, err := res.Age()
	if err != nil {
		http.Error(w, "Error calculating age: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	warnings, _ := res.Warnings()
	for _, warn := range warnings {
		w.Header().Add("Warning", warn)
	}

	w.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))

	// hacky handler for non-ok statuses
	if res.Status() != http.StatusOK {
		w.WriteHeader(res.Status())
		io.Copy(w, res)
	} else {
		http.ServeContent(w, req, "", res.LastModified(), res)
	}
}

func (h *Handler) storeResource(r *http.Request, res *Resource) {
	Writes.Add(1)

	go func() {
		defer Writes.Done()
		t := time.Now()
		keys := []string{NewRequestKey(r).String()}
		headers := res.Header()

		// store a secondary vary version
		if vary := headers.Get("Vary"); vary != "" {
			keys = append(keys, NewRequestKey(r).Vary(vary, r).String())
		}

		if err := h.cache.Store(res, keys...); err != nil {
			Errorf("storing resource %#v failed: %s", err.Error(), keys)
		}

		Debugf("stored resources %+v in %s", keys, time.Now().Sub(t))
	}()
}

func (h *Handler) validate(r *request, res *Resource) (valid bool) {
	req := r.cloneRequest()
	resHeaders := res.Header()

	if etag := resHeaders.Get("Etag"); etag != "" {
		req.Header.Set("If-None-Match", etag)
	} else if lastMod := resHeaders.Get("Last-Modified"); lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	resp := httptest.NewRecorder()
	h.upstream.ServeHTTP(resp, req)
	resp.Flush()

	return validateHeaders(resHeaders, resp.HeaderMap)
}

var validationHeaders = []string{"ETag", "Content-MD5", "Last-Modified", "Content-Length"}

func validateHeaders(h1, h2 http.Header) bool {
	for _, header := range validationHeaders {
		if value := h2.Get(header); value != "" {
			if h1.Get(header) != value {
				Debugf("%s changed, %q != %q", header, value, h1.Get(header))
				return false
			}
		}
	}

	return true
}

// lookupResource finds the best matching Resource for the
// request, or nil and ErrNotFoundInCache if none is found
func (h *Handler) lookup(req *request) (*Resource, error) {
	key := req.key()
	res, err := h.cache.Retrieve(key.String())

	// HEAD requests can possibly be served from GET
	if err == ErrNotFoundInCache && req.Method == "HEAD" {
		res, err = h.cache.Retrieve(key.ForMethod("GET").String())
		if err != nil {
			return nil, err
		}

		if res.HasExplicitExpiration() && req.isCacheable() {
			Debugf("using cached GET request for serving HEAD")
			return res, nil
		} else {
			return nil, ErrNotFoundInCache
		}
	} else if err != nil {
		return res, err
	}

	// Secondary lookup for Vary
	if vary := res.Header().Get("Vary"); vary != "" {
		res, err = h.cache.Retrieve(key.Vary(vary, req.Request).String())
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

type request struct {
	*http.Request
	cc CacheControl
}

func (r *request) isCacheable() bool {
	if !(r.Method == "GET" || r.Method == "HEAD") {
		return false
	}

	cc, err := r.cacheControl()
	if err != nil {
		return false
	}

	maxAge, _ := cc.Get("max-age")

	if cc.Has("no-store") || cc.Has("no-cache") || maxAge == "0" {
		return false
	}

	return true
}

func (r *request) isBadRequest() (valid bool, reason string) {
	if _, err := r.cacheControl(); err != nil {
		return false, "Failed to parse Cache-Control header"
	}

	if r.Request.Proto == "HTTP/1.1" && r.Request.Host == "" {
		return true, "Host header can't be empty"
	}
	return false, ""
}

func (r *request) mustValidate() bool {
	if r.Request.Header.Get("If-Modified-Since") != "" {
		return true
	}

	cc, err := r.cacheControl()
	if err != nil {
		return false
	}

	return cc.Has("no-cache")
}

func (r *request) maxStale() (time.Duration, error) {
	cc, err := r.cacheControl()
	if err != nil {
		return time.Duration(0), err
	}

	if cc.Has("max-stale") {
		return cc.Duration("max-stale")
	}

	return time.Duration(0), nil
}

func (r *request) cacheControl() (CacheControl, error) {
	if r.cc != nil {
		return r.cc, nil
	}

	cc, err := ParseCacheControl(r.Request.Header.Get("Cache-Control"))
	if err != nil {
		return cc, err
	}

	r.cc = cc
	return cc, nil
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func (r *request) cloneRequest() *http.Request {
	r2 := new(http.Request)
	*r2 = *r.Request
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}

func (r *request) key() Key {
	return NewRequestKey(r.Request)
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		buf:            &bytes.Buffer{},
	}
}

type responseWriter struct {
	http.ResponseWriter
	buf        *bytes.Buffer
	statusCode int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.statusCode = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.buf.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Resource returns a copy of the responseWriter as a Resource object
func (rw *responseWriter) Resource() *Resource {
	return NewResourceBytes(rw.statusCode, rw.buf.Bytes(), rw.Header())
}
