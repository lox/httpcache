
# httpcache

`httpcache` provides an RFC2616 compliant golang [http.Handler](http://golang.org/pkg/net/http/#Handler). 

[![wercker status](https://app.wercker.com/status/a76986990d27e72ea656bb37bb93f59f/m "wercker status")](https://app.wercker.com/project/bykey/a76986990d27e72ea656bb37bb93f59f)

## Example

This example if from the included CLI, it runs a caching proxy on http://localhost:8080.

```go
proxy := &httputil.ReverseProxy{
    Director: func(r *http.Request) {
        log.Println("proxying", r.Method, r.URL.String())
    },
}

// build up our handler chain
cacher := httpcache.NewHandler(httpcache.NewPublicCache(), proxy)
logger := httpcache.NewLogger(os.Stderr, cacher)

log.Printf("proxy listening on http://localhost", listen)
log.Fatal(http.ListenAndServe("127.0.0.1:8080", logger))
```

## Todo

- Revalidation
- Heuristic freshness
- Private / Public caching
- Vary support 
- Cache invalidation
- Pluggable backends

## Reading List

Preventing Request Splitting:
 - http://tools.ietf.org/html/draft-ietf-httpbis-p1-messaging-14#section-3.3
 - http://projects.webappsec.org/w/page/13246931/HTTP-Response-Splitting

Latitudes afforded to caches:
 - http://tools.ietf.org/html/draft-ietf-httpbis-p6-cache-14#section-2.2

What Proxies must do:
 - https://www.mnot.net/blog/2011/07/11/what_proxies_must_do

