package httpcache

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Key generates a cache key from MD5(Lowercase(METHOD),Canonical(URL))
func Key(method, url string) (string, error) {
	canonical, err := CanonicalUrl(url)
	if err != nil {
		return "", err
	}

	hasher := md5.New()
	io.WriteString(hasher, strings.ToLower(method))
	io.WriteString(hasher, canonical)

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// SecondaryKey generates a key from MD5(Key,Headers)
func SecondaryKey(method, url string, headers http.Header) (string, error) {
	primary, err := Key(method, url)
	if err != nil {
		return "", err
	}

	hasher := md5.New()
	io.WriteString(hasher, string(primary))
	headers.Write(hasher)

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// ConditionalKey generates a key from MD5(SecondaryKey(If-None-Match && If-Modified-Since))
// The second return parameter will be false if the request isn't conditional
func ConditionalKey(method, url string, headers http.Header) (string, bool, error) {
	validators := http.Header{}

	if v := headers.Get("If-None-Match"); v != "" {
		validators.Set("If-None-Match", v)
	}

	if v := headers.Get("If-Modified-Since"); v != "" {
		validators.Set("If-Modified-Since", v)
	}

	if len(validators) > 0 {
		if key, err := SecondaryKey(method, url, validators); err != nil {
			return "", false, err
		} else {
			return key, true, nil
		}
	}

	return "", false, nil
}

func CanonicalUrl(url string) (string, error) {
	return strings.ToLower(url), nil
}
