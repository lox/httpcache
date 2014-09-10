package store_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/lox/httpcache/store"
)

func TestLevelDb(t *testing.T) {
	d, err := ioutil.TempDir("", "leveldb")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(d)

	s, err := store.NewLevelDbStore(d)
	if err != nil {
		t.Fatal(err)
	}

	tester := &storeTester{s}
	tester.Test(t)
}
