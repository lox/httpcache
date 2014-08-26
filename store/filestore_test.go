package store_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/lox/httpcache/store"
)

func testDir(t *testing.T) string {
	d, err := ioutil.TempDir("", "filestore")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFileStore(t *testing.T) {
	dir := testDir(t)
	defer os.RemoveAll(dir)

	s, err := store.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	tester := &storeTester{s}
	tester.Test(t)
}
