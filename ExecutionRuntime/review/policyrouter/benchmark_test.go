package policyrouter_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/policyrouter"
)

func BenchmarkRouterV1(b *testing.B) {
	now := time.Unix(1_900_700_000, 0)
	input := contract.RouteInputV1{ToolCapability: "praxis.tool/future", Profile: contract.ProfileStandardV1, Risk: contract.RiskMediumV1, EffectClass: contract.EffectReversibleV1, Environment: contract.EnvironmentProductionV1, EvidenceSufficient: true, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := policyrouter.DecideV1(input, now); err != nil {
			b.Fatal(err)
		}
	}
}
