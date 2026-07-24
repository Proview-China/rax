// Package storetestkit contains test-only constructors that depend on the
// concrete memory Store. It is intentionally separate from internal/testkit
// so contract/read-model tests do not acquire a memory -> decisioncurrent SCC.
package storetestkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
)

func NewMemoryStoreV1(clock func() time.Time) *memory.Store {
	store, err := memory.NewStoreWithClockV1(clock)
	if err != nil {
		panic(err)
	}
	return store
}
