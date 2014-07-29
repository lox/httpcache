package main

import (
	"flag"
	"log"
	"os"

	"net/http"
	"net/http/httputil"

	"github.com/lox/httpcache"
)

const (
	defaultListen = "0.0.0.0:3124"
)

// command line arguments
var (
	listen string
)

func init() {
	flag.StringVar(&listen, "listen", defaultListen,
		"the host and port to bind to")
}

func main() {
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			log.Println("proxying", r.Method, r.URL.String())
		},
	}

	// build up our handler chain
	cacher := httpcache.NewHandler(httpcache.NewPrivateCache(), proxy)
	logger := httpcache.NewLogger(os.Stderr, cacher)

	log.Printf("proxy listening on http://%s", listen)
	log.Fatal(http.ListenAndServe(listen, logger))
}
