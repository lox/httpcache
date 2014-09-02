package httpcache

import (
	"bytes"
	"flag"
	"fmt"
	"io"
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

var (
	dumpHttp bool
)

func init() {
	flag.BoolVar(&dumpHttp, "dumphttp", false, "dumps http requests and responses")
}

type responseLogger struct {
	http.ResponseWriter
	status int
	size   int
	t      time.Time
}

func (l *responseLogger) Header() http.Header {
	return l.ResponseWriter.Header()
}

func (l *responseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	if l.status < 200 || l.status >= 300 {
		os.Stderr.Write(b)
	}
	size, err := l.ResponseWriter.Write(b)
	l.size += size
	return size, err
}

func (l *responseLogger) WriteHeader(s int) {
	l.ResponseWriter.WriteHeader(s)
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
	if h.Dump || dumpHttp {
		b, _ := httputil.DumpRequest(r, false)
		writePrefixString(strings.TrimSpace(string(b)), ">> ", os.Stderr)
	}

	logger := &responseLogger{ResponseWriter: w, t: time.Now()}
	h.Handler.ServeHTTP(logger, r)

	if h.Dump || dumpHttp {
		buf := &bytes.Buffer{}
		buf.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n",
			logger.status, http.StatusText(logger.status),
		))
		logger.ResponseWriter.Header().Write(buf)
		writePrefixString(strings.TrimSpace(buf.String()), "<< ", os.Stderr)
	}

	h.writeLog(r, logger)
}

func (h *Logger) writeLog(r *http.Request, logger *responseLogger) {
	cacheStatus := logger.ResponseWriter.Header().Get(CacheHeader)

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

func writePrefixString(s, prefix string, w io.Writer) {
	w.Write([]byte("\n"))
	for _, line := range strings.Split(s, "\r\n") {
		w.Write([]byte(prefix))
		w.Write([]byte(line))
		w.Write([]byte("\n"))
	}
	w.Write([]byte("\n"))
}
