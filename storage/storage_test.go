package storage

import (
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
)

func storageImpls(size uint64) []Storage {
	disk, err := NewDiskStorage("/tmp/httpcachetest", 0700, size)
	if err != nil {
		panic(err)
	}

	return []Storage{NewMemoryStorage(size), disk}
}

func testStorable(body string, statusCode int, h ...http.Header) Storable {
	r := &byteStorable{body: []byte(body)}
	r.statusCode = statusCode

	if len(h) > 0 {
		r.header = h[0]
	}
	return r
}

func readAllStorable(s Storable, t *testing.T) []byte {
	reader, err := s.Reader()
	if err != nil {
		t.Fatal(err)
		return nil
	}
	defer reader.Close()
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return b
}

func assertEqual(got, expected string, t *testing.T) {
	if got != expected {
		t.Fatalf("Got string %q, expected %q", got, expected)
	}
}

func TestStorageStoreAndOverwrite(t *testing.T) {
	for _, s := range storageImpls(1024) {
		if err := s.Store("test", testStorable("Testing Response", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		res1, err := s.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(string(readAllStorable(res1, t)), "Testing Response", t)

		if res1.Status() != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", res1.Status())
		}

		if err := s.Store("test", testStorable("Overwritten", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		res2, err := s.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(string(readAllStorable(res2, t)), "Overwritten", t)
	}
}

func TestStorageSizeConstrained(t *testing.T) {
	for _, s := range storageImpls(50) {
		if err := s.Store("testkey1", testStorable("xxx", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		if err := s.Store("testkey2", testStorable("xxxxxxxxxxxxxxx", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		if err := s.Store("testkey3", testStorable("xxxxxxxxxxxxxxxxxx", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		keys := s.Keys()
		if !reflect.DeepEqual(keys, []string{"testkey3", "testkey2"}) {
			t.Fatalf("Expected trimmed keys to be []string{testkey3, testkey2}, was %#v", keys)
		}
	}
}

func TestStorageDelete(t *testing.T) {
	for _, s := range storageImpls(1024) {
		if err := s.Store("test", testStorable("xxx", http.StatusOK)); err != nil {
			t.Fatal(err)
		}

		res1, err := s.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(string(readAllStorable(res1, t)), "xxx", t)

		err = s.Delete("test")
		if err != nil {
			t.Fatal(err)
		}

		_, err = s.Get("test")
		if err == nil {
			t.Fatal("Expected error when requesting deleted resource")
		}
	}
}

func TestStorageFreshen(t *testing.T) {
	for _, s := range storageImpls(1024) {
		if err := s.Store("test", testStorable("Testing Response", http.StatusOK, http.Header{"X-Test": []string{"llamas"}})); err != nil {
			t.Fatal(err)
		}

		res1, err := s.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(string(readAllStorable(res1, t)), "Testing Response", t)
		assertEqual(res1.Header()["X-Test"][0], "llamas", t)

		if err := s.Freshen("test", http.StatusOK, http.Header{"X-Test": []string{"alpacas"}}); err != nil {
			t.Fatal(err)
		}

		res2, err := s.Get("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(string(readAllStorable(res2, t)), "Testing Response", t)
		assertEqual(res1.Header()["X-Test"][0], "alpacas", t)
	}
}

func TestStorageGetMeta(t *testing.T) {
	for _, s := range storageImpls(1024) {
		if err := s.Store("test", testStorable("Testing Response", http.StatusOK, http.Header{"X-Test": []string{"llamas"}})); err != nil {
			t.Fatal(err)
		}

		_, header, err := s.GetMeta("test")
		if err != nil {
			t.Fatal(err)
		}

		assertEqual(header["X-Test"][0], "llamas", t)
		// assertEqual(statusCode, http.StatusOK, t)
	}
}
