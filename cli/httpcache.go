package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/lox/httpcache"
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

	handler := httpcache.NewHandler(httpcache.NewMapStore(), proxy)
	handler.Shared = true

	log.Printf("proxy listening on http://%s", listen)
	log.Fatal(http.ListenAndServe(listen, handler))
}
