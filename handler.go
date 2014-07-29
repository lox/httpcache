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
	CacheHeader = "X-Cache"
)

var writewg sync.WaitGroup

type CacheHandler struct {
	Handler http.Handler
	Cache   *Cache
	NowFunc func() time.Time
	Debug   bool
}

func (h *CacheHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	cacheable, _ := h.IsRequestCacheable(r)
	if !cacheable {
		if h.Debug {
			log.Printf("Request isn't cacheable")
		}
		h.cacheSkip(rw, r)
		return
	}

	key, err := Key(r)
	if err != nil {
		log.Println("Failed to calculate request key: ", err)
		h.cacheSkip(rw, r)
		return
	}

	if h.Cache.Has(key) {
		h.cacheHit(key, rw, r)
	} else {
		h.cacheMiss(key, rw, r)
	}
}

func (h *CacheHandler) IsRequestCacheable(r *http.Request) (bool, error) {
	cc, err := ParseCacheControl(r.Header.Get(CacheControlHeader))
	if err != nil {
		log.Printf("Failed to parse Cache-Control on request: %s", err)
		return false, err
	}

	if cc.NoCache {
		return false, nil
	}

	return true, nil
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

func (h *CacheHandler) cacheHit(k string, rw http.ResponseWriter, r *http.Request) {
	ent, err := h.Cache.Retrieve(k)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}

	for key, headers := range ent.Header {
		for _, value := range headers {
			rw.Header().Set(key, value)
		}
	}

	rw.Header().Set(CacheHeader, "HIT")

	// set an Age header
	if age, err := ent.Age(h.NowFunc()); err != nil {
		log.Printf("Error calculating age: %s", err)
	} else {
		rw.Header().Set("Age", fmt.Sprintf("%.f", age.Seconds()))
	}

	t, _, _ := ent.LastModified()
	http.ServeContent(rw, r, "", t, ent.Body)
}

func (h *CacheHandler) cacheMiss(k string, rw http.ResponseWriter, r *http.Request) {
	crw := &cachedResponseWriter{
		ResponseWriter: rw,
		req:            r,
		recorder:       httptest.NewRecorder(),
	}

	crw.Header().Set(CacheHeader, "MISS")
	h.serveUpstream(crw, r)

	entity := crw.entity()
	storeable, err := h.Cache.IsStoreable(entity)
	if err != nil {
		log.Println(err)
	}

	if storeable {
		writewg.Add(1)
		go h.store(k, crw.entity())
	} else {
		if h.Debug {
			log.Printf("Response isn't storeable")
		}
	}
}

var hopByHopHeaders []string = []string{
	"Connection",
	"Keep-Alive",
	"Public",
	"Proxy-Authenticate",
	"Transfer-Encoding",
	"Upgrade",
}

func (h *CacheHandler) serveUpstream(rw http.ResponseWriter, r *http.Request) {
	for _, exclude := range hopByHopHeaders {
		r.Header.Del(exclude)
	}

	h.Handler.ServeHTTP(rw, r)
}

func (h *CacheHandler) store(k string, ent *Entity) {
	if h.Debug {
		log.Printf("Writing entity to cache key %s", k)
	}

	if err := h.Cache.Store(k, ent); err != nil {
		log.Println(err)
	}

	writewg.Done()
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
}

func (crw *cachedResponseWriter) Write(p []byte) (int, error) {
	crw.recorder.Write(p)
	return crw.ResponseWriter.Write(p)
}

func (crw *cachedResponseWriter) WriteHeader(status int) {
	crw.code = status
	crw.recorder.WriteHeader(status)
	crw.ResponseWriter.WriteHeader(status)
}

func (crw *cachedResponseWriter) entity() *Entity {
	return NewEntity(
		crw.req.Method,
		crw.code,
		crw.ResponseWriter.Header(),
		bytes.NewReader(crw.recorder.Body.Bytes()),
	)
}
