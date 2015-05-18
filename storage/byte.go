package storage

import (
	"bytes"
	"net/http"
)

func NewByteStorable(body []byte, status int, h http.Header) *byteStorable {
	return &byteStorable{body, h, status}
}

type byteStorable struct {
	body       []byte
	header     http.Header
	statusCode int
}

func (s *byteStorable) Status() int {
	return s.statusCode
}

func (s *byteStorable) Size() uint64 {
	return uint64(len(s.body))
}

func (s *byteStorable) Header() http.Header {
	return s.header
}

func (s *byteStorable) Reader() (ReadSeekCloser, error) {
	return byteReadSeekCloser{bytes.NewReader(s.body)}, nil
}

type byteReadSeekCloser struct {
	*bytes.Reader
}

func (brsc byteReadSeekCloser) Close() error { return nil }
