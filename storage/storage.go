package storage

import "net/http"

// Storage is a place to store cached resources and associated metadata
type Storage interface {

	// Freshen writes just the metadata of a key. Returns error if key doesn't exist.
	Freshen(key string, statusCode int, header http.Header) error

	// Store writes both body and metadata
	Store(key string, s Storable) error

	// Meta returns the statuscode and headers of a resource, or returns an error if missing
	GetMeta(key string) (int, http.Header, error)

	// Get returns a stored resource, or an error if missing
	Get(key string) (Storable, error)

	// Deletes a resource by key
	Delete(key string) error

	// Len returns number of resources stored
	Len() int

	// Keys returns the keys of the resources in storage
	Keys() []string
}

type keyNotFoundError struct {
	message, key string
}

func (k keyNotFoundError) Error() string {
	return k.message
}

func IsErrNotFound(e error) bool {
	return true
}
