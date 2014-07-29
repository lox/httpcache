package httpcache

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"

	"strconv"
	"strings"
	"testing"
	"time"
)

var testTime time.Time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
var dumpRequests = false

func init() {
	if os.Getenv("DUMP_REQUESTS") != "" {
		dumpRequests = true
	}
}

func TestUpstreamHeadersCopied(t *testing.T) {
	requests := []testRequest{
		testRequest{
			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Copied-Header", "Llamas")
				defaultHandler.ServeHTTP(w, r)
			}),
			Request: NewRequest("GET", "http://example.org/test", nil),
			AssertResponse: func(r *httptest.ResponseRecorder) {
				if r.HeaderMap.Get("X-Copied-Header") == "" {
					t.Fatal("Headers not copied from upstream response")
				}
			},
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheMiss(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request:           NewRequest("GET", "http://example.org/test", nil),
			AssertCacheStatus: "MISS",
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheHit(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request:           NewRequest("GET", "http://example.org/test", nil),
			AssertCacheStatus: "MISS",
		},
		testRequest{
			Request:           NewRequest("GET", "http://example.org/test", nil),
			AssertCacheStatus: "HIT",
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheHitWithHead(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request:           NewRequest("GET", "http://example.org/test", nil),
			AssertCacheStatus: "MISS",
		},
		testRequest{
			Request:             NewRequest("HEAD", "http://example.org/test", nil),
			AssertCacheStatus:   "HIT",
			AssertContentLength: defaultHandler.SizeString(),
			AssertResponse: func(r *httptest.ResponseRecorder) {
				if r.Body.String() != "" {
					t.Fatal("HEAD responses should have no body")
				}
			},
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheAge(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", nil),
			Time:    testTime,
		},
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", nil),
			Time:    testTime.AddDate(0, 0, 1),
			AssertResponse: func(r *httptest.ResponseRecorder) {
				age := r.HeaderMap.Get("Age")
				if age == "" {
					t.Fatal("Expected Age header")
				}

				if ageInt, err := strconv.Atoi(age); err != nil {
					t.Fatal(err)
				} else if expect := 86400; ageInt != expect {
					t.Fatalf("Age, expected %d, got %d", expect, ageInt)
				}
			},
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheControlUpstreamNoStore(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", nil),
			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store, no-cache")
				defaultHandler.ServeHTTP(w, r)
			}),
			AssertCacheStatus: "SKIP",
		},
		testRequest{
			Request:           NewRequest("GET", "http://example.org/test", nil),
			AssertCacheStatus: "MISS",
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheControlRequestNoStore(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", http.Header{
				"Cache-Control": []string{"no-cache"},
			}),
			AssertCacheStatus: "SKIP",
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheNegotiation(t *testing.T) {
	requests := []testRequest{
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", http.Header{}),
			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Last-Modified", testTime.Format(http.TimeFormat))
				w.Header().Set("ETag", "llamas-rock")
				defaultHandler.ServeHTTP(w, r)
			}),
		},
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", http.Header{
				"If-Modified-Since": []string{testTime.Format(http.TimeFormat)},
			}),
			AssertCode: http.StatusNotModified,
			AssertResponse: func(r *httptest.ResponseRecorder) {
				if r.Body.String() != "" {
					t.Fatal("Response with 304 Not Modified should have no body")
				}
				if expect, got := r.HeaderMap.Get("Etag"), "llamas-rock"; got != expect {
					t.Fatalf("Expected etag %q, got %q", expect, got)
				}
			},
		},
		testRequest{
			Request: NewRequest("GET", "http://example.org/test", http.Header{
				"If-None-Match": []string{"llamas-rock"},
			}),
			AssertCode: http.StatusNotModified,
			AssertResponse: func(r *httptest.ResponseRecorder) {
				if r.Body.String() != "" {
					t.Fatal("Response with 304 Not Modified should have no body")
				}
			},
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

func TestHopByHopHeadersNotSentUpstream(t *testing.T) {
	requests := []testRequest{
		testRequest{
			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defaultHandler.ServeHTTP(w, r)
			}),
			Request: NewRequest("GET", "http://example.org/test", http.Header{
				"Connection": []string{"close"},
			}),
		},
	}

	if err := runRequests(requests, NewPrivateCache()); err != nil {
		t.Fatal(err)
	}
}

// func TestCachingNotModifiedResponses(t *testing.T) {
// 	requests := []testRequest{
// 		testRequest{
// 			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				if r.Header.Get("If-None-Match") != "llamas-rock" {
// 					t.Fatal("If-None-Match headers not forwarded upstream")
// 				}
// 				w.WriteHeader(http.StatusNotModified)
// 			}),
// 			Request: NewRequest("GET", "http://example.org/test", http.Header{
// 				"If-None-Match": []string{"llamas-rock"},
// 			}),
// 			AssertCacheStatus: "MISS",
// 			AssertCode:        http.StatusNotModified,
// 		},
// 		testRequest{
// 			UpstreamHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				t.Fatal("Shouldn't hit upstream for cached requests")
// 			}),
// 			Request: NewRequest("GET", "http://example.org/test", http.Header{
// 				"If-None-Match": []string{"llamas-rock"},
// 			}),
// 			AssertCacheStatus: "HIT",
// 		},
// 	}

// 	if err := runRequests(requests, NewPrivateCache()); err != nil {
// 		t.Fatal(err)
// 	}
// }

func NewRequest(method string, url string, h http.Header) *http.Request {
	r, err := http.NewRequest(method, url, strings.NewReader(""))
	if err != nil {
		panic(err)
	}

	r.Header = h
	return r
}

type testHandler struct {
	body        string
	timeUpdated time.Time
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Date", testTime.Format(http.TimeFormat))
	b := []byte(h.body)
	http.ServeContent(w, r, "", h.timeUpdated, bytes.NewReader(b))
}

func (h *testHandler) Size() int {
	return len(h.body)
}

func (h *testHandler) SizeString() string {
	return strconv.Itoa(h.Size())
}

var defaultHandler *testHandler = &testHandler{
	body: "default handler content",
}

func debugHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dumpRequests {
			b, err := httputil.DumpRequest(r, true)
			if err != nil {
				panic(err)
			}

			log.Printf("DEBUG ----> Upstream Request ----> \n%s", b)
		}

		h.ServeHTTP(w, r)
	})
}

type testRequest struct {
	// request / response handling
	Request         *http.Request
	UpstreamHandler http.Handler

	// assertions on the response
	AssertResponse      func(*httptest.ResponseRecorder)
	AssertContent       string
	AssertContentLength string
	AssertCacheStatus   string
	AssertCode          int

	// Time is the time used for age calculations
	Time time.Time
}

func (t *testRequest) applyDefaults() *testRequest {
	if t.AssertCode == 0 {
		t.AssertCode = http.StatusOK
	}
	if t.UpstreamHandler == nil {
		t.UpstreamHandler = debugHandler(defaultHandler)
	} else {
		t.UpstreamHandler = debugHandler(t.UpstreamHandler)
	}
	return t
}

func (t *testRequest) checkAssertions(r *httptest.ResponseRecorder) error {
	if got, want := r.Code, t.AssertCode; got != want {
		return fmt.Errorf("Response code = %d, want %d", got, want)
	}

	cacheStatus := r.HeaderMap.Get(CacheHeader)
	if t.AssertCacheStatus != "" {
		if got, want := cacheStatus, t.AssertCacheStatus; got != want {
			return fmt.Errorf("Cache status = %s want %s", got, want)
		}
	}

	contentLength := r.HeaderMap.Get("Content-Length")
	if t.AssertContentLength != "" {
		if got, want := contentLength, t.AssertContentLength; got != want {
			return fmt.Errorf("Content-Length = %s want %s", got, want)
		}
	}

	if t.AssertResponse != nil {
		t.AssertResponse(r)
	}

	return nil
}

func (t *testRequest) run(c *Cache) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler := NewHandler(c, t.UpstreamHandler)

	if !t.Time.IsZero() {
		handler.(*CacheHandler).NowFunc = func() time.Time {
			return t.Time
		}
	}

	if dumpRequests {
		b, err := httputil.DumpRequest(t.Request, true)
		if err != nil {
			panic(err)
		}

		log.Printf("DEBUG Request ----> \n%s", b)
	}

	handler.ServeHTTP(recorder, t.Request)
	WaitForWrites()

	if dumpRequests {
		buf := &bytes.Buffer{}
		buf.WriteString("HTTP/1.1 " + http.StatusText(recorder.Code) + "\n")
		recorder.HeaderMap.Write(buf)
		buf.WriteString("\n")
		buf.Write(recorder.Body.Bytes())

		log.Printf("DEBUG Response <---- \n%s", buf.String())
	}

	return recorder
}

func runRequests(reqs []testRequest, c *Cache) error {
	for _, req := range reqs {
		if err := req.checkAssertions(req.applyDefaults().run(c)); err != nil {
			return err
		}
	}
	return nil
}
