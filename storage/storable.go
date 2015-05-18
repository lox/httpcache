package storage

import (
	"io"
	"net/http"
	"strconv"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type Storable interface {
	Reader() (ReadSeekCloser, error)
	Header() http.Header
	Status() int
	Size() uint64
}

func StorableCopy(w io.Writer, s Storable) (int64, error) {
	reader, err := s.Reader()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	if length, err := strconv.ParseInt(s.Header().Get("Content-Length"), 10, 64); err == nil {
		return io.CopyN(w, reader, length)
	}

	return io.Copy(w, reader)
}
