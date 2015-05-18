package httpcache

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/lox/httpcache/storage"
)

type Dogpile struct {
	sync.Mutex
	entries map[string]*dogpileEntry
}

func NewDogpile() *Dogpile {
	return &Dogpile{
		entries: map[string]*dogpileEntry{},
	}
}

type dogpileEntry struct {
	eof  bool
	f    *os.File
	size int64
}

func newDogpileEntry(w http.ResponseWriter) (*dogpileEntry, error) {
	f, err := ioutil.TempFile("", "httpcache")
	if err != nil {
		return nil, err
	}

	go func() {

	}()

	return &dogpileEntry{f: f}, nil
}

func (de *dogpileEntry) Reader() (storage.ReadSeekCloser, error) {
	return nil, nil
}

func (de *dogpileEntry) Header() http.Header {
	return http.Header{}

}
func (de *dogpileEntry) Status() int {
	return 500

}
func (de *dogpileEntry) Size() uint64 {
	return 0
}

func (d *Dogpile) Resource(w http.ResponseWriter, r *cacheRequest) (*Resource, error) {
	d.Lock()
	defer d.Unlock()

	key := r.Key.String()
	ent, exists := d.entries[key]
	if !exists {
		var err error
		ent, err = newDogpileEntry(w)
		if err != nil {
			return nil, err
		}
		d.entries[key] = ent
	}

	log.Printf("%#v", ent)

	return NewResource(ent)
}
