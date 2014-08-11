package httpcache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func durationRef(d time.Duration) *time.Duration {
	return &d
}

func TestParsingCacheControl(t *testing.T) {
	table := []struct {
		ccString string
		ccStruct CacheControl
	}{
		{`public, private="set-cookie", max-age=100`, CacheControl{
			Public:        true,
			PrivateFields: []string{"Set-Cookie"},
			MaxAge:        durationRef(time.Second * 100),
		}},
		{` foo="max-age=8",  public`, CacheControl{
			Public: true,
			Extension: map[string][]string{
				"foo": []string{"max-age=8"},
			},
		}},
		{`s-maxage=86400`, CacheControl{
			SMaxAge:         durationRef(time.Second * 86400),
			ProxyRevalidate: true,
		}},
		{`max-stale`, CacheControl{
			MaxStale:    true,
			MaxStaleAge: nil,
		}},
		{`max-stale=60`, CacheControl{
			MaxStale:    true,
			MaxStaleAge: durationRef(time.Second * 60),
		}},
	}

	for i, expect := range table {
		cc, err := ParseCacheControl(expect.ccString)
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, cc.String(), expect.ccStruct.String(),
			fmt.Sprintf("Failed to parse #%d: %q", i+1, expect.ccString))
	}
}
