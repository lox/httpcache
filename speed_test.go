package httpcache_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/lox/httpcache"
	"github.com/lox/httpcache/store"
)

const (
	keyCount = 1000
	kb       = 1024
	mb       = 1048576
)

var (
	randomData = map[int][]byte{}
)

type randomDataMaker struct {
	src rand.Source
}

func (r *randomDataMaker) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = byte(r.src.Int63() & 0xff)
	}
	return len(p), nil
}

func init() {
	// generating data is slow, do this upfront
	dataMaker := &randomDataMaker{rand.NewSource(time.Now().UnixNano())}
	for _, size := range [...]int{1, 1 * kb, 256 * kb, 1 * mb, 10 * mb} {
		buf := &bytes.Buffer{}
		io.CopyN(buf, dataMaker, int64(size))
		randomData[size] = buf.Bytes()
	}
}

func shuffle(keys []string) {
	ints := rand.Perm(len(keys))
	for i := range keys {
		keys[i], keys[ints[i]] = keys[ints[i]], keys[i]
	}
}

func tmpFileStore(b *testing.B) (store.Store, string) {
	d, err := ioutil.TempDir("", "speedtest")
	if err != nil {
		b.Fatal(err)
	}
	s, err := store.NewFileStore(d)
	if err != nil {
		b.Fatal(err)
	}
	return s, d
}

func genKeys() []string {
	keys := make([]string, keyCount)
	for i := 0; i < keyCount; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	shuffle(keys)
	return keys
}

func benchStoreRead(b *testing.B, size int, s store.Store) {
	b.StopTimer()
	b.SetBytes(int64(size))

	keys := genKeys()
	for _, k := range keys {
		s.WriteFrom(k, bytes.NewReader(randomData[size]))
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		r, err := s.Read(keys[i%len(keys)])
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(ioutil.Discard, r)
		r.Close()
	}
	b.StopTimer()
}

func benchCacheable(b *testing.B, size int, s store.Store) {
	b.StopTimer()
	b.SetBytes(int64(size))

	h := httpcache.NewHandler(s, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
			w.Header().Set("Cache-Control", "max-age=6000")
			w.WriteHeader(http.StatusOK)
			io.Copy(w, bytes.NewReader(randomData[size]))
		}))

	if testing.Verbose() == false {
		h.Logger = log.New(ioutil.Discard, "", 0)
	}

	ts := httptest.NewServer(h)
	defer ts.Close()
	keys := genKeys()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(ts.URL + "/" + keys[i%len(keys)])
		if err != nil {
			b.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(ioutil.Discard, resp.Body)
	}
	b.StopTimer()
}

func BenchmarkStoreRead_32B_MapStore(b *testing.B) {
	benchStoreRead(b, 32, store.NewMapStore())
}

func BenchmarkStoreRead_1K_MapStore(b *testing.B) {
	benchStoreRead(b, 1*kb, store.NewMapStore())
}

func BenchmarkStoreRead_256K_MapStore(b *testing.B) {
	benchStoreRead(b, 256*kb, store.NewMapStore())
}

func BenchmarkStoreRead_1M_MapStore(b *testing.B) {
	benchStoreRead(b, 1*mb, store.NewMapStore())
}

func BenchmarkStoreRead_32B_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchStoreRead(b, 32, s)
}

func BenchmarkStoreRead_1K_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchStoreRead(b, 1*kb, s)
}

func BenchmarkStoreRead_256K_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchStoreRead(b, 256*kb, s)
}

func BenchmarkStoreRead_1M_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchStoreRead(b, 1*mb, s)
}

func BenchmarkServeBaseline_1M(b *testing.B) {
	b.StopTimer()
	b.SetBytes(int64(1 * mb))

	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			io.CopyN(w, bytes.NewReader(randomData[1*mb]), int64(1*mb))
		}))
	defer ts.Close()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(ts.URL)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
	b.StopTimer()
}

func BenchmarkCacheable_32B_MapStore(b *testing.B) {
	benchCacheable(b, 32, store.NewMapStore())
}

func BenchmarkCacheable_1K_MapStore(b *testing.B) {
	benchCacheable(b, 1*kb, store.NewMapStore())
}

func BenchmarkCacheable_256K_MapStore(b *testing.B) {
	benchCacheable(b, 256*kb, store.NewMapStore())
}

func BenchmarkCacheable_1M_MapStore(b *testing.B) {
	benchCacheable(b, 1*mb, store.NewMapStore())
}

func BenchmarkCacheable_10M_MapStore(b *testing.B) {
	benchCacheable(b, 10*mb, store.NewMapStore())
}

func BenchmarkCacheable_32B_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchCacheable(b, 32, s)
}

func BenchmarkCacheable_1K_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchCacheable(b, 1*kb, s)
}

func BenchmarkCacheable_256K_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchCacheable(b, 256*kb, s)
}

func BenchmarkCacheable_1M_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchCacheable(b, 1*mb, s)
}

func BenchmarkCacheable_10M_FileStore(b *testing.B) {
	s, dir := tmpFileStore(b)
	defer os.RemoveAll(dir)

	benchCacheable(b, 10*mb, s)
}
