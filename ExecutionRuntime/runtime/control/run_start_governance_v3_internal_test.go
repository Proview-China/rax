package control

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type runStartBundleReadSpyV3 struct {
	RunSettlementFactPortV2
	reads int
}

func (s *runStartBundleReadSpyV3) InspectRunBundleV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunBundleV3, error) {
	s.reads++
	return RunBundleV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "unexpected bundle read")
}

func (s *runStartBundleReadSpyV3) CreateRunBundleV3(context.Context, RunBundleCreateRequestV3) (RunBundleV3, error) {
	return RunBundleV3{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidState, "not used")
}

func TestRunStartProviderBindingV3RequiresExactCertifiedRevision(t *testing.T) {
	manifest := core.DigestBytes([]byte("manifest"))
	artifact := core.DigestBytes([]byte("artifact"))
	certified := ports.EvidenceProducerBindingRefV2{
		BindingSetID: "binding-set", BindingSetRevision: 1, ComponentID: "custom/provider",
		ManifestDigest: manifest, ArtifactDigest: artifact, Capability: "custom/execute",
	}
	current := ports.ProviderBindingRefV2(certified)
	if !sameRunStartProviderBindingV3(current, certified) {
		t.Fatal("exact certified provider binding was rejected")
	}
	current.BindingSetRevision++
	if sameRunStartProviderBindingV3(current, certified) {
		t.Fatal("higher current BindingSet revision bypassed immutable Run Plan certification")
	}
}

func TestRunStartGovernanceV3InspectValidatesInputBeforeBackend(t *testing.T) {
	spy := &runStartBundleReadSpyV3{}
	gateway := RunStartGovernanceGatewayV3{Runs: spy}
	if _, err := gateway.InspectRunStartV3(context.Background(), core.ExecutionScope{}, ""); err == nil {
		t.Fatal("invalid Run start inspection unexpectedly succeeded")
	}
	if spy.reads != 0 {
		t.Fatalf("invalid Run start inspection reached backend: %d", spy.reads)
	}
}
