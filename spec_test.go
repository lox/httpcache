package httpcache_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/lox/httpcache"
	"github.com/lox/httpcache/store"
	"github.com/stretchr/testify/assert"
)

func testSetup() (*client, *upstreamServer) {
	upstream := &upstreamServer{
		Body:    []byte("llamas"),
		asserts: []func(r *http.Request){},
		Now:     time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
		Header:  http.Header{},
	}

	httpcache.Clock = func() time.Time {
		return upstream.Now
	}

	hc := httpcache.NewHandler(
		store.NewMapStore(),
		upstream,
	)

	var handler http.Handler = hc

	if testing.Verbose() {
		handler = &httpcache.Logger{
			Handler: hc,
		}
	}

	return &client{handler}, upstream
}

func TestSpecNoCaching(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=0, no-cache"

	r1 := client.get("/")
	assert.Equal(t, http.StatusOK, r1.Code)
	assert.Equal(t, "SKIP", r1.cacheStatus)
	assert.Equal(t, string(upstream.Body), string(r1.body))

	r2 := client.get("/")
	assert.Equal(t, http.StatusOK, r2.Code)
	assert.Equal(t, "SKIP", r2.cacheStatus)
	assert.Equal(t, string(upstream.Body), string(r1.body))
}

func TestSpecBasicCaching(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"

	r1 := client.get("/")
	assert.Equal(t, "MISS", r1.cacheStatus)
	assert.Equal(t, string(upstream.Body), string(r1.body))

	upstream.timeTravel(time.Second * 10)
	r2 := client.get("/")
	assert.Equal(t, "HIT", r2.cacheStatus)
	assert.Equal(t, string(upstream.Body), string(r2.body))
	assert.Equal(t, time.Second*10, r2.age)
}

func TestSpecHeuristicCaching(t *testing.T) {
	client, upstream := testSetup()
	upstream.LastModified = upstream.Now.AddDate(-1, 0, 0)
	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	upstream.timeTravel(time.Hour * 48)
	r2 := client.get("/")
	assert.Equal(t, "HIT", r2.cacheStatus)
	assert.Equal(t, []string{"113 - \"Heuristic Expiration\""}, r2.Header()["Warning"])
	assert.Equal(t, 1, upstream.requests, "The second request shouldn't validate")
}

func TestSpecNoCachingByDefault(t *testing.T) {
	client, upstream := testSetup()
	upstream.LastModified = time.Time{}
	upstream.Etag = ""

	assert.Equal(t, "SKIP", client.get("/").cacheStatus)
}

func TestSpecNoCachingForInvalidExpires(t *testing.T) {
	client, upstream := testSetup()
	upstream.LastModified = time.Time{}
	upstream.Header.Set("Expires", "-1")

	assert.Equal(t, "SKIP", client.get("/").cacheStatus)
}

func TestSpecRequestsWithoutHostHeader(t *testing.T) {
	client, _ := testSetup()

	r := newRequest("GET", "http://example.org")
	r.Header.Del("Host")
	r.Host = ""

	resp := client.do(r)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"Requests without a Host header should result in a 400")
}

func TestSpecValidatingStaleResponsesUnchanged(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	upstream.Etag = "llamas1"
	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	upstream.timeTravel(time.Second * 90)
	upstream.Header.Add("X-New-Header", "1")

	r2 := client.get("/")
	assert.Equal(t, http.StatusOK, r2.Code)
	assert.Equal(t, "HIT", r2.cacheStatus)
}

func TestSpecValidatingStaleResponsesWithNewContent(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	upstream.timeTravel(time.Second * 90)
	upstream.Body = []byte("brand new content")

	r2 := client.get("/")
	assert.Equal(t, http.StatusOK, r2.Code)
	assert.Equal(t, "MISS", r2.cacheStatus)
	assert.Equal(t, "brand new content", string(r2.body))
	assert.Equal(t, 0, r2.age)
}

func TestSpecValidatingStaleResponsesWithNewEtag(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	upstream.Etag = "llamas1"

	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	upstream.timeTravel(time.Second * 90)
	upstream.Etag = "llamas2"

	r2 := client.get("/")
	assert.Equal(t, http.StatusOK, r2.Code)
	assert.Equal(t, "MISS", r2.cacheStatus)
}

func TestSpecVaryHeader(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	upstream.Vary = "Accept-Language"
	upstream.Etag = "llamas"

	assert.Equal(t, "MISS", client.get("/", "Accept-Language: en").cacheStatus)
	assert.Equal(t, "HIT", client.get("/", "Accept-Language: en").cacheStatus)
	assert.Equal(t, "MISS", client.get("/", "Accept-Language: de").cacheStatus)
	assert.Equal(t, "HIT", client.get("/", "Accept-Language: de").cacheStatus)
}

func TestSpecHeadersPropagated(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	upstream.Header.Add("X-Llamas", "1")
	upstream.Header.Add("X-Llamas", "3")
	upstream.Header.Add("X-Llamas", "2")

	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	r2 := client.get("/")
	assert.Equal(t, "HIT", r2.cacheStatus)
	assert.Equal(t, []string{"1", "3", "2"}, r2.Header()["X-Llamas"])
}

func TestSpecAgeHeaderFromUpstream(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=86400"
	upstream.Header.Set("Age", "3600") //1hr
	assert.Equal(t, time.Hour, client.get("/").age)

	upstream.timeTravel(time.Hour * 2)
	assert.Equal(t, time.Hour*3, client.get("/").age)
}
