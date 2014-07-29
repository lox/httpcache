package httpcache

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
)

var keyHeaders []string = []string{
	// "Range",
	// "Content-Range",
	"Host",
}

func Key(r *http.Request) (string, error) {
	hasher := md5.New()
	io.WriteString(hasher, r.URL.String())

	for _, header := range keyHeaders {
		io.WriteString(hasher, r.Header.Get(header))
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
