package httpcache

import (
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	RequestStartHeader = "X-Request-Start"
	CacheHeader        = "X-Cache"
)

type LogTransport struct {
	http.RoundTripper
}

func (l *LogTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	startTime := time.Now()
	resp, err := l.RoundTripper.RoundTrip(r)
	if err != err {
		return resp, err
	}

	cacheStatus := resp.Header.Get(CacheHeader)

	if strings.HasPrefix(cacheStatus, "HIT") {
		cacheStatus = "\x1b[32;1mHIT\x1b[0m"
	} else if strings.HasPrefix(cacheStatus, "MISS") {
		cacheStatus = "\x1b[31;1mMISS\x1b[0m"
	} else {
		cacheStatus = "\x1b[33;1mSKIP\x1b[0m"
	}

	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}

	log.Printf(
		"%s \"%s %s %s\" (%s) %d %s %s",
		clientIP,
		r.Method,
		r.URL.String(),
		r.Proto,
		http.StatusText(resp.StatusCode),
		resp.ContentLength,
		cacheStatus,
		time.Now().Sub(startTime).String(),
	)

	return resp, nil
}
