package httpcache

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	CacheControlHeader = "Cache-Control"
)

type CacheControl map[string][]string

func ParseCacheControl(input string) (CacheControl, error) {
	cc := make(CacheControl)
	length := len(input)

	var inToken, inQuote bool
	var offset int

	// split the string into tokens key=value
	for i := 0; i < length; i++ {
		c := input[i]

		if inToken && c == ',' && !inQuote {
			addToken(cc, input[offset:i])
			inToken = false
		} else if inToken && c == '"' && i > 0 && input[i-1] == '=' {
			inQuote = true
		} else if !inToken && (c != ',' && c != ' ') {
			inToken = true
			offset = i
		} else if inToken && inQuote && c == '"' {
			addToken(cc, input[offset:i+1])
			inToken = false
		}
	}

	// process leftovers
	if offset < length {
		addToken(cc, input[offset:length])
	}

	return cc, nil
}

func addToken(cc CacheControl, input string) {
	var key, val string

	if idx := strings.Index(input, "="); idx != -1 {
		key = input[0:idx]
		val = strings.Trim(input[idx+1:], `"`)
	} else {
		key = input
	}

	cc.Add(key, val)
}

func (cc CacheControl) Get(key string) (string, bool) {
	v, exists := cc[key]
	if exists && len(v) > 0 {
		return v[0], true
	}
	return "", exists
}

func (cc CacheControl) Add(key, val string) {
	if !cc.Has(key) {
		cc[key] = []string{}
	}
	if val != "" {
		cc[key] = append(cc[key], val)
	}
}

func (cc CacheControl) Has(key string) bool {
	_, exists := cc[key]
	return exists
}

func (cc CacheControl) Duration(key string) (time.Duration, error) {
	d, _ := cc.Get(key)
	return time.ParseDuration(d + "s")
}

func (cc CacheControl) String() string {
	buf := bytes.Buffer{}
	for k, vals := range cc {
		if len(vals) == 0 {
			buf.WriteString(k + ", ")
		}
		for _, val := range vals {
			if strings.ContainsAny(val, `,"= `) {
				buf.WriteString(fmt.Sprintf("%s=%q, ", k, val))
			} else if val != "" {
				buf.WriteString(fmt.Sprintf("%s=%s, ", k, val))
			}
		}
	}

	return strings.TrimSuffix(buf.String(), ", ")
}

func (cc CacheControl) Equals(cc2 CacheControl) bool {
	if len(cc) != len(cc2) {
		return false
	}

	for k, vals := range cc {
		vals2 := cc2[k]
		if len(vals2) != len(vals) {
			return false
		}
		for i, val := range vals {
			if val != vals2[i] {
				return false
			}
		}
	}

	return true
}
