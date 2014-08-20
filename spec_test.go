package httpcache_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/lox/httpcache"
	"github.com/stretchr/testify/assert"
)

func testSetup() (*client, *upstreamServer) {
	upstream := &upstreamServer{
		Body:    []byte("llamas"),
		asserts: []func(r *http.Request){},
		Now:     time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	httpcache.Clock = func() time.Time {
		return upstream.Now
	}

	handler := httpcache.NewHandler(
		httpcache.NewMapStore(),
		upstream,
	)

	return &client{handler}, upstream
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
	assert.Equal(t, 10, r2.age)
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

func TestSpecHeadInvalidatesCachedGet(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"

	assert.Equal(t, "MISS", client.get("/").cacheStatus)
	assert.Equal(t, "HIT", client.get("/").cacheStatus)
	assert.Equal(t, "HIT", client.head("/").cacheStatus)

	upstream.Etag = "llamas1"
	assert.Equal(t, "MISS", client.head("/", cc("no-cache")).cacheStatus)
	assert.Equal(t, "MISS", client.get("/").cacheStatus)
}

func TestSpecValidatingStaleResponsesUnchanged(t *testing.T) {
	client, upstream := testSetup()
	upstream.CacheControl = "max-age=60"
	upstream.Etag = "llamas1"
	assert.Equal(t, "MISS", client.get("/").cacheStatus)

	upstream.timeTravel(time.Second * 90)

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
