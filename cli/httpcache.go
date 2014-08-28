package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/lox/httpcache"
	"github.com/lox/httpcache/store"
)

const (
	defaultListen = "0.0.0.0:8080"
)

var (
	listen string
)

func init() {
	flag.StringVar(&listen, "listen", defaultListen, "the host and port to bind to")
	flag.Parse()
}

func main() {
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
		},
	}

	handler := httpcache.NewHandler(store.NewMapStore(), proxy)
	handler.Shared = true

	logger := &httpcache.Logger{
		Handler: handler,
	}

	log.Printf("proxy listening on http://%s", listen)
	log.Fatal(http.ListenAndServe(listen, logger))
}
