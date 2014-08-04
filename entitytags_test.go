package httpcache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsingEntityTags(t *testing.T) {
	tags, err := ParseEntityTags(`W/"xyzzy", W/"r2d2xxxx", "c3piozzzz"`)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, 3, len(tags), "Expected 3 entity tags")
	assert.Equal(t, tags[0], EntityTag{"xyzzy", true})
	assert.Equal(t, tags[1], EntityTag{"r2d2xxxx", true})
	assert.Equal(t, tags[2], EntityTag{"c3piozzzz", false})
}
