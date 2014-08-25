package store_test

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/lox/httpcache/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testData = map[string][]byte{
	"key1": []byte("this is a test string"),
	"key2": []byte("llamas rock"),
	"key://with/slashes#and&chars":   []byte("tough key"),
	"key with space":                 []byte("blargh"),
	strings.Repeat("looooong", 5000): []byte("long key"),
}

type storeTester struct {
	s store.Store
}

func (st *storeTester) Test(t *testing.T) {
	// test writing
	for key, val := range testData {
		for i := 0; i < 5; i++ {
			if err := st.s.WriteStream(key, bytes.NewReader(val)); err != nil {
				t.Fatal(err)
			}
		}
	}

	// test has keys
	for key := range testData {
		require.True(t, st.s.Has(key))
		require.False(t, st.s.Has(key+"doesn't exist"))
	}

	// test reading
	for key, val := range testData {
		for i := 0; i < 5; i++ {
			r, err := st.s.ReadStream(key)
			if err != nil {
				t.Fatal(err)
			}
			out, err := ioutil.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}

			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, string(val), string(out))
		}
	}

	// test copying
	for key := range testData {
		if err := st.s.Copy(key+"new", key); err != nil {
			t.Fatal(err)
		} else {
			require.True(t, st.s.Has(key+"new"))
		}
	}

	// test deleting
	for key := range testData {
		if err := st.s.Delete(key); err != nil {
			t.Fatal(err)
		}
	}

	// test doesn't have keys
	for key := range testData {
		require.False(t, st.s.Has(key))
	}
}
