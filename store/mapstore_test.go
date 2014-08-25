package store_test

import (
	"testing"

	"github.com/lox/httpcache/store"
)

func TestMapStore(t *testing.T) {
	tester := &storeTester{store.NewMapStore()}
	tester.Test(t)
}
