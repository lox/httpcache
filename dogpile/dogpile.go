package dogpile

import (
	"io"
	"log"
	"net/http"

	"sync"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type KeyFunc func(r *http.Request) string

var DefaultKeyFunc = KeyFunc(func(r *http.Request) string {
	return r.URL.String()
})

type Pool struct {
	upstream http.Handler
	sync.Mutex
	responses map[string]*response
	keyFunc   KeyFunc
}

func New(upstream http.Handler) *Pool {
	return &Pool{
		upstream:  upstream,
		responses: map[string]*response{},
		keyFunc:   DefaultKeyFunc,
	}
}

func (p *Pool) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	key := p.keyFunc(r)

	// find or create a response entry
	p.Lock()
	resp, exists := p.responses[key]
	if !exists {
		log.Printf("no responses found for %q", key)
		resp = newResponse()
		p.responses[key] = resp

		go func() {
			log.Printf("dispatching request to upstream")
			p.upstream.ServeHTTP(resp.UpstreamWriter(), r)
			log.Printf("upstream request done")
		}()
	} else {
		log.Printf("found responses for %q", key)
	}
	p.Unlock()

	// pass headers down stream
	for key, vals := range resp.Header() {
		for _, val := range vals {
			rw.Header().Add(key, val)
		}
	}
	log.Printf("wrote headers downstream: %#v", resp.Header())

	// stream from buffer to downstream
	_, err := io.Copy(rw, resp)
	if err != nil {
		panic(err)
	}

	log.Printf("done copying to downstream")
}

func newResponse() *response {
	return &response{buffered: false}
}

type response struct {
	sync.RWMutex
	buffered bool
}

func (resp *response) UpstreamWriter() http.ResponseWriter {
	return &upstreamWriter{}
}

func (resp *response) DownstreamWriter() http.ResponseWriter {
	return &downstreamWriter{}
}

func (resp *response) Header() http.Header {
	return http.Header{}
}

type upstreamWriter struct {
}

func (uw *upstreamWriter) Header() http.Header {
	return http.Header{}
}

func (uw *upstreamWriter) Write(b []byte) (int, error) {
	log.Printf("upstream write of %q", b)
	log.Printf("%#v", uw)
	return 0, nil
}

func (uw *upstreamWriter) WriteHeader(status int) {
	log.Printf("upstream status %d %s", status, http.StatusText(status))
	// uw.ResponseWriter.WriteHeader(status)
	// resp.Unlock()
}

type downstreamWriter struct {
}

func (dw *downstreamWriter) Header() http.Header {
	return http.Header{}
}

func (dw *downstreamWriter) Write(b []byte) (int, error) {
	log.Printf("downstream write of %q", b)
	log.Printf("%#v", dw)
	return 0, nil
}

func (dw *downstreamWriter) WriteHeader(status int) {
	log.Printf("downstream status %d %s", status, http.StatusText(status))
	// uw.ResponseWriter.WriteHeader(status)
	// resp.Unlock()
}

// func (resp *response) Read(p []byte) (n int, err error) {
// 	log.Printf("trying to read from response")
// 	n, err = resp.f.Read(p)
// 	log.Printf("read %d bytes", n)
// 	return
// }
