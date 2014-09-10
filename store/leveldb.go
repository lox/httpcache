package store

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/syndtr/goleveldb/leveldb"
)

type LevelDbStore struct {
	db *leveldb.DB
}

func NewLevelDbStore(dir string) (*LevelDbStore, error) {
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		return nil, err
	}

	return &LevelDbStore{db}, nil
}

func (ldb *LevelDbStore) Has(key string) bool {
	if _, err := ldb.db.Get([]byte(key), nil); err == nil {
		return true
	}
	return false
}

func (ldb *LevelDbStore) Delete(key string) error {
	return ldb.db.Delete([]byte(key), nil)
}

func (ldb *LevelDbStore) Read(key string) (io.ReadCloser, error) {
	data, err := ldb.db.Get([]byte(key), nil)
	if err != nil && err == leveldb.ErrNotFound {
		return nil, ErrNotExists
	} else if err != nil {
		return nil, err
	}

	return ioutil.NopCloser(bytes.NewReader(data)), nil
}

func (ldb *LevelDbStore) WriteFrom(key string, r io.Reader) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	return ldb.db.Put([]byte(key), b, nil)
}
