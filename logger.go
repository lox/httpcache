package httpcache

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	RequestStartHeader = "X-Request-Start"
)

const (
	TimeFormat = time.RFC3339Nano
)

type logRecord struct {
	http.ResponseWriter
	ip                    string
	time                  time.Time
	method, uri, protocol string
	status                int
	responseBytes         int64
	elapsedTime           time.Duration
	cacheStatus           string
}

func (r *logRecord) Log(logger *log.Logger) {
	cacheStatus := r.cacheStatus

	if strings.HasPrefix(cacheStatus, "HIT") {
		cacheStatus = "\x1b[32;1mHIT\x1b[0m"
	} else if strings.HasPrefix(cacheStatus, "MISS") {
		cacheStatus = "\x1b[31;1mMISS\x1b[0m"
	} else {
		cacheStatus = "\x1b[33;1mSKIP\x1b[0m"
	}

	logger.Printf(
		"%s \"%s %s %s\" (%s) %d %s %s",
		r.ip,
		r.method,
		r.uri,
		r.protocol,
		http.StatusText(r.status),
		r.responseBytes,
		cacheStatus,
		r.elapsedTime,
	)
}

func (r *logRecord) Write(p []byte) (int, error) {
	written, err := r.ResponseWriter.Write(p)
	r.responseBytes += int64(written)
	return written, err
}

func (r *logRecord) WriteHeader(status int) {
	r.status = status
	r.cacheStatus = r.ResponseWriter.Header().Get(CacheHeader)
	r.ResponseWriter.WriteHeader(status)
}

type loggingHandler struct {
	handler http.Handler
	logger  *log.Logger
}

func NewLogger(out io.Writer, h http.Handler) http.Handler {
	return &loggingHandler{
		handler: h,
		logger:  log.New(out, "", log.LstdFlags),
	}
}

func (h *loggingHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}

	record := &logRecord{
		ResponseWriter: rw,
		ip:             clientIP,
		time:           time.Time{},
		method:         r.Method,
		uri:            r.RequestURI,
		protocol:       r.Proto,
		status:         http.StatusOK,
		elapsedTime:    time.Duration(0),
	}

	startTime := time.Now()

	// use the request start time if we can
	if s := r.Header.Get(RequestStartHeader); s != "" {
		sT, err := time.Parse(TimeFormat, s)
		if err != nil {
			log.Println(err)
		} else {
			startTime = sT
		}
	}

	h.handler.ServeHTTP(record, r)
	finishTime := time.Now()

	record.time = finishTime.UTC()
	record.elapsedTime = finishTime.Sub(startTime)
	record.Log(h.logger)
}

func WriteLogHeaders(req *http.Request) {
	req.Header.Set(RequestStartHeader, time.Now().Format(TimeFormat))
}
