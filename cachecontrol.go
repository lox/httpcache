package httpcache

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	CacheControlHeader = "Cache-Control"
)

type CacheControl struct {
	// common directives
	NoCache     bool
	NoStore     bool
	NoTransform bool
	Extension   map[string][]string
	MaxAge      *time.Duration

	// request directives
	MaxStale     bool
	MaxStaleAge  *time.Duration
	MinFresh     *time.Duration
	OnlyIfCached bool

	// response directives
	SMaxAge         *time.Duration
	Public          bool
	Private         bool
	PrivateFields   []string
	NoCacheFields   []string
	MustRevalidate  bool
	ProxyRevalidate bool
}

// ParseCacheControl parses a RFC2616 Cache-Control header
func ParseCacheControl(s string) (cc CacheControl, err error) {
	cc = CacheControl{
		Extension: map[string][]string{},
	}

	if s == "" {
		return
	}

	lexer := ccLexer{
		input: s,
		width: len(s),
	}

	noValues := map[string]bool{
		"public":           true,
		"no-store":         true,
		"no-transform":     true,
		"must-revalidate":  true,
		"proxy-revalidate": true,
		"only-if-cached":   true,
	}

	for directive := range lexer.Lex() {
		if val := noValues[directive.key]; val && directive.value != "" {
			return cc, fmt.Errorf("Directive %s shouldn't have a value", directive.key)
		}

		switch directive.key {
		case "no-cache":
			if directive.value != "" {
				k := http.CanonicalHeaderKey(directive.value)
				if cc.NoCacheFields == nil {
					cc.NoCacheFields = []string{k}
				} else {
					cc.NoCacheFields = append(cc.NoCacheFields, k)
				}
			} else {
				cc.NoCache = true
			}
		case "no-store":
			cc.NoStore = true
		case "no-transform":
			cc.NoTransform = true
		case "max-age":
			d, err := directive.Seconds()
			if err != nil {
				return cc, fmt.Errorf("Error parsing max-age: %s", err)
			}
			cc.MaxAge = &d
		case "max-stale":
			cc.MaxStale = true
			if directive.value != "" {
				d, err := directive.Seconds()
				if err != nil {
					return cc, fmt.Errorf("Error parsing max-stale: %s", err)
				}
				cc.MaxStaleAge = &d
			}
		case "min-fresh":
			d, err := directive.Seconds()
			if err != nil {
				return cc, fmt.Errorf("Error parsing min-fresh: %s", err)
			}
			cc.MinFresh = &d
		case "only-if-cached":
			cc.NoCache = true
		case "s-max-age":
			fallthrough
		case "s-maxage":
			d, err := directive.Seconds()
			if err != nil {
				return cc, fmt.Errorf("Error parsing s-maxage: %s", err)
			}
			cc.SMaxAge = &d
			cc.ProxyRevalidate = true
		case "public":
			cc.Public = true
		case "private":
			if directive.value != "" {
				k := http.CanonicalHeaderKey(directive.value)
				if cc.PrivateFields == nil {
					cc.PrivateFields = []string{k}
				} else {
					cc.PrivateFields = append(cc.PrivateFields, k)
				}
			} else {
				cc.Private = true
			}
		default:
			_, ok := cc.Extension[directive.key]
			if ok {
				cc.Extension[directive.key] = append(cc.Extension[directive.key], directive.value)
			} else {
				cc.Extension[directive.key] = []string{directive.value}
			}
		}
	}

	return
}

const (
	whitespace = " \t"
)

type ccDirective struct {
	key   string
	value string
}

func (d *ccDirective) Seconds() (time.Duration, error) {
	i, err := strconv.Atoi(d.value)
	if err != nil {
		return time.Duration(0), err
	}
	return time.Duration(i) * time.Second, nil
}

type ccLexer struct {
	input      string
	pos, width int
	state      int
	inToken    bool
}

func (l *ccLexer) current() string {
	if l.valid() {
		return string(l.input[l.pos])
	} else {
		return ""
	}
}

func (l *ccLexer) valid() bool {
	return l.pos < l.width
}

func (l *ccLexer) skipWhitespace() {
	for l.valid() {
		if !strings.ContainsAny(l.current(), whitespace) {
			return
		}
		l.pos++
	}
}

func (l *ccLexer) scanUntil(any string) string {
	buffer := ""
	for l.valid() {
		if strings.ContainsAny(l.current(), any) {
			break
		}
		buffer = buffer + l.current()
		l.pos++
	}
	return buffer
}

func (l *ccLexer) scanDirective() ccDirective {
	key := l.scanUntil("=,")
	val := ""

	if l.current() == "=" {
		l.pos++
		val = l.scanValue()
	}

	return ccDirective{strings.ToLower(key), val}
}

func (l *ccLexer) scanValue() string {
	if l.current() == "\"" {
		l.pos++
		quote := l.scanUntil("\"")
		l.pos++
		return quote
	}

	return l.scanUntil(", ")
}

func (l *ccLexer) Lex() chan ccDirective {
	ch := make(chan ccDirective)

	go func() {
		for l.valid() {
			l.skipWhitespace()

			if !l.valid() {
				break
			}

			if l.current() == "," {
				l.pos++
			} else {
				ch <- l.scanDirective()
			}
		}
		close(ch)
	}()

	return ch
}
