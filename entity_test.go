package httpcache

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

// Format is "02 Jan 2006 15:04:05 GMT"
func mustParseTime(s string) time.Time {
	t, err := time.Parse("02 Jan 2006 15:04:05 GMT", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAge(t *testing.T) {
	time1 := mustParseTime("24 Sep 1981 01:00:00 GMT")
	entity := &Entity{
		Body: bytes.NewReader([]byte("llamas")),
		Header: http.Header{
			"Date": []string{time1.Format(http.TimeFormat)},
		},
	}
	age, err := entity.Age(mustParseTime("25 Sep 1981 01:00:00 GMT"))
	if err != nil {
		t.Fatal(err)
	}

	if age != time.Hour*24 {
		t.Fatalf("Age, expected %q, got %q", "24h0m0s", age)
	}
}

func TestMaxAgeFreshness(t *testing.T) {
	entity := &Entity{
		Body: bytes.NewReader([]byte("llamas")),
		Header: http.Header{
			"Cache-Control": []string{"max-age=3600 s-maxage=86400"},
		},
	}

	freshness, err := entity.Freshness(mustParseTime("24 Sep 1981 01:00:00 GMT"))
	if err != nil {
		t.Fatal(err)
	}

	if freshness != time.Second*3600 {
		t.Fatalf("Freshness, expected %q, got %q", "1h0m0s", freshness)
	}
}

func TestSharedMaxAgeFreshness(t *testing.T) {
	entity := &Entity{
		Body: bytes.NewReader([]byte("llamas")),
		Header: http.Header{
			"Cache-Control": []string{"max-age=600 s-maxage=3600"},
		},
	}

	freshness, err := entity.SharedFreshness(mustParseTime("24 Sep 1981 01:00:00 GMT"))
	if err != nil {
		t.Fatal(err)
	}

	if freshness != time.Second*3600 {
		t.Fatalf("Freshness, expected %q, got %q", "1h0m0s", freshness)
	}
}

func TestExpires(t *testing.T) {
	time1 := mustParseTime("24 Sep 1981 01:00:00 GMT")
	entity := &Entity{
		Body: bytes.NewReader([]byte("llamas")),
		Header: http.Header{
			"Expires": []string{time1.Format(http.TimeFormat)},
		},
	}

	expires, err := entity.Expires()
	if err != nil {
		t.Fatal(err)
	}

	if expires != time1 {
		t.Fatalf("Expires, expected %q, got %q", time1, expires)
	}
}

func TestExpiresFreshness(t *testing.T) {
	time1 := mustParseTime("24 Sep 1981 01:00:00 GMT")
	time2 := mustParseTime("25 Sep 1981 01:00:00 GMT")

	entity := &Entity{
		Body: bytes.NewReader([]byte("llamas")),
		Header: http.Header{
			"Expires": []string{time1.AddDate(0, 0, 2).Format(http.TimeFormat)},
		},
	}

	freshness, err := entity.Freshness(time2)
	if err != nil {
		t.Fatal(err)
	}

	if freshness != time.Hour*24 {
		t.Fatalf("Freshness, expected %q, got %q", "24h0m0s", freshness)
	}
}
