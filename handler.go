package httpcache

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"
)

var Writes sync.WaitGroup

type Handler struct {
	Shared   bool
	upstream http.Handler
	cache    *Cache
	Logger   *log.Logger
}

func NewHandler(cache *Cache, upstream http.Handler) *Handler {
	return &Handler{
		upstream: upstream,
		cache:    cache,
		Shared:   false,
		Logger:   log.New(os.Stderr, "", log.LstdFlags),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req := &request{Request: r}

	// Preconditions
	if bad, reason := req.isBadRequest(); bad {
		http.Error(rw, "invalid request: "+reason,
			http.StatusBadRequest)
		return
	}

	// Request might not be cacheable
	if !req.isCacheable() {
		h.Logger.Printf("request not cacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		h.pipeUpstream(rw, r)
		return
	}

	// Lookup cached resource
	res, err := h.lookup(req)
	if err != nil && err != ErrNotFoundInCache {
		http.Error(rw, "lookup error: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	// Cache miss
	if err == ErrNotFoundInCache {
		h.Logger.Printf("%s %s not in cache", r.Method, r.URL.String())
		h.passUpstream(rw, r)
		return
	} else {
		h.Logger.Printf("%s %s found in cache", r.Method, r.URL.String())
	}

	if !res.IsCacheable(h.Shared) {
		h.Logger.Printf("stored, but not cacheable")
		h.passUpstream(rw, r)
		return
	}

	// Sometimes we'll need to validate the resource
	if req.mustValidate() || res.MustValidate() || !h.isFresh(req, res) {
		h.Logger.Printf("validating cached response")

		if !h.validate(req, res) {
			h.Logger.Printf("response is changed")
			h.passUpstream(rw, r)
			return
		} else {
			h.Logger.Printf("response is valid")
		}
	}

	h.Logger.Printf("response is fresh")
	h.serveResource(res, rw, r)
	rw.Header().Set(CacheHeader, "HIT")

	if err := res.Close(); err != nil {
		log.Println("Error closing resource", err)
	}
}

// pipeUpstream makes the request via the upstream handler, the response is not stored or modified
func (h *Handler) pipeUpstream(w http.ResponseWriter, r *http.Request) {
	h.Logger.Printf("piping request upstream")
	h.upstream.ServeHTTP(w, r)
}

// passUpstream makes the request via the upstream handler and stores the result
func (h *Handler) passUpstream(w http.ResponseWriter, r *http.Request) {
	rw := newResponseWriter(w)

	t := time.Now()
	h.Logger.Printf("passing request upstream")
	h.upstream.ServeHTTP(rw, r)
	res := rw.Resource()

	h.Logger.Printf("upstream responded in %s", time.Now().Sub(t))

	if h.isStoreable(&request{Request: r}, res) {
		ttl, _ := res.MaxAge(h.Shared)
		if ttl == 0 && !res.HasExplicitFreshness() {
			ttl = res.HeuristicFreshness()
		}

		h.Logger.Printf("storing resource, ttl of %s", ttl)
		h.storeResource(r, res)
	}

	if res.IsCacheable(h.Shared) {
		rw.Header().Set(CacheHeader, "MISS")
	} else {
		rw.Header().Set(CacheHeader, "SKIP")
	}
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
	http.ServeContent(w, req, "", res.LastModified(), res)
}

func (h *Handler) storeResource(r *http.Request, res *Resource) {
	Writes.Add(1)

	go func() {
		defer Writes.Done()
		keys := []string{RequestKey(r)}
		headers := res.Header()

		// store a secondary vary version
		if vary := headers.Get("Vary"); vary != "" {
			keys = append(keys, VaryKey(headers.Get("Vary"), r))
		}

		if err := h.cache.Store(res, keys...); err != nil {
			h.Logger.Println("storing resource failed", keys, err)
		}
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

	checkHeaders := []string{"ETag", "Content-MD5", "Last-Modified", "Content-Length"}

	for _, header := range checkHeaders {
		if value := resp.HeaderMap.Get(header); value != "" {
			if resHeaders.Get(header) != value {
				h.Logger.Printf("%s changed, %q != %q", header, value, resHeaders.Get(header))
				return false
			}
		}
	}

	return true
}

// lookupResource finds the best matching Resource for the
// request, or nil and ErrNotFoundInCache if none is found
func (h *Handler) lookup(r *request) (*Resource, error) {
	res, err := h.cache.Retrieve(Key(r.Method, r.URL))

	// HEAD requests can be served from GET
	if err == ErrNotFoundInCache && r.Method == "HEAD" {
		res, err = h.cache.Retrieve(Key("GET", r.URL))
		if err != nil {
			return res, err
		}
	} else if err != nil {
		return res, err
	}

	// Secondary lookup for Vary
	if vary := res.Header().Get("Vary"); vary != "" {
		res, err = h.cache.Retrieve(VaryKey(vary, r.Request))
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

func (h *Handler) isStoreable(r *request, res *Resource) bool {
	cc, err := res.cacheControl()
	if err != nil {
		return false
	}

	if cc.Has("no-store") {
		return true
	}

	if r.Request.Method == "GET" || r.Method == "HEAD" {
		return true
	}

	return false
}

func (h *Handler) isFresh(r *request, res *Resource) bool {
	maxAge, err := res.MaxAge(h.Shared)
	if err != nil {
		h.Logger.Printf("error calculating max-age: %s", err.Error())
		return false
	}

	age, err := res.Age()
	if err != nil {
		h.Logger.Printf("error calculating age: %s", err.Error())
		return false
	}

	if maxAge > age {
		h.Logger.Printf("max-age is %q, age is %q", maxAge, age)
		return true
	}

	if hFresh := res.HeuristicFreshness(); hFresh > age {
		h.Logger.Printf("heuristic freshness of %q", hFresh)
		return true
	}

	return false
}

func (h *Handler) invalidate(r *http.Request) {
	Writes.Add(1)
	go func() {
		defer Writes.Done()
		h.cache.Purge(Key("HEAD", r.URL), Key("GET", r.URL))
	}()
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

	if cc.Has("no-cache") || maxAge == "0" {
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
	// res := NewResourceString(rw.buf.Bytes())
	// res.StatusCode = rw.statusCode
	// res.Header = rw.Header()
	// res.Body = ioutil.NopCloser(bytes.NewReader(rw.buf.Bytes()))
	// res.ContentLength = int64(rw.buf.Len())
	// return res
}
