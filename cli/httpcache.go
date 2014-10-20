package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/lox/httpcache"
)

const (
	defaultListen = "0.0.0.0:8080"
	defaultDir    = "./cachedata"
)

var (
	listen  string
	useDisk bool
	private bool
	dir     string
)

func init() {
	flag.StringVar(&listen, "listen", defaultListen, "the host and port to bind to")
	flag.StringVar(&dir, "dir", defaultDir, "the dir to store cache data in, implies -disk")
	flag.BoolVar(&useDisk, "disk", false, "whether to store cache data to disk")
	flag.BoolVar(&private, "private", false, "make the cache private")
	flag.Parse()
}

func main() {
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
		},
	}

	var cache *httpcache.Cache

	if useDisk || dir != "" {
		log.Printf("storing cached resources in %s", dir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatal(err)
		}
		var err error
		cache, err = httpcache.NewDiskCache(dir)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		cache = httpcache.NewMemoryCache()
	}

	handler := httpcache.NewHandler(cache, proxy)
	handler.Shared = !private

	logger := &httpcache.Logger{
		Handler: handler,
	}

	log.Printf("listening on http://%s", listen)
	log.Fatal(http.ListenAndServe(listen, logger))
}
