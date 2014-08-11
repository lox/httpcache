package httpcache_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/lox/httpcache"
	"github.com/stretchr/testify/assert"
)

var (
	testTime = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
)

func init() {
	httpcache.Now = func() time.Time {
		return testTime
	}
}

func TestUpstreamHeadersCopied(t *testing.T) {
	upstream := NewUpstream(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Copied-Header", "Llamas")
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader("content"))
	})

	ht := NewHandlerTest(t, upstream)
	resp := ht.Request("GET", "http://example.org/test", nil)

	assert.Equal(t, "Llamas", resp.Header.Get("X-Copied-Header"))
}

func TestCacheStatus(t *testing.T) {
	ht := NewHandlerTest(t, defaultUpstream)

	reqs := []struct {
		method, url, status, msg string
	}{
		{"HEAD", "http://example.org/test", "MISS", ""},
		{"GET", "http://example.org/test", "MISS", ""},
		{"GET", "http://example.org/test", "HIT", ""},
		{"POST", "http://example.org/test", "SKIP", ""},
		{"GET", "http://example.org/test", "MISS", "POST should invalidate cache"},
		{"OPTIONS", "http://example.org/test", "SKIP", ""},
		{"PUT", "http://example.org/test", "SKIP", ""},
		{"XGET", "http://example.org/test", "SKIP", ""},
		{"GET", "http://example.org/test2", "MISS", ""},
		{"HEAD", "http://example.org/test2", "HIT", "HEAD should be cacheable after GET"},
	}

	for i, req := range reqs {
		resp := ht.Request(req.method, req.url, nil)
		assert.Equal(t, req.status, resp.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))
	}
}

func TestCacheAge(t *testing.T) {
	ht := NewHandlerTest(t, defaultUpstream)

	resp1 := ht.Request("GET", "http://example.org/test", nil)
	assert.Equal(t, "MISS", resp1.Header.Get("X-Cache"))

	httpcache.Now = func() time.Time {
		return testTime.Add(time.Hour * 24)
	}
	resp2 := ht.Request("GET", "http://example.org/test", nil)

	assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	assert.Equal(t, "86400", resp2.Header.Get("Age"))
}

func TestResponseCacheControl(t *testing.T) {
	reqs := []struct {
		cacheControl, status1, status2 string
	}{
		{"no-cache", "SKIP", "SKIP"},
		{"no-store, no-cache", "SKIP", "SKIP"},
		{"max-age=60", "MISS", "HIT"},
	}

	for i, req := range reqs {
		upstream := NewUpstream(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", req.cacheControl)
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("content"))
		})

		ht := NewHandlerTest(t, upstream)
		resp1 := ht.Request("GET", "http://example.org/test", nil)
		assert.Equal(t, req.status1, resp1.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))

		resp2 := ht.Request("GET", "http://example.org/test", nil)
		assert.Equal(t, req.status2, resp2.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))
	}
}

func TestRequestCacheControl(t *testing.T) {
	reqs := []struct {
		cacheControl, status string
		httpCode             int
	}{
		{"only-if-cached", "MISS", http.StatusGatewayTimeout},
	}

	for i, req := range reqs {
		ht := NewHandlerTest(t, defaultUpstream)
		resp := ht.Request("GET", "http://example.org/test", http.Header{
			"Cache-Control": []string{req.cacheControl},
		})

		assert.Equal(t, req.status, resp.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))
		assert.Equal(t, req.httpCode, resp.StatusCode)
	}
}

func TestConditionalResponses(t *testing.T) {
	etag := "llamas-rock"
	upstream := NewUpstream(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "", testTime, strings.NewReader("llamas"))
	})

	ht := NewHandlerTest(t, upstream)
	resp1 := ht.Request("GET", "http://example.org/test", http.Header{
		"If-Modified-Since": []string{testTime.Format(http.TimeFormat)},
	})
	assert.Equal(t, http.StatusNotModified, resp1.StatusCode)

	resp2 := ht.Request("GET", "http://example.org/test", http.Header{
		"If-None-Match": []string{etag},
	})
	assert.Equal(t, http.StatusNotModified, resp2.StatusCode)
}

func TestHopByHopHeadersNotSentUpstream(t *testing.T) {
	hopByHop := []string{
		"Connection",
	}
	upstream := NewUpstream(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range hopByHop {
			if r.Header.Get(h) != "" {
				t.Fatalf("Hop by hop header sent upstream: %s", h)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	ht := NewHandlerTest(t, upstream)
	resp := ht.Request("GET", "http://example.org/test", http.Header{
		"Connection": []string{"close"},
	})

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestStaleResponses(t *testing.T) {
	var table = []struct {
		clientCacheControl, serverCacheControl string
		hasWarning                             bool
		age                                    time.Duration
		status1, status2                       string
	}{
		{"", "max-age=86400", true, time.Hour * 24, "MISS", "HIT"},
		{"", "max-age=86400", false, time.Hour * 14, "MISS", "HIT"},
		{"", "max-age=86400", false, time.Hour * 1, "MISS", "HIT"},
		{"max-age=30", "max-age=86400", true, time.Hour * 1, "MISS", "HIT"},
	}

	for i, req := range table {
		upstream := NewUpstream(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Date", testTime.Format(http.TimeFormat))
			w.Header().Set("Cache-Control", req.serverCacheControl)
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("content"))
		})

		ht := NewHandlerTest(t, upstream)
		resp1 := ht.Request("GET", "http://example.org/test", http.Header{
			"Cache-Control": []string{req.clientCacheControl},
		})
		assert.Equal(t, req.status1, resp1.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))

		httpcache.Now = func() time.Time {
			return testTime.Add(req.age)
		}

		resp2 := ht.Request("GET", "http://example.org/test", http.Header{
			"Cache-Control": []string{req.clientCacheControl},
		})
		assert.Equal(t, req.status2, resp2.Header.Get("X-Cache"),
			fmt.Sprintf("#%d %+v", i+1, req))

		w := resp2.Header.Get("Warning")
		if !strings.HasPrefix(w, "110 - ") && req.hasWarning {
			t.Fatalf("Expected a Warning 110 header, got %q", w)
		} else if strings.HasPrefix(w, "110 - ") && !req.hasWarning {
			t.Fatalf("Expected no Warning header, got %q", w)
		}
		assert.Equal(t, resp2.Header.Get("Age"),
			fmt.Sprintf("%.f", req.age.Seconds()))
	}
}

func dumpResponse(resp *http.Response) {
	b, _ := httputil.DumpResponse(resp, false)
	log.Printf("%s", b)
}

type Upstream struct {
	http.Handler
}

type HandlerTest struct {
	Handler http.Handler
	Cache   *httpcache.Cache
	t       *testing.T
}

func NewUpstream(h func(w http.ResponseWriter, r *http.Request)) *Upstream {
	return &Upstream{http.HandlerFunc(h)}
}

func NewHandlerTest(t *testing.T, upstream *Upstream) *HandlerTest {
	cache := httpcache.NewPrivateCache()
	handler := httpcache.NewHandler(cache, upstream)

	if testing.Verbose() {
		handler = httpcache.NewLogger(handler)
	}

	return &HandlerTest{handler, cache, t}
}

var defaultUpstream = NewUpstream(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Date", testTime.Format(http.TimeFormat))
	if r.Method == "GET" || r.Method == "HEAD" {
		w.Header().Set("Cache-Control", "max-age=86400")
	}
	http.ServeContent(w, r, "", time.Time{}, strings.NewReader("content"))
})

func (ht *HandlerTest) Request(method, url string, h http.Header) *http.Response {
	r, err := http.NewRequest(method, url, strings.NewReader(""))
	if err != nil {
		panic(err)
	}
	r.RemoteAddr = "testcase.local"
	r.RequestURI = url
	if h != nil {
		r.Header = h
	}
	return ht.RoundTrip(r)
}

func (ht *HandlerTest) RoundTrip(r *http.Request) *http.Response {
	recorder := httptest.NewRecorder()
	ht.Handler.ServeHTTP(recorder, r)
	httpcache.WaitForWrites()

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", recorder.Code, http.StatusText(recorder.Code)),
		StatusCode:    recorder.Code,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: int64(recorder.Body.Len()),
		Body:          ioutil.NopCloser(recorder.Body),
		Header:        recorder.Header(),
	}
}
