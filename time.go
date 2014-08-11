package httpcache

import "time"

var Now = func() time.Time {
	return time.Now()
}
