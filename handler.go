package httpcache

import (
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
)

var ErrNotFound = errors.New("Not found")
var Writes sync.WaitGroup

type Handler struct {
	Shared   bool
	upstream http.Handler
	store    Store
}

func NewHandler(store Store, upstream http.Handler) *Handler {
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

	// Otherwise, Cache hit
	rw.Header().Set(CacheHeader, "HIT")

	// Sometimes we'll need to validate the resource
	if req.mustValidate() || res.MustValidate() || !h.isFresh(req, res) {
		log.Printf("validating cached response")

		if !h.validate(req, res) {
			log.Printf("response is changed")
			rw.Header().Set(CacheHeader, "MISS")
			h.serveUpstream(rw, r)
			return
		} else {
			log.Printf("response is valid, freshening cache")
			h.freshen(r, res)
		}
	}

	log.Printf("response is fresh")
	res.ServeHTTP(rw, r)
}

func (h *Handler) serveUpstream(rw http.ResponseWriter, r *http.Request) {
	log.Printf("serving upstream")
	Writes.Add(1)
	resRw := NewResourceWriter(rw)
	defer resRw.Close()

	go func() {
		res := <-resRw.Resource
		h.invalidate(r, res)
		if res.IsCacheable(h.Shared) {
			log.Println("cacheable, storing")
			h.storeResource(r, res)
		} else {
			log.Println("not cacheable")
		}
		Writes.Done()
	}()

	rw.Header().Set(CacheHeader, "MISS")
	h.upstream.ServeHTTP(resRw, r)
	return
}

func (h *Handler) storeResource(r *http.Request, res *Resource) {
	h.store.Set(RequestKey(r), res)

	// Secondary store for vary
	if vary := res.Header.Get("Vary"); vary != "" {
		h.store.Set(VaryKey(vary, r), res)
	}
}

func (h *Handler) validate(r *request, res *Resource) (valid bool) {
	req := r.cloneRequest()

	if etag := res.Header.Get("Etag"); etag != "" {
		req.Header.Set("If-None-Match", etag)
	} else if lastMod := res.Header.Get("Last-Modified"); lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	log.Printf("%#v", res.Header)

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

func (h *Handler) freshen(r *http.Request, res *Resource) {
	Writes.Add(1)

	go func() {
		h.invalidate(r, res)
		if res.IsCacheable(h.Shared) {
			h.storeResource(r, res)
		}
		Writes.Done()
	}()
}

// lookupResource finds the best matching Resource for the
// request, or nil and false if none is found
func (h *Handler) lookup(r *request) (*Resource, error) {
	var res *Resource
	var found bool

	if res, found, _ = h.store.Get(Key(r.Method, r.URL)); !found {
		if r.Method == "HEAD" {
			res, found, _ = h.store.Get(Key("GET", r.URL))
		}
	}

	if !found {
		return nil, ErrNotFound
	}

	// Secondary lookup for vary
	if vary := res.Header.Get("Vary"); vary != "" {
		res, found, _ = h.store.Get(VaryKey(vary, r.Request))
	}

	if !found {
		return nil, ErrNotFound
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

func (h *Handler) invalidate(r *http.Request, res *Resource) {
	h.store.Delete(Key("HEAD", r.URL))
	h.store.Delete(Key("GET", r.URL))
}

type request struct {
	*http.Request
	cc CacheControl
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
