package dogpile

import (
	"bytes"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

type testResponseWriter struct {
	buf    *bytes.Buffer
	h      http.Header
	status int
	delay  time.Duration
}

func (rb *testResponseWriter) WriteHeader(status int) {
	rb.status = status
}

func (rb *testResponseWriter) Header() http.Header {
	return rb.h
}

func (rb *testResponseWriter) Write(b []byte) (int, error) {
	rb.buf.Write(b)
	return len(b), nil
}

type testHandler struct {
	reqCount int
}

func (t *testHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("X-Llamas", "1")
	rw.Header().Set("Content-Length", "11")
	rw.Header().Set("X-Request-Id", strconv.Itoa(t.reqCount))
	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("llamas rock"))
	t.reqCount += 1
}

func TestDogpile(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	upstream := &testHandler{}
	pool := New(upstream)
	wg := sync.WaitGroup{}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rb := &testResponseWriter{buf: &bytes.Buffer{}, h: http.Header{}}
			pool.ServeHTTP(rb, req)
			if rb.buf.String() != "llamas rock" {
				t.Fatalf("Expected response body %q in req #%d, got %q",
					"llamas rock", i+1, rb.buf.String())
			}
		}(i)
	}

	wg.Wait()
	if upstream.reqCount != 1 {
		t.Fatalf("got %d upstream responses, expected 1", upstream.reqCount)
	}
}
