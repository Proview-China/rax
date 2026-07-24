package assemblycontract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
)

func FuzzSlotContributionCanonicalDigest(f *testing.F) {
	f.Add("praxis.fixture/fuzz", int32(0))
	f.Add("bad id", int32(101))
	f.Fuzz(func(t *testing.T, id string, priority int32) {
		input := assemblytestkit.ValidInput()
		value := input.SlotContributions[0]
		value.ContributionID = id
		value.Priority = priority
		digest, err := assemblycontract.SlotContributionDigestV1(value)
		if err != nil {
			return
		}
		value.Digest = digest
		_ = value.Validate()
	})
}
