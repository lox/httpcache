package httpcache

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Key generates a unique string that identifies the resource that
// is being requested. The request method and url are taken into
// account and canonicalized, the result is a hash of the inputs
func Key(method string, u *url.URL) string {
	return fmt.Sprintf("%s:%s",
		method, strings.ToLower(CanonicalUrl(u).String()))
}

// RequestKey generates a Key for a request
func RequestKey(r *http.Request) string {
	return Key(r.Method, r.URL)
}

// VaryKey generates a key from a Vary header
func VaryKey(vary string, r *http.Request) string {
	b := bytes.NewBufferString(RequestKey(r))
	b.WriteString("::")

	for _, header := range strings.Split(vary, ", ") {
		b.WriteString(header + "=" + r.Header.Get(header))
	}

	return strings.TrimSuffix(b.String(), ":")
}

func CanonicalUrl(u *url.URL) *url.URL {
	return u
}
