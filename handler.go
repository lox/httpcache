package httpcache

import (
	"errors"
	"log"
	"net/http"
	"sync"
)

var ErrNotFound = errors.New("Not found")
var Writes sync.WaitGroup

type Handler struct {
	Shared    bool
	Transport http.RoundTripper
	upstream  http.Handler
	store     Store
}

func NewHandler(store Store, upstream http.Handler) *Handler {
	return &Handler{
		upstream:  upstream,
		store:     store,
		Shared:    false,
		Transport: http.DefaultTransport,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	req := &request{Request: r}

	defer func() {
		logRequest(r, rw.Header().Get(CacheHeader))
	}()

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
		Writes.Add(1)
		resRw := NewResourceWriter(rw)
		defer resRw.Close()

		go func() {
			res := <-resRw.Resource
			h.store.Set(RequestKey(r), res)
			Writes.Done()
		}()

		rw.Header().Set(CacheHeader, "MISS")
		h.upstream.ServeHTTP(resRw, r)
		return
	}

	// Otherwise, Cache hit
	rw.Header().Set(CacheHeader, "HIT")

	// Sometimes we'll need to validate the resource
	if req.mustValidate() || res.MustValidate() || !h.isFresh(req, res) {
		logRequest(r, "stale, validating response")

		changed, err := h.validate(req, res)
		if err != nil {
			http.Error(rw, "Error validating response: "+err.Error(),
				http.StatusBadGateway)
			return
		}

		if changed {
			log.Printf("response is changed")
			rw.Header().Set(CacheHeader, "MISS")
		} else {
			log.Printf("response is unchanged")
		}
	} else {
		logRequest(r, "response is fresh")
	}

	res.ServeHTTP(rw, r)

	Writes.Add(1)
	go func() {
		h.invalidateStored(r, res)
		// h.store.Set(RequestKey(r), res)
		Writes.Done()
	}()
}

// lookupResource finds the best matching Resource for the
// request, or nil and false if none is found
func (h *Handler) lookup(r *request) (*Resource, error) {
	res, found, err := h.store.Get(Key(r.Method, r.URL))

	if err != nil {
		return nil, err
	}

	// HEAD requests can be served from GET cache
	if !found && r.Method == "HEAD" {
		res, found, err = h.store.Get(Key("GET", r.URL))
		if err != nil {
			return nil, err
		}
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

	return maxAge > age
}

func (h *Handler) validate(r *request, res *Resource) (changed bool, err error) {
	req := r.Request

	for k, headers := range res.Validators() {
		for _, val := range headers {
			switch k {
			case "Etag":
				req.Header.Set("If-None-Match", val)
				log.Printf("Etag: %s", val)
			case "Last-Modified":
				req.Header.Set("If-Modified-Since", val)
				log.Printf("Last-Modified: %s", val)
			}
		}
	}

	resp, err := h.Transport.RoundTrip(req)
	if err != nil {
		return false, err
	}

	return res.Freshen(resp)
}

func (h *Handler) invalidateStored(r *http.Request, res *Resource) error {
	switch r.Method {
	case "GET":
		h.store.Delete(Key("HEAD", r.URL))
	case "HEAD":
		h.store.Delete(Key("GET", r.URL))
	case "PUT":
		fallthrough
	case "POST":
		h.store.Delete(Key("HEAD", r.URL))
		h.store.Delete(Key("GET", r.URL))
	}
	return nil
}

func (h *Handler) freshenStored(r *http.Request, res *Resource) error {
	return nil
}

func logRequest(r *http.Request, msg string) {
	log.Printf("[%s %s] %s", r.Method, r.URL.String(), msg)
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

	return cc.Has("must-validate")
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
