package httpcache

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/lox/httpcache/store"
)

var ErrNotFound = errors.New("Not found")
var Writes sync.WaitGroup

type Handler struct {
	Shared   bool
	upstream http.Handler
	store    store.Store
}

func NewHandler(store store.Store, upstream http.Handler) *Handler {
	return &Handler{
		upstream: upstream,
		store:    store,
		Shared:   false,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req := &request{Request: r}

	// Preconditions
	if bad, reason := req.isBadRequest(); bad {
		http.Error(rw, "Invalid request: "+reason,
			http.StatusBadRequest)
		return
	}

	if !req.isCacheable() {
		log.Printf("request not cacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		h.upstream.ServeHTTP(rw, r)
		h.invalidate(r)
		return
	}

	res, err := h.lookup(req)

	// Cache error
	if err != nil && err != ErrNotFound {
		http.Error(rw, "Lookup error: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	// Cache miss
	if err == ErrNotFound {
		h.serveUpstream(rw, r)
		return
	}

	// Sometimes we'll need to validate the resource
	if req.mustValidate() || res.MustValidate() || !h.isFresh(req, res) {
		log.Printf("validating cached response")

		if !h.validate(req, res) {
			log.Printf("response is changed")
			h.serveUpstream(rw, r)
			rw.Header().Set(CacheHeader, "MISS")
			return
		} else {
			log.Printf("response is valid")
		}
	}

	log.Printf("response is fresh")
	res.ServeHTTP(rw, r)

	// Otherwise, Cache hit
	rw.Header().Set(CacheHeader, "HIT")
}

func (h *Handler) serveUpstream(w http.ResponseWriter, r *http.Request) {
	h.invalidate(r)
	rw := newResponseWriter(w)

	log.Printf("serving upstream")
	h.upstream.ServeHTTP(rw, r)
	res := rw.Resource()

	if res.IsCacheable(h.Shared) {
		ttl, _ := res.MaxAge(h.Shared)
		log.Printf("cacheable, storing for %s", ttl.String())
		h.storeResource(r, res)
		rw.Header().Set(CacheHeader, "MISS")
	} else {
		log.Println("not cacheable")
		rw.Header().Set(CacheHeader, "SKIP")
	}
}

func (h *Handler) storeResource(r *http.Request, res *Resource) {
	res.Save(RequestKey(r), h.store)

	// Secondary store for vary
	if vary := res.Header.Get("Vary"); vary != "" {
		err := store.Copy(
			VaryKey(res.Header.Get("Vary"), r),
			RequestKey(r),
			h.store,
		)
		if err != nil {
			log.Println(err)
		}
	}
}

func (h *Handler) validate(r *request, res *Resource) (valid bool) {
	req := r.cloneRequest()

	if etag := res.Header.Get("Etag"); etag != "" {
		req.Header.Set("If-None-Match", etag)
	} else if lastMod := res.Header.Get("Last-Modified"); lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	resp := httptest.NewRecorder()
	h.upstream.ServeHTTP(resp, req)
	resp.Flush()

	checkHeaders := []string{"ETag", "Content-MD5", "Last-Modified", "Content-Length"}

	for _, h := range checkHeaders {
		if value := resp.HeaderMap.Get(h); value != "" {
			if res.Header.Get(h) != value {
				log.Printf("%s changed, %q != %q", h, value, res.Header.Get(h))
				return false
			}
		}
	}

	return true
}

// lookupResource finds the best matching Resource for the
// request, or nil and ErrNotFound if none is found
func (h *Handler) lookup(r *request) (*Resource, error) {
	res, err := LoadResource(Key(r.Method, r.URL), r.Request, h.store)

	// HEAD requests can be served from GET
	if err == ErrNotFound && r.Method == "HEAD" {
		res, err = LoadResource(Key("GET", r.URL), r.Request, h.store)
		if err != nil {
			return res, err
		}
	} else if err != nil {
		return res, err
	}

	// Secondary lookup for Vary
	if vary := res.Header.Get("Vary"); vary != "" {
		res, err = LoadResource(VaryKey(vary, r.Request), r.Request, h.store)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

func (h *Handler) isFresh(r *request, res *Resource) bool {
	maxAge, err := res.MaxAge(h.Shared)
	if err != nil {
		log.Printf("Error calculating max-age: ", err.Error())
		return false
	}

	age, err := res.Age()
	if err != nil {
		log.Printf("Error calculating age: ", err.Error())
		return false
	}

	if maxAge > age {
		log.Printf("max-age is %q, age is %q", maxAge, age)
		return true
	}

	if hFresh := res.HeuristicFreshness(); hFresh > age {
		log.Printf("heuristic freshness of %q", hFresh)
		return true
	}

	return false
}

func (h *Handler) invalidate(r *http.Request) {
	h.store.Delete(Key("HEAD", r.URL))
	h.store.Delete(Key("GET", r.URL))
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

func (rw *responseWriter) Resource() *Resource {
	res := NewResource()
	res.StatusCode = rw.statusCode
	res.Header = rw.Header()
	res.Body = ioutil.NopCloser(bytes.NewReader(rw.buf.Bytes()))
	res.ContentLength = int64(rw.buf.Len())
	return res
}
