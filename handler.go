package httpcache

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

const (
	CacheHeader      = "X-Cache"
	CacheDebugHeader = "X-Cache-Debug"
)

var writewg sync.WaitGroup

type CacheHandler struct {
	Handler http.Handler
	Cache   *Cache
	NowFunc func() time.Time
	Debug   bool
}

func NewHandler(c *Cache, h http.Handler) http.Handler {
	return &CacheHandler{
		Handler: h,
		Cache:   c,
		NowFunc: func() time.Time {
			return time.Now()
		},
	}
}

func (h *CacheHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ok, reason, _ := h.Cache.IsRequestCacheable(r)
	if !ok {
		if h.Debug {
			log.Printf("Request isn't cacheable: %s", reason)
		}
		h.cacheSkip(rw, r)
		rw.Header().Set(CacheDebugHeader, reason)
		return
	}

	searchKeys := map[string]string{}

	if key, ok, _ := ConditionalKey(r.Method, r.URL.String(), r.Header); ok {
		searchKeys["conditional"] = key
	}

	if key, err := Key(r.Method, r.URL.String()); serverError(err, rw) {
		return
	} else {
		searchKeys["basic"] = key
	}

	if r.Method == "HEAD" {
		getKey, err := Key("GET", r.URL.String())
		if serverError(err, rw) {
			return
		} else {
			searchKeys["basic_get"] = getKey
		}
	}

	for _, key := range searchKeys {
		if res, ok := h.Cache.Get(key); ok {
			// log.Printf("Cache hit on %s (%s): %#v", key, t, res)
			h.cacheHit(res, rw, r)
			return
		}
	}

	h.cacheMiss(rw, r)
}

func serverError(err error, rw http.ResponseWriter) bool {
	if err != nil {
		log.Println(err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return true
	}

	return false
}

func (h *CacheHandler) handleHead(rw http.ResponseWriter, r *http.Request) bool {
	key, err := Key("GET", r.URL.String())
	if serverError(err, rw) {
		return true
	}

	res, ok := h.Cache.Get(key)
	if ok {
		h.cacheHit(res, rw, r)
		return true
	}

	return false
}

func (h *CacheHandler) cacheSkip(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set(CacheHeader, "SKIP")
	h.serveUpstream(rw, r)
}

func (h *CacheHandler) cacheHit(res *Resource, rw http.ResponseWriter, r *http.Request) {
	for key, headers := range res.Header {
		for _, value := range headers {
			rw.Header().Set(key, value)
		}
	}

	rw.Header().Set(CacheHeader, "HIT")

	if age, err := res.Age(h.NowFunc()); err != nil {
		log.Printf("Error calculating age: %s", err)
	} else {
		rw.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))
	}

	t, _, _ := res.LastModified()
	http.ServeContent(rw, r, "", t, res.Body)
}

func (h *CacheHandler) cacheMiss(rw http.ResponseWriter, r *http.Request) {
	crw := &cachedResponseWriter{
		ResponseWriter: rw,
		req:            r,
		recorder:       httptest.NewRecorder(),
		cache:          h.Cache,
	}

	crw.Header().Set(CacheHeader, "MISS")
	h.serveUpstream(crw, r)

	resource := crw.resource()
	storeable, reason, err := h.Cache.IsStoreable(resource)
	if err != nil {
		log.Println(err)
	}

	var key string

	if condKey, ok, _ := ConditionalKey(r.Method, r.URL.String(), r.Header); ok {
		key = condKey
	} else {
		basicKey, err := Key(r.Method, r.URL.String())
		if err != nil {
			serverError(err, rw)
			return
		}
		key = basicKey
	}

	if storeable {
		writewg.Add(1)
		go h.store(key, resource)
	} else {
		if h.Debug {
			log.Printf("Response isn't cacheable: %s", reason)
		}
	}
}

var hopByHopHeaders []string = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func (h *CacheHandler) serveUpstream(rw http.ResponseWriter, r *http.Request) {
	for _, exclude := range hopByHopHeaders {
		r.Header.Del(exclude)
	}

	h.Handler.ServeHTTP(rw, r)
}

func (h *CacheHandler) store(key string, res *Resource) {
	if key == "" {
		panic("Refusing to store an empty key")
	}

	// log.Printf("storing %s => %#v", key, res)
	if err := h.Cache.Set(key, res); err != nil {
		log.Println(err)
	}

	writewg.Done()
}

func WaitForWrites() {
	writewg.Wait()
}

// responseWriter writes a response to the cache and also
// to a delegate http.ResponseWriter
type cachedResponseWriter struct {
	http.ResponseWriter
	req      *http.Request
	recorder *httptest.ResponseRecorder
	code     int
	cache    *Cache
}

func (crw *cachedResponseWriter) Write(p []byte) (int, error) {
	crw.recorder.Write(p)
	return crw.ResponseWriter.Write(p)
}

func (crw *cachedResponseWriter) WriteHeader(status int) {
	crw.code = status

	if ok, reason, _ := crw.cache.IsCacheable(crw.resource()); !ok {
		crw.ResponseWriter.Header().Set(CacheHeader, "SKIP")
		crw.ResponseWriter.Header().Set(CacheDebugHeader, reason)

	}

	crw.recorder.WriteHeader(status)
	crw.ResponseWriter.WriteHeader(status)
}

func (crw *cachedResponseWriter) resource() *Resource {
	return NewResource(
		crw.req.Method,
		crw.code,
		crw.ResponseWriter.Header(),
		bytes.NewReader(crw.recorder.Body.Bytes()),
	)
}
