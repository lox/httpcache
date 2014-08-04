package httpcache

import (
	"errors"
	"fmt"
	"strings"
)

// http://httpwg.github.io/specs/rfc7232.html#header.if-none-match
// http://httpwg.github.io/specs/rfc7232.html#entity.tag.comparison

type EntityTag struct {
	Tag  string
	Weak bool
}

func ParseEntityTags(input string) ([]EntityTag, error) {
	fields := strings.Fields(input)
	result := make([]EntityTag, len(fields))

	for i, f := range fields {
		f = strings.TrimSuffix(f, ",")
		if strings.HasPrefix(f, `W/"`) && strings.HasSuffix(f, `"`) {
			parts := strings.SplitN(f, `"`, 3)
			if len(parts) != 3 {
				return result, errors.New("Failed to parse entity " + f)
			}
			result[i] = EntityTag{parts[1], true}
		} else {
			result[i] = EntityTag{strings.Trim(f, `"`), false}
		}
	}

	return result, nil
}

func (e *EntityTag) String() string {
	if e.Weak {
		return fmt.Sprintf(`W/%q`, e.Tag)
	}
	return fmt.Sprintf(`%q`, e.Tag)
}
