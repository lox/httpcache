package httpcache_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/lox/httpcache"
	"github.com/lox/httpcache/store"
)

func BenchmarkAccessParallel(b *testing.B) {
	log.SetOutput(ioutil.Discard)

	// an upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "max-age=100")
		fmt.Fprintln(w, "Hello, client")
	}))
	defer upstream.Close()

	u, _ := url.Parse(upstream.URL)

	cache := httpcache.NewHandler(store.NewMapStore(), httputil.NewSingleHostReverseProxy(u))
	cacheServer := httptest.NewServer(cache)
	defer cacheServer.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			res, err := http.Get(cacheServer.URL)
			if err != nil {
				log.Fatal(err)
			}
			_, err = ioutil.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				log.Fatal(err)
			}
		}
	})
}
