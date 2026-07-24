package applicationadapter

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

type generationPortStubV1 struct{}

func (*generationPortStubV1) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return runtimeports.GenerationBindingAssociationFactV1{}, errors.New("unexpected generation current read")
}

func (*generationPortStubV1) AssociateGenerationBindingV1(context.Context, runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return runtimeports.GenerationBindingAssociationFactV1{}, errors.New("unexpected generation write")
}

type settlementPortStubV4 struct{}

type checkpointEnforcementPortStubV1 struct{}

func (*checkpointEnforcementPortStubV1) EnforceCurrentCheckpointRestoreDispatchV1(context.Context, runtimeports.EnforceCurrentCheckpointRestoreDispatchRequestV1) (runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	return runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1{}, errors.New("unexpected checkpoint enforcement")
}

func (*checkpointEnforcementPortStubV1) InspectCurrentCheckpointRestoreDispatchV1(context.Context, runtimeports.InspectCurrentCheckpointRestoreDispatchRequestV1) (runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	return runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1{}, errors.New("unexpected checkpoint enforcement inspect")
}

func (*settlementPortStubV4) SettleOperationV4(context.Context, runtimeports.OperationSettlementSubmissionV4) (runtimeports.OperationSettlementRefV4, error) {
	return runtimeports.OperationSettlementRefV4{}, errors.New("unexpected settlement")
}
func (*settlementPortStubV4) InspectOperationSettlementV4(context.Context, runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementFactV4, error) {
	return runtimeports.OperationSettlementFactV4{}, errors.New("unexpected settlement inspect")
}
func (*settlementPortStubV4) InspectOperationSettlementClosureV4(context.Context, runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementCommitBundleV4, error) {
	return runtimeports.OperationSettlementCommitBundleV4{}, errors.New("unexpected settlement closure inspect")
}
func (*settlementPortStubV4) InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	return runtimeports.OperationInspectionSettlementRefV4{}, errors.New("unexpected current settlement inspect")
}
func (*settlementPortStubV4) InspectOperationSettlementEvidenceAssociationV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	return runtimeports.OperationSettlementEvidenceAssociationV4{}, errors.New("unexpected settlement evidence inspect")
}
func (*settlementPortStubV4) InspectOperationSettlementTerminalGuardV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementTerminalGuardRefV4) (runtimeports.OperationSettlementTerminalGuardV4, error) {
	return runtimeports.OperationSettlementTerminalGuardV4{}, errors.New("unexpected settlement guard inspect")
}
func (*settlementPortStubV4) InspectOperationSettlementTerminalProjectionV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementTerminalProjectionRefV4) (runtimeports.OperationSettlementTerminalProjectionV4, error) {
	return runtimeports.OperationSettlementTerminalProjectionV4{}, errors.New("unexpected settlement projection inspect")
}

type productionDomainBindingStoreV4 struct {
	mu     sync.Mutex
	values map[string]runtimeports.OperationSettlementDomainResultFactRefV4
}

func (s *productionDomainBindingStoreV4) CreateDomainResultRuntimeBindingV4(_ context.Context, value runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[value.ID]; ok {
		if runtimeports.SameOperationSettlementDomainResultFactRefV4(existing, value) {
			return existing, nil
		}
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("binding conflict")
	}
	s.values[value.ID] = value
	return value, nil
}

func (s *productionDomainBindingStoreV4) InspectDomainResultRuntimeBindingV4(_ context.Context, id string) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id]
	if !ok {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("binding not found")
	}
	return value, nil
}

func TestProductionCompositionV4WiresPublicPortsAndStrictSockets(t *testing.T) {
	config := productionCompositionConfigFixtureV4()
	composition, err := NewProductionCompositionV4(config)
	if err != nil {
		t.Fatal(err)
	}
	if composition.Lifecycle == nil || composition.SandboxCurrent == nil {
		t.Fatal("production composition omitted a public lifecycle or current-reader port")
	}
	if composition.CurrentServer.SocketPath != config.CurrentSocketPath || composition.CurrentServer.SocketMode != config.CurrentSocketMode || composition.CurrentServer.AllowedUID != config.CurrentAllowedUID || composition.CurrentServer.Governance != config.Enforcement || composition.CurrentServer.CheckpointGovernance != config.CheckpointEnforcement || composition.CurrentServer.Sandbox != config.Current {
		t.Fatalf("reverse-current server drifted from exact production config: %#v", composition.CurrentServer)
	}
}

func TestProductionCompositionV4RejectsTypedNilAndUnsafeSocketTopology(t *testing.T) {
	config := productionCompositionConfigFixtureV4()
	var typedNil *testkit.MemoryStore
	config.Facts = typedNil
	if _, err := NewProductionCompositionV4(config); err == nil {
		t.Fatal("typed-nil durable Fact store was accepted")
	}
	config = productionCompositionConfigFixtureV4()
	config.CurrentSocketPath = config.DataPlaneSocketPath
	if _, err := NewProductionCompositionV4(config); err == nil {
		t.Fatal("one UDS path was accepted for both directions")
	}
	config = productionCompositionConfigFixtureV4()
	config.CurrentSocketMode = 0o666
	if _, err := NewProductionCompositionV4(config); err == nil {
		t.Fatal("world-accessible reverse-current UDS was accepted")
	}
	config = productionCompositionConfigFixtureV4()
	config.CurrentSocketMode = fs.ModeSetuid | 0o660
	if _, err := NewProductionCompositionV4(config); err == nil {
		t.Fatal("non-permission UDS mode bits were accepted")
	}
}

func productionCompositionConfigFixtureV4() ProductionCompositionConfigV4 {
	store := testkit.NewMemoryStore()
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	return ProductionCompositionConfigV4{
		Facts: store, Current: store, GenerationBindings: &generationPortStubV1{},
		Enforcement: &enforcementPortStubV4{}, CheckpointEnforcement: &checkpointEnforcementPortStubV1{}, Evidence: &evidencePortStubV3{}, Settlements: &settlementPortStubV4{},
		DomainResultBindings: &productionDomainBindingStoreV4{values: make(map[string]runtimeports.OperationSettlementDomainResultFactRefV4)},
		LifecyclePlans:       &lifecyclePlanReaderStubV4{}, LifecycleResults: &lifecycleResultStoreStubV4{},
		DomainResultOwner: runtimeports.ProviderBindingRefV2{
			BindingSetID: "sandbox-bindings", BindingSetRevision: 1, ComponentID: "praxis.sandbox/controller",
			ManifestDigest: digest("sandbox-manifest"), ArtifactDigest: digest("sandbox-artifact"), Capability: "praxis.sandbox/domain-result-owner",
		},
		DomainResultSchema: runtimeports.SchemaRefV2{
			Namespace: "praxis.sandbox", Name: "domain-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("domain-result-schema"),
		},
		DataPlaneSocketPath: "/run/praxis/sandbox-dataplane.sock", DataPlaneAllowedUID: 101,
		CurrentSocketPath: "/run/praxis/sandbox-current.sock", CurrentSocketMode: 0o660, CurrentAllowedUID: 102,
		Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
	}
}
