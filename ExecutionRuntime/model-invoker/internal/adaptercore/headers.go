package adaptercore

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RequestID returns the first non-empty provider request-id header.
func RequestID(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

// RetryAfter parses the standard Retry-After header using the current time.
func RetryAfter(headers http.Header) time.Duration {
	return RetryAfterAt(headers, time.Now())
}

// RetryAfterAt is the deterministic form used by tests and callers that own a
// clock. Invalid, elapsed, or overflowing values produce zero.
func RetryAfterAt(headers http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(headers.Get("retry-after"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
		if math.IsInf(seconds, 0) || seconds > float64(math.MaxInt64)/float64(time.Second) {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := at.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}

// ResponseHeaders returns a defensive copy of response headers.
func ResponseHeaders(response *http.Response) http.Header {
	if response == nil {
		return nil
	}
	return response.Header.Clone()
}
