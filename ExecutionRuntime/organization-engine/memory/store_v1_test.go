package memory_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
)

func TestMemoryStoreConformanceV1(t *testing.T) {
	conformance.RunStoreAndReaderV1(t, func(*testing.T) (ports.StoreV1, func()) { return memory.NewStore(), func() {} })
}
