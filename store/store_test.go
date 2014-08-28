package store_test

import (
	"bytes"
	"io/ioutil"
	"log"
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
	store.Store
}

func (st *storeTester) Test(t *testing.T) {
	log.Printf("Testing writing")
	for key, val := range testData {
		for i := 0; i < 5; i++ {
			err := st.WriteFrom(key, bytes.NewReader(val))
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	log.Printf("Testing has")
	for key := range testData {
		require.True(t, st.Store.Has(key))
		require.False(t, st.Store.Has(key+"doesn't exist"))
	}

	log.Printf("Testing missing key from read")
	_, err := st.Store.Read("llamas-no-exist")
	require.True(t, store.IsNotExists(err))

	log.Printf("Testing reading")
	for key, val := range testData {
		for i := 0; i < 5; i++ {
			r, err := st.Store.Read(key)
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

	log.Printf("Testing copying")
	for key, val := range testData {
		err := store.Copy(key+"new", key, st.Store)
		if err != nil {
			t.Fatal(err)
		}
		r2, err := st.Store.Read(key + "new")
		if err != nil {
			t.Fatal(err)
		}
		out2, err := ioutil.ReadAll(r2)
		if err != nil {
			t.Fatal(err)
		}
		require.True(t, st.Store.Has(key+"new"))
		assert.Equal(t, string(val), string(out2))
	}

	log.Printf("Testing deleting")
	for key := range testData {
		if err := st.Store.Delete(key); err != nil {
			t.Fatal(err)
		}
	}

	log.Printf("Testing has not")
	for key := range testData {
		require.False(t, st.Store.Has(key))
	}
}
