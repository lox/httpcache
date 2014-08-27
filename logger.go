package httpcache

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

const (
	CacheHeader = "X-Cache"
)

type responseLogger struct {
	w      http.ResponseWriter
	status int
	size   int
	t      time.Time
}

func (l *responseLogger) Header() http.Header {
	return l.w.Header()
}

func (l *responseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	if l.status < 200 || l.status >= 300 {
		os.Stderr.Write(b)
	}
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *responseLogger) WriteHeader(s int) {
	l.w.WriteHeader(s)
	l.status = s
}

func (l *responseLogger) Status() int {
	return l.status
}

func (l *responseLogger) Size() int {
	return l.size
}

type Logger struct {
	http.Handler
	Dump bool
}

func (h *Logger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Dump {
		b, _ := httputil.DumpRequest(r, false)
		log.Printf("Request:\n%s", b)
	}

	logger := &responseLogger{w: w, t: time.Now()}
	h.Handler.ServeHTTP(logger, r)

	if h.Dump {
		buf := &bytes.Buffer{}
		buf.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n",
			logger.status, http.StatusText(logger.status),
		))
		logger.w.Header().Write(buf)

		log.Printf("Response:\n%s", buf.String())
	}

	h.writeLog(r, logger)
}

func (h *Logger) writeLog(r *http.Request, logger *responseLogger) {
	cacheStatus := logger.w.Header().Get(CacheHeader)

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
		http.StatusText(logger.status),
		logger.size,
		cacheStatus,
		time.Now().Sub(logger.t).String(),
	)
}
