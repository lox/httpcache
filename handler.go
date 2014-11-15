package httpcache

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	CacheHeader     = "X-Cache"
	ProxyDateHeader = "Proxy-Date"
)

var Writes sync.WaitGroup

var storeable = map[int]bool{
	http.StatusOK:                   true,
	http.StatusFound:                true,
	http.StatusNonAuthoritativeInfo: true,
	http.StatusMultipleChoices:      true,
	http.StatusMovedPermanently:     true,
	http.StatusGone:                 true,
	http.StatusNotFound:             true,
}

var cacheableByDefault = map[int]bool{
	http.StatusOK:                   true,
	http.StatusFound:                true,
	http.StatusNotModified:          true,
	http.StatusNonAuthoritativeInfo: true,
	http.StatusMultipleChoices:      true,
	http.StatusMovedPermanently:     true,
	http.StatusGone:                 true,
	http.StatusPartialContent:       true,
}

type Handler struct {
	Shared    bool
	upstream  http.Handler
	validator *Validator
	cache     *Cache
}

func NewHandler(cache *Cache, upstream http.Handler) *Handler {
	return &Handler{
		upstream:  upstream,
		cache:     cache,
		validator: &Validator{upstream},
		Shared:    false,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	cReq, err := newCacheRequest(r)
	if err != nil {
		http.Error(rw, "invalid request: "+err.Error(),
			http.StatusBadRequest)
		return
	}

	if !cReq.isCacheable() {
		Debugf("request not cacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		h.pipeUpstream(rw, cReq)
		return
	}

	res, err := h.lookup(cReq)
	if err != nil && err != ErrNotFoundInCache {
		http.Error(rw, "lookup error: "+err.Error(),
			http.StatusInternalServerError)
		return
	}

	if err == ErrNotFoundInCache {
		if cReq.CacheControl.Has("only-if-cached") {
			http.Error(rw, "key not in cache",
				http.StatusGatewayTimeout)
			return
		}
		Debugf("%s %s not in cache", r.Method, r.URL.String())
		h.passUpstream(rw, cReq)
		return
	} else {
		Debugf("%s %s found in cache", r.Method, r.URL.String())
	}

	if h.needsValidation(res, cReq) {
		if cReq.CacheControl.Has("only-if-cached") {
			http.Error(rw, "key was in cache, but required validation",
				http.StatusGatewayTimeout)
			return
		}

		Debugf("validating cached response")
		if h.validator.Validate(r, res) {
			Debugf("response is valid")
			h.cache.Freshen(res, cReq.Key.String())
		} else {
			Debugf("response is changed")
			h.passUpstream(rw, cReq)
			return
		}
	}

	h.serveResource(res, rw, cReq)
	rw.Header().Set(CacheHeader, "HIT")

	if err := res.Close(); err != nil {
		Errorf("Error closing resource: %s", err.Error())
	}
}

func (h *Handler) needsValidation(res *Resource, r *cacheRequest) bool {
	if res.MustValidate(h.Shared) {
		return true
	}

	if res.IsStale() {
		return true
	}

	maxAge, err := res.MaxAge(h.Shared)
	if err != nil {
		Debugf("error parsing max-age: %s", err.Error())
		return true
	}

	if r.CacheControl.Has("max-age") {
		reqMaxAge, err := r.CacheControl.Duration("max-age")
		if err != nil {
			Debugf("error parsing request max-age: %s", err.Error())
			return true
		}

		if reqMaxAge < maxAge {
			Debugf("using request max-age of %s", reqMaxAge.String())
			maxAge = reqMaxAge
		}
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

	if r.CacheControl.Has("min-fresh") {
		reqMinFresh, err := r.CacheControl.Duration("min-fresh")
		if err != nil {
			Debugf("error parsing request min-fresh: %s", err.Error())
			return true
		}

		if (age + reqMinFresh) > maxAge {
			log.Printf("fresh, but won't satisfy min-fresh of %s", reqMinFresh)
			return true
		}
	}

	if age > maxAge {
		Debugf("age %q > max-age %q", age, maxAge)
		maxStale, _ := r.CacheControl.Duration("max-stale")
		if maxStale > (age - maxAge) {
			log.Printf("stale, but within allowed max-stale period of %s", maxStale)
			return false
		}
		return true
	}

	return false
}

// pipeUpstream makes the request via the upstream handler, the response is not stored or modified
func (h *Handler) pipeUpstream(w http.ResponseWriter, r *cacheRequest) {
	rw := newResponseWriter(w)

	Debugf("piping request upstream")
	h.upstream.ServeHTTP(rw, r.Request)

	if r.Method == "HEAD" {
		res := rw.Resource()
		h.cache.Freshen(res, r.Key.ForMethod("GET").String())
	}
}

// passUpstream makes the request via the upstream handler and stores the result
func (h *Handler) passUpstream(w http.ResponseWriter, r *cacheRequest) {
	rw := newResponseWriter(w)

	t := Clock()
	Debugf("passing request upstream")
	h.upstream.ServeHTTP(rw, r.Request)
	res := rw.Resource()
	Debugf("upstream responded in %s", Clock().Sub(t).String())

	if !h.isCacheable(res, r) {
		Debugf("resource is uncacheable")
		rw.Header().Set(CacheHeader, "SKIP")
		return
	}

	if age, err := correctedAge(res.Header(), t, Clock()); err == nil {
		res.Header().Set("Age", strconv.Itoa(int(math.Ceil(age.Seconds()))))
	} else {
		Debugf("error calculating corrected age: %s", err.Error())
	}

	rw.Header().Set(CacheHeader, "MISS")
	rw.Header().Set(ProxyDateHeader, Clock().Format(http.TimeFormat))
	h.storeResource(res, r)
}

// correctedAge adjusts the age of a resource for clock skeq and travel time
// https://httpwg.github.io/specs/rfc7234.html#rfc.section.4.2.3
func correctedAge(h http.Header, reqTime, respTime time.Time) (time.Duration, error) {
	date, err := timeHeader("Date", h)
	if err != nil {
		return time.Duration(0), err
	}

	Debugf("response_time: %d (%s relative to now)\n",
		respTime.Unix(), Clock().Sub(respTime).String())

	apparentAge := respTime.Sub(date)
	if apparentAge < 0 {
		apparentAge = 0
	}
	Debugf("apparent_age: %s\n", apparentAge.String())

	respDelay := respTime.Sub(reqTime)
	ageSeconds, err := intHeader("Age", h)
	age := time.Second * time.Duration(ageSeconds)
	correctedAge := age + respDelay

	Debugf("sent_age: %s\n", age.String())
	Debugf("corrected_age: %s (delay of %s)\n", correctedAge.String(), respDelay.String())

	if apparentAge > correctedAge {
		correctedAge = apparentAge
	}

	residentTime := Clock().Sub(respTime)
	currentAge := correctedAge + residentTime

	return currentAge, nil
}

func (h *Handler) isCacheable(res *Resource, r *cacheRequest) bool {
	cc, err := res.cacheControl()
	if err != nil {
		Errorf("Error parsing cache-control: %s", err.Error())
		return false
	}

	if cc.Has("no-cache") || cc.Has("no-store") || (cc.Has("private") && h.Shared) {
		return false
	}

	if _, ok := storeable[res.Status()]; !ok {
		return false
	}

	if r.Header.Get("Authorization") != "" && h.Shared {
		return false
	}

	if res.HasExplicitExpiration() {
		return true
	}

	if _, ok := cacheableByDefault[res.Status()]; ok {
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

func (h *Handler) serveResource(res *Resource, w http.ResponseWriter, req *cacheRequest) {
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

	Debugf("resource is %s old, updating age from %q",
		age.String(), w.Header().Get("Age"))

	w.Header().Set("Age", fmt.Sprintf("%.f", math.Floor(age.Seconds())))
	w.Header().Set("Via", res.Via())

	// hacky handler for non-ok statuses
	if res.Status() != http.StatusOK {
		w.WriteHeader(res.Status())
		io.Copy(w, res)
	} else {
		http.ServeContent(w, req.Request, "", res.LastModified(), res)
	}
}

func (h *Handler) storeResource(res *Resource, r *cacheRequest) {
	Writes.Add(1)

	go func() {
		defer Writes.Done()
		t := Clock()
		keys := []string{r.Key.String()}
		headers := res.Header()

		// store a secondary vary version
		if vary := headers.Get("Vary"); vary != "" {
			keys = append(keys, r.Key.Vary(vary, r.Request).String())
		}

		if err := h.cache.Store(res, keys...); err != nil {
			Errorf("storing resources %#v failed with error: %s", keys, err.Error())
		}

		Debugf("stored resources %+v in %s", keys, Clock().Sub(t))
	}()
}

// lookupResource finds the best matching Resource for the
// request, or nil and ErrNotFoundInCache if none is found
func (h *Handler) lookup(req *cacheRequest) (*Resource, error) {
	res, err := h.cache.Retrieve(req.Key.String())

	// HEAD requests can possibly be served from GET
	if err == ErrNotFoundInCache && req.Method == "HEAD" {
		res, err = h.cache.Retrieve(req.Key.ForMethod("GET").String())
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
		res, err = h.cache.Retrieve(req.Key.Vary(vary, req.Request).String())
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

type cacheRequest struct {
	*http.Request
	Key          Key
	Time         time.Time
	CacheControl CacheControl
}

func newCacheRequest(r *http.Request) (*cacheRequest, error) {
	cc, err := ParseCacheControl(r.Header.Get("Cache-Control"))
	if err != nil {
		return nil, err
	}

	if r.Proto == "HTTP/1.1" && r.Host == "" {
		return nil, errors.New("Host header can't be empty")
	}

	return &cacheRequest{
		Request:      r,
		Key:          NewRequestKey(r),
		Time:         Clock(),
		CacheControl: cc,
	}, nil
}

func (r *cacheRequest) isCacheable() bool {
	if !(r.Method == "GET" || r.Method == "HEAD") {
		return false
	}

	if maxAge, ok := r.CacheControl.Get("max-age"); ok && maxAge == "0" {
		return false
	}

	if r.CacheControl.Has("no-store") || r.CacheControl.Has("no-cache") {
		return false
	}

	return true
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
