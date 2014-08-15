package httpcache_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func newRequest(method, url string, h ...string) *http.Request {
	req, err := http.NewRequest(method, url, strings.NewReader(""))
	if err != nil {
		panic(err)
	}
	req.Header = parseHeaders(h)
	return req
}

func newResponse(status int, body string, h ...string) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: int64(len(body)),
		Body:          ioutil.NopCloser(strings.NewReader(body)),
		Header:        parseHeaders(h),
	}
}

func parseHeaders(input []string) http.Header {
	headers := http.Header{}
	for _, header := range input {
		if idx := strings.Index(header, ": "); idx != -1 {
			headers.Add(header[0:idx], strings.TrimSpace(header[idx+1:]))
		}
	}
	return headers
}

type mockTransport struct {
	resp *http.Response
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, nil
}
