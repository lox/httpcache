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
		h.cacheSkip(rw, r)
		rw.Header().Set(CacheDebugHeader, reason)
		return
	}

	key := Key(r.Method, r.URL)

	if res, ok := h.Cache.Get(key); ok {
		// log.Printf("cache hit on %s", key)
		h.cacheHit(res, rw, r)
		return
	}

	// log.Printf("cache miss on %s", key)

	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" {
		condKey := SecondaryKey(key, http.Header{
			"If-None-Match": []string{ifNoneMatch},
		})

		if res, ok := h.Cache.Get(condKey); ok {
			// log.Printf("cache hit on %s", condKey)
			h.cacheHit(res, rw, r)
			return
		}

		// log.Printf("cache miss on %s", condKey)
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
		log.Println("Error calculating age", err)
	} else {
		rw.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))
	}

	if fresh, err := h.Cache.IsFresh(res, h.NowFunc()); err != nil {
		log.Println("Error calculating freshness", err)
	} else if !fresh {
		rw.Header().Add("Warning", fmt.Sprintf(
			"110 - %q %q", "Response is Stale", res.Header.Get("Date"),
		))
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

	if storeable {
		writewg.Add(1)
		go h.store(r, resource)
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

func (h *CacheHandler) store(req *http.Request, res *Resource) {
	defer writewg.Done()

	key := RequestKey(req)

	// store with a conditional key
	if ifNoneMatch := req.Header.Get("If-None-Match"); ifNoneMatch != "" {
		condKey := SecondaryKey(key, http.Header{
			"If-None-Match": []string{ifNoneMatch},
		})

		// log.Printf("storing %s => %#v", condKey, res)
		h.Cache.Set(condKey, res)
		return
	}

	// log.Printf("storing %s => %#v", key, res)
	h.Cache.Set(key, res)
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
	return &Resource{
		URL:        crw.req.URL,
		StatusCode: crw.code,
		Header:     crw.ResponseWriter.Header(),
		Body:       bytes.NewReader(crw.recorder.Body.Bytes()),
		Method:     crw.req.Method,
	}
}
