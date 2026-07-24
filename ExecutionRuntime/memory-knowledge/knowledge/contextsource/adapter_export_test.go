package contextsource

import (
	"testing"
	"time"
)

type AdapterTestFixtureV2 struct {
	Reader  *CurrentReaderV2
	Request CurrentRequestV2
	Now     func() time.Time
	SetNow  func(time.Time)
}

func NewAdapterTestFixtureV2(t *testing.T) AdapterTestFixtureV2 {
	t.Helper()
	fixture := newReaderFixtureV2(t)
	return AdapterTestFixtureV2{Reader: fixture.reader, Request: fixture.request, Now: func() time.Time { return fixture.now }, SetNow: func(value time.Time) { fixture.now = value }}
}
