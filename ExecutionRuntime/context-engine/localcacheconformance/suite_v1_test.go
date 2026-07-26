package localcacheconformance_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/localcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/localcacheconformance"
)

func TestMemoryFacadeConformanceV1(t *testing.T) {
	localcacheconformance.RunV1(t, func(config localcache.LocalCacheConfigV1) (localcache.ContextLocalCacheFacadeV1, error) {
		return localcache.NewMemoryFacadeV1(config)
	})
}
