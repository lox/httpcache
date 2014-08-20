package httpcache

import (
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

const (
	RequestStartHeader = "X-Request-Start"
	CacheHeader        = "X-Cache"
)

type LoggerTransport struct {
	http.Transport
	Dump bool
}

func (t *LoggerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	timer := time.Now().UTC()

	if t.Dump {
		b, err := httputil.DumpRequest(r, false)
		if err != nil {
			return nil, err
		}
		log.Printf("Request:\n%s", b)
	}

	resp, err := t.Transport.RoundTrip(r)
	if err != nil {
		return resp, err
	}

	if startTime := responseStartTime(resp); startTime.Before(timer) {
		timer = startTime
	} else {
		resp.Header.Set(RequestStartHeader, timer.Format(http.TimeFormat))
	}

	if t.Dump {
		b, err := httputil.DumpResponse(resp, false)
		if err != nil {
			panic(err)
			return nil, err
		}
		log.Printf("Response:\n%s", b)
	}

	t.writeLog(timer, r, resp)
	return resp, nil
}

func (t *LoggerTransport) writeLog(startTime time.Time, r *http.Request, resp *http.Response) {
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
}

func responseStartTime(r *http.Response) time.Time {
	if reqStart := r.Header.Get(RequestStartHeader); reqStart != "" {
		if ts, err := http.ParseTime(reqStart); err == nil {
			return ts
		}
	}

	return time.Now()
}
