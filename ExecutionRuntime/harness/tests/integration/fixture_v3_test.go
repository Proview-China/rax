package integration_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessfakes "github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const integrationModelTurnKindV3 runtimeports.NamespacedNameV2 = "praxis.harness/model-turn"

func integrationDigestV3(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func integrationPayloadV3(value string) runtimeports.OpaquePayloadV2 {
	payload := []byte(value)
	return runtimeports.OpaquePayloadV2{
		Schema: runtimeports.SchemaRefV2{
			Namespace: "praxis.integration", Name: "payload", Version: "1.0.0",
			MediaType: "application/octet-stream", ContentDigest: integrationDigestV3("schema/payload"),
		},
		ContentDigest: integrationDigestV3(value), Length: uint64(len(payload)), Inline: payload,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.integration/default", Digest: integrationDigestV3("opaque-policy")},
	}
}

func integrationSettlementSchemaV3() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.integration", Name: "run-settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: integrationDigestV3("run-settlement-schema")}
}

type integrationBindingBundleV3 struct {
	store      *runtimefakes.BindingStoreV2
	set        control.BindingSetFactV2
	host       runtimeports.EvidenceProducerBindingRefV2
	harness    runtimeports.ProviderBindingRefV2
	provider   runtimeports.ProviderBindingRefV2
	boundByID  map[string]control.BindingFactV2
	manifestBy map[runtimeports.ComponentIDV2]runtimeports.ComponentManifestV2
}

func newIntegrationBindingsV3(t *testing.T, now time.Time) integrationBindingBundleV3 {
	t.Helper()
	type specification struct {
		id           string
		kind         string
		locality     runtimeports.LocalityV2
		capabilities []runtimeports.CapabilityNameV2
	}
	specs := []specification{
		{id: "praxis.runtime/host-governance", kind: "praxis.runtime/host-governance", locality: runtimeports.LocalityHostControlPlane, capabilities: []runtimeports.CapabilityNameV2{control.RunSettlementPlanCertifyCapabilityV3, "runtime/settle-run"}},
		{id: "praxis.harness/domain-adapter", kind: "praxis.harness/domain-adapter", locality: runtimeports.LocalityHostControlPlane, capabilities: []runtimeports.CapabilityNameV2{"praxis.harness/relay-model-turn"}},
		{id: "praxis.model/provider", kind: "praxis.model/provider", locality: runtimeports.LocalityInstanceDataPlane, capabilities: []runtimeports.CapabilityNameV2{"praxis.model/invoke"}},
	}
	manifests := make([]runtimeports.ComponentManifestV2, 0, len(specs))
	registrations := make([]runtimeports.GovernanceRegistrationV2, 0, len(specs))
	for _, spec := range specs {
		provided := make([]runtimeports.ProvidedCapabilityV2, 0, len(spec.capabilities))
		for _, capability := range spec.capabilities {
			schemas := []runtimeports.SchemaRefV2{}
			if capability == "runtime/settle-run" {
				schemas = []runtimeports.SchemaRefV2{integrationSettlementSchemaV3()}
			}
			provided = append(provided, runtimeports.ProvidedCapabilityV2{Capability: capability, TTLSeconds: 3600, Schemas: schemas})
		}
		manifestSchemas := []runtimeports.SchemaRefV2{}
		if spec.id == "praxis.runtime/host-governance" {
			manifestSchemas = []runtimeports.SchemaRefV2{integrationSettlementSchemaV3()}
		}
		manifest := runtimeports.ComponentManifestV2{
			ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: runtimeports.ComponentIDV2(spec.id), Kind: runtimeports.ComponentKindV2(spec.kind),
			GovernanceCategory: "praxis.integration/component", SemanticVersion: "1.0.0", ArtifactDigest: integrationDigestV3("artifact/" + spec.id),
			Contract: runtimeports.ContractBindingV2{Name: "praxis.integration/component", Version: "2.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}},
			Schemas:  manifestSchemas, Locality: spec.locality, Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: provided,
			Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualInspectable,
			Owners:      []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: runtimeports.ComponentIDV2(spec.id)}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: runtimeports.ComponentIDV2(spec.id)}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: runtimeports.ComponentIDV2(spec.id)}},
			Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
		}
		manifests = append(manifests, manifest)
		registrations = append(registrations, runtimeports.GovernanceRegistrationV2{Kind: manifest.Kind, Category: manifest.GovernanceCategory, Capabilities: append([]runtimeports.CapabilityNameV2{}, spec.capabilities...), Schemas: append([]runtimeports.SchemaRefV2{}, manifestSchemas...), ExtensionPolicies: []runtimeports.ExtensionPolicyV2{}, AllowedLocalities: []runtimeports.LocalityV2{spec.locality}, AllowedConformance: []runtimeports.ConformanceLevel{runtimeports.ConformanceFullyControlled}})
	}
	catalog := runtimeports.GovernanceCatalogV2{Registrations: registrations}
	governanceDigest, err := catalog.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	store := runtimefakes.NewBindingStoreV2(func() time.Time { return now.Add(3 * time.Second) })
	certified := make([]control.BindingFactV2, 0, len(manifests))
	requirements := make([]runtimeports.BindingRequirementV2, 0, len(manifests))
	for index, manifest := range manifests {
		manifestDigest, err := manifest.BindingDigestV2()
		if err != nil {
			t.Fatal(err)
		}
		declared := control.BindingFactV2{ID: "binding-" + string(manifest.ComponentID), ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governanceDigest, State: control.BindingDeclared, Revision: 1, Grants: []runtimeports.CapabilityGrantV2{}}
		if _, err := store.CreateBinding(context.Background(), declared); err != nil {
			t.Fatal(err)
		}
		probed := declared
		probed.State, probed.Revision, probed.ProbedUnixNano, probed.ExpiresUnixNano = control.BindingProbed, 2, now.UnixNano(), now.Add(time.Hour).UnixNano()
		for _, capability := range manifest.ProvidedCapabilities {
			probed.Grants = append(probed.Grants, runtimeports.CapabilityGrantV2{Capability: capability.Capability, EvidenceDigest: integrationDigestV3("grant/" + string(capability.Capability)), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
		}
		if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
			t.Fatal(err)
		}
		cert := probed
		cert.State, cert.Revision, cert.CertifiedUnixNano, cert.ConformanceEvidenceDigest = control.BindingCertified, 3, now.Add(time.Second).UnixNano(), integrationDigestV3("conformance/"+string(manifest.ComponentID))
		if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: cert}); err != nil {
			t.Fatal(err)
		}
		certified = append(certified, cert)
		caps := make([]runtimeports.CapabilityNameV2, 0, len(manifest.ProvidedCapabilities))
		for _, capability := range manifest.ProvidedCapabilities {
			caps = append(caps, capability.Capability)
		}
		requirements = append(requirements, runtimeports.BindingRequirementV2{ComponentID: manifest.ComponentID, Kind: manifest.Kind, SemanticVersion: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: manifest.Contract.Name, Contract: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}, ArtifactDigest: manifest.ArtifactDigest, RequiredCapabilities: caps, Required: true})
		_ = index
	}
	bindingPlan, err := runtimeports.SealBindingPlanV2(runtimeports.BindingPlanV2{ID: "binding-plan-integration", GovernanceDigest: governanceDigest, Requirements: requirements})
	if err != nil {
		t.Fatal(err)
	}
	set, err := control.BuildBindingSetV2("binding-set-integration", bindingPlan, catalog, certified, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	expected := make([]control.ExpectedBindingRevisionV2, 0, len(certified))
	for _, fact := range certified {
		expected = append(expected, control.ExpectedBindingRevisionV2{BindingID: fact.ID, ExpectedRevision: fact.Revision})
	}
	set, err = store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: expected})
	if err != nil {
		t.Fatal(err)
	}
	bound := map[string]control.BindingFactV2{}
	manifestBy := map[runtimeports.ComponentIDV2]runtimeports.ComponentManifestV2{}
	for _, member := range set.Members {
		fact, inspectErr := store.InspectBinding(context.Background(), member.BindingID)
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		bound[fact.ID], manifestBy[fact.ComponentID] = fact, fact.Manifest
	}
	refFor := func(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
		for _, member := range set.Members {
			if member.ComponentID == component {
				return runtimeports.ProviderBindingRefV2{BindingSetID: set.ID, BindingSetRevision: set.Revision, ComponentID: component, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, Capability: capability}
			}
		}
		t.Fatalf("binding component missing: %s", component)
		return runtimeports.ProviderBindingRefV2{}
	}
	hostProvider := refFor("praxis.runtime/host-governance", control.RunSettlementPlanCertifyCapabilityV3)
	return integrationBindingBundleV3{
		store: store, set: set, boundByID: bound, manifestBy: manifestBy,
		host:     runtimeports.EvidenceProducerBindingRefV2(hostProvider),
		harness:  refFor("praxis.harness/domain-adapter", "praxis.harness/relay-model-turn"),
		provider: refFor("praxis.model/provider", "praxis.model/invoke"),
	}
}

type integrationCurrentReaderV3 struct {
	mu       sync.Mutex
	snapshot runtimeports.OperationGovernanceSnapshotV3
	intent   runtimeports.OperationEffectIntentV3
}

func (r *integrationCurrentReaderV3) InspectOperationGovernance(_ context.Context, subject runtimeports.OperationSubjectV3) (runtimeports.OperationGovernanceSnapshotV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !runtimeports.SameOperationSubjectV3(subject, r.snapshot.Operation) {
		return runtimeports.OperationGovernanceSnapshotV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectFenceStale, "operation projection missing")
	}
	return r.snapshot, nil
}

func (r *integrationCurrentReaderV3) InspectOperationIntentAdmission(_ context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationIntentAdmissionFactV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !runtimeports.SameOperationSubjectV3(intent.Operation, r.intent.Operation) || intent.ID != r.intent.ID {
		return runtimeports.OperationIntentAdmissionFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectAuthorizationMissing, "operation admission missing")
	}
	digest, _ := intent.DigestV3()
	owner := intent.Owners[1]
	return runtimeports.OperationIntentAdmissionFactV3{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: digest, IntentOwner: owner, Provider: intent.Provider, Binding: r.snapshot.Binding, OwnerAttestation: runtimeports.OperationGovernanceFactRefV3{Ref: "owner-attestation", Revision: 1, Digest: integrationDigestV3("owner-attestation"), ExpiresUnixNano: r.snapshot.ExpiresUnixNano}, Active: true, ExpiresUnixNano: r.snapshot.ExpiresUnixNano}, nil
}

type integrationDispatchReaderV3 struct {
	effects     control.OperationEffectFactPortV3
	delegations runtimeports.ExecutionDelegationFactPortV2
}

func (r integrationDispatchReaderV3) InspectOperationDispatch(ctx context.Context, operation runtimeports.OperationSubjectV3, permitID, delegationID string) (runtimeports.OperationDispatchCurrentProjectionV3, error) {
	permit, err := r.effects.InspectOperationDispatchPermitV3(ctx, operation, permitID)
	if err != nil {
		return runtimeports.OperationDispatchCurrentProjectionV3{}, err
	}
	delegation, err := r.delegations.InspectExecutionDelegationV2(ctx, delegationID)
	if err != nil {
		return runtimeports.OperationDispatchCurrentProjectionV3{}, err
	}
	ref, _ := delegation.RefV2()
	projection := runtimeports.OperationDispatchCurrentProjectionV3{Operation: operation, Permit: permit.Permit, PermitDigest: permit.PermitDigest, PermitFactRevision: permit.Revision, PermitFactState: string(permit.State), Delegation: ref, DelegationState: delegation.State, PreparedAttemptID: delegation.PreparedAttemptID, ExpiresUnixNano: delegation.ExpiresUnixNano}
	if permit.Enforcement != nil {
		enforcement, refErr := permit.PersistedEnforcementRefV3()
		if refErr != nil {
			return runtimeports.OperationDispatchCurrentProjectionV3{}, refErr
		}
		projection.Enforcement = &enforcement
	}
	if delegation.Preparation != nil {
		projection.PreparationDigest, err = core.CanonicalJSONDigest("praxis.runtime.execution-governance", runtimeports.ExecutionGovernanceContractVersionV2, "ProviderPreparationAttestationV2", delegation.Preparation)
	}
	return projection, err
}

type integrationEvidenceReaderV3 struct {
	mu       sync.Mutex
	bySource map[runtimeports.EvidenceSourceKeyV2]runtimeports.EvidenceLedgerRecordV2
	byRef    map[runtimeports.EvidenceRecordRefV2]runtimeports.EvidenceLedgerRecordV2
	claim    *integrationClaimEvidenceV3
}

func newIntegrationEvidenceReaderV3() *integrationEvidenceReaderV3 {
	return &integrationEvidenceReaderV3{bySource: map[runtimeports.EvidenceSourceKeyV2]runtimeports.EvidenceLedgerRecordV2{}, byRef: map[runtimeports.EvidenceRecordRefV2]runtimeports.EvidenceLedgerRecordV2{}}
}
func (r *integrationEvidenceReaderV3) add(record runtimeports.EvidenceLedgerRecordV2) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySource[runtimeports.EvidenceSourceKeyV2{RegistrationID: record.Candidate.RegistrationID, SourceEpoch: record.Candidate.SourceEpoch, SourceSequence: record.Candidate.SourceSequence}] = record
	r.byRef[record.Ref] = record
}
func (r *integrationEvidenceReaderV3) InspectBySource(ctx context.Context, key runtimeports.EvidenceSourceKeyV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	r.mu.Lock()
	record, ok := r.bySource[key]
	claim := r.claim
	r.mu.Unlock()
	if !ok {
		if claim != nil {
			return claim.InspectGovernedBySource(ctx, key)
		}
		return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "source evidence missing")
	}
	return record, nil
}
func (r *integrationEvidenceReaderV3) InspectRecord(ctx context.Context, ref runtimeports.EvidenceRecordRefV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	r.mu.Lock()
	record, ok := r.byRef[ref]
	claim := r.claim
	r.mu.Unlock()
	if !ok {
		if claim != nil {
			return claim.InspectGovernedRecord(ctx, ref)
		}
		return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "evidence missing")
	}
	return record, nil
}

type integrationProviderV3 struct {
	mu                 sync.Mutex
	current            runtimeports.OperationGovernanceCurrentReaderV3
	dispatch           runtimeports.OperationDispatchCurrentReaderV3
	evidence           *integrationEvidenceReaderV3
	clock              func() time.Time
	preparations       map[string]runtimeports.ProviderPreparationAttestationV2
	observations       map[string]runtimeports.ProviderAttemptObservationV2
	executeEntries     int
	logicalEffects     int
	inspectEntries     int
	loseExecuteReply   bool
	inspectUnavailable bool
}

func (p *integrationProviderV3) Prepare(ctx context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	now := p.clock()
	dispatch, err := p.dispatch.InspectOperationDispatch(ctx, request.Intent.Operation, request.Permit.ID, request.Delegation.ID)
	if err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	current, err := p.current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	if err := dispatch.ValidateForPrepare(request, current, now); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.preparations[dispatch.PreparedAttemptID]; ok {
		return existing, nil
	}
	operationDigest, _ := request.Intent.Operation.DigestV3()
	permitDigest, _ := request.Permit.DigestV3()
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: dispatch.PreparedAttemptID, Revision: 1, DeclaredDelegation: request.Delegation, OperationDigest: operationDigest, IntentID: request.Intent.ID, IntentRevision: request.Intent.Revision, IntentDigest: request.Permit.IntentDigest, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, PermitDigest: permitDigest, AttemptID: request.Permit.AttemptID, Provider: request.Permit.Provider, PayloadSchema: request.Intent.Payload.Schema, PayloadDigest: request.Intent.Payload.ContentDigest, PayloadRevision: request.Intent.PayloadRevision, PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: request.Permit.ExpiresUnixNano})
	if err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	attestation := runtimeports.ProviderPreparationAttestationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: prepared, Enforcement: runtimeports.OperationEnforcementReceiptV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, AttemptID: request.Permit.AttemptID, PermitDigest: permitDigest, Operation: request.Intent.Operation, Verifier: request.Permit.EnforcementPoint, ValidatedUnixNano: now.UnixNano()}, ObservedUnixNano: now.UnixNano()}
	if err := attestation.ValidateAgainstPrepare(request, now); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	p.preparations[prepared.ID] = attestation
	return attestation, nil
}

func (p *integrationProviderV3) InspectPrepared(_ context.Context, request runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectEntries++
	value, ok := p.preparations[request.PreparedAttemptID]
	if !ok {
		return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "prepared attempt missing")
	}
	return value, nil
}

func (p *integrationProviderV3) ExecutePrepared(ctx context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	now := p.clock()
	dispatch, err := p.dispatch.InspectOperationDispatch(ctx, request.Intent.Operation, request.Permit.ID, request.Delegation.ID)
	if err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	current, err := p.current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	if err := dispatch.ValidateForExecute(request, current, now); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	p.executeEntries++
	if existing, ok := p.observations[request.Prepared.ID]; ok {
		p.mu.Unlock()
		return existing, nil
	}
	p.logicalEffects++
	observation := runtimeports.ProviderAttemptObservationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: request.Prepared, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Payload: integrationPayloadV3("model-result"), PayloadRevision: 1, ProviderOperationRef: "provider-operation-integration", SourceRegistrationID: "provider-source-integration", SourceEpoch: 1, SourceSequence: 1, ObservedUnixNano: now.UnixNano()}
	record := integrationOperationEvidenceV3(request, observation, now)
	observation.Evidence = record.Ref
	p.observations[request.Prepared.ID] = observation
	lose := p.loseExecuteReply
	p.loseExecuteReply = false
	p.mu.Unlock()
	p.evidence.add(record)
	if lose {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Execute reply loss")
	}
	return observation, observation.ValidateAgainstPrepared(request.Prepared)
}

func (p *integrationProviderV3) InspectLocalAttempt(_ context.Context, request runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectEntries++
	value, ok := p.observations[request.Prepared.ID]
	if !ok {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "provider observation missing")
	}
	if p.inspectUnavailable {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Inspect unavailability")
	}
	return value, nil
}

func integrationOperationEvidenceV3(request runtimeports.ExecutePreparedRequestV2, observation runtimeports.ProviderAttemptObservationV2, now time.Time) runtimeports.EvidenceLedgerRecordV2 {
	scope := request.Intent.Operation.ExecutionScope
	ledger := runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID, RunID: request.Intent.Operation.RunID}
	ledgerDigest, _ := ledger.DigestV2()
	candidate := runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: observation.ProviderOperationRef, RegistrationID: observation.SourceRegistrationID, RegistrationRevision: 1, SourceConfigurationDigest: integrationDigestV3("provider-source-config"), SourcePolicy: runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "provider-source-policy", Revision: 1, Digest: integrationDigestV3("provider-source-policy")}, SourceID: "praxis.model/provider", SourceEpoch: observation.SourceEpoch, SourceSequence: observation.SourceSequence, TrustClass: runtimeports.EvidenceTrustAttestation, EventKind: "praxis.model/observation", CustomClass: "praxis.model/observation", ExecutionScope: scope, Payload: runtimeports.EvidencePayloadRefV2{Schema: observation.Payload.Schema, ContentDigest: observation.Payload.ContentDigest, Revision: observation.PayloadRevision, Length: observation.Payload.Length, Ref: "memory://provider-observation"}, Causation: []runtimeports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerDigest, EventID: request.Delegation.ID}}, CorrelationID: request.Prepared.ID, Producer: runtimeports.EvidenceProducerBindingRefV2(request.Intent.Provider), Authority: runtimeports.AuthorityBindingRefV2{Ref: "provider-evidence-authority", Revision: 1, Digest: integrationDigestV3("provider-evidence-authority"), Epoch: scope.AuthorityEpoch}, ObservedUnixNano: observation.ObservedUnixNano}
	record, _ := control.NewEvidenceLedgerRecordV2(candidate, 1, runtimeports.EvidenceGenesisDigestV2, now)
	return record
}

type integrationFixtureV3 struct {
	now           time.Time
	bindings      integrationBindingBundleV3
	harnessStore  *harnessfakes.GovernedStoreV2
	harnessFacts  *harnessfakes.ModelTurnOperationBindingStoreV3
	loop          *kernel.GovernedLoopV2
	domain        *applicationadapter.ModelTurnDomainAdapterV3
	candidate     harnesscontract.ModelTurnCandidateV2
	intent        runtimeports.OperationEffectIntentV3
	plan          applicationcontract.WorkflowPlanV2
	journal       applicationcontract.WorkflowJournalV2
	initial       applicationcontract.GovernedOperationAttemptFactV3
	appAttempts   *applicationfakes.GovernedOperationAttemptStoreV3
	appJournals   *applicationfakes.FactStoreV2
	coordinator   *application.GovernedOperationCoordinatorV3
	effectStore   *runtimefakes.OperationEffectStoreV3
	provider      *integrationProviderV3
	relay         *runtimeadapter.GovernedRelayV2
	evidence      *integrationEvidenceReaderV3
	operationNow  func() time.Time
	runGateway    runtimekernel.RunSettlementGatewayV2
	runFacts      *runtimefakes.RunSettlementStoreV2
	runEffects    *runtimefakes.EffectStoreV2
	planStore     *runtimefakes.RunPlanAdmissionStoreV3
	planGateway   control.RunSettlementPlanAdmissionGatewayV3
	run           core.AgentRunRecord
	runPlan       runtimeports.RunSettlementPlanFactV2
	runPolicies   map[runtimeports.NamespacedNameV2]runtimeports.RunSettlementPolicyFactV2
	certification runtimeports.RunSettlementPlanCertificationFactV3
	claimEvidence *integrationClaimEvidenceV3
	claimStore    *runtimefakes.RunClaimAssociationStoreV2
	claimGateway  runtimekernel.RunClaimGatewayV3
}

func newIntegrationFixtureV3(t *testing.T) *integrationFixtureV3 {
	t.Helper()
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	bindings := newIntegrationBindingsV3(t, now)
	_, template := testkit.GovernedFactsV2(now)
	template.Provider = bindings.provider
	endpoint, err := harnesscontract.NewEndpointRefV2(template.Endpoint.ID, template.Run.Scope, bindings.harness)
	if err != nil {
		t.Fatal(err)
	}
	template.Endpoint = endpoint
	harnessStore := harnessfakes.NewGovernedStoreV2()
	harnessStore.Clock = func() time.Time { return now }
	loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: harnessStore, Candidates: harnessStore, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := loop.PrepareInitialCandidateV2(context.Background(), kernel.PrepareInitialCandidateRequestV2{Run: template.Run, Endpoint: template.Endpoint, SessionID: template.SessionRef, CandidateID: template.ID, Input: template.Input, ContextRef: template.ContextRef, ContextDigest: template.ContextDigest, Provider: template.Provider, CreatedUnixNano: template.CreatedUnixNano, ExpiresUnixNano: template.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	candidate := prepared.Candidate
	payload, err := harnesscontract.NewModelTurnEffectPayloadV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: candidate.Run.Scope, ExecutionScopeDigest: scopeDigest, RunID: candidate.Run.RunID, SubjectRevision: 1, CurrentProjectionRef: "run-projection-integration", CurrentProjectionDigest: integrationDigestV3("run-projection"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-integration", Digest: integrationDigestV3("authority"), Revision: 1, Epoch: candidate.Run.Scope.AuthorityEpoch}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "review-integration", CandidateDigest: integrationDigestV3("review-candidate"), CandidateRevision: 1, PolicyDigest: integrationDigestV3("review-policy")}
	budget := runtimeports.OperationBudgetBindingRefV3{Ref: "budget-integration", Digest: integrationDigestV3("budget"), Revision: 1, PolicyDigest: integrationDigestV3("budget-policy"), SubjectDigest: operationDigest}
	policy := runtimeports.OperationPolicyBindingRefV3{Ref: "policy-integration", Digest: integrationDigestV3("policy"), Revision: 1, SubjectDigest: operationDigest}
	owner := runtimeports.EffectOwnerRefV2{ComponentID: bindings.provider.ComponentID, ManifestDigest: bindings.provider.ManifestDigest}
	intent := runtimeports.OperationEffectIntentV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "model-effect-integration", Revision: 1, Operation: operation, Kind: "praxis.harness/model-turn", RiskClass: "praxis.harness/controlled", ActionScopeDigest: integrationDigestV3("model-action"), Payload: payload, PayloadRevision: 1, Target: "model://integration", ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "praxis.model/turn", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(candidate.Run.Scope.Identity.TenantID)}, Owners: []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest}, {Role: runtimeports.OwnerEffect, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest}, {Role: runtimeports.OwnerSettlement, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest}}, Provider: bindings.provider, Authority: authority, Review: review, Budget: budget, Policy: policy, Idempotency: runtimeports.IdempotencyBindingV2{Key: "model-turn-integration", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(candidate.Run.Scope.Identity.TenantID), Class: core.IdempotencyQueryable}, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(50 * time.Second).UnixNano()}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	descriptor := applicationcontract.StepDescriptorRefV2{Kind: integrationModelTurnKindV3, Revision: 1, Digest: integrationDigestV3("model-step-descriptor"), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	provider, domainAdapter := candidate.Provider, candidate.Endpoint.Binding
	plan := applicationcontract.WorkflowPlanV2{ContractVersion: applicationcontract.WorkflowContractVersionV2, ID: "workflow-integration", Revision: 1, CommandID: "command-integration", CommandPayloadDigest: integrationDigestV3("command"), Target: candidate.Run.Scope, Authority: authority, Steps: []applicationcontract.WorkflowStepV2{{ID: "model-turn", Kind: integrationModelTurnKindV3, Descriptor: descriptor, ExecutionClass: applicationcontract.StepGovernedEffectV2, Required: true, Dependencies: []string{}, Payload: payload, Provider: &provider, DomainAdapter: &domainAdapter}}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	journal, err := applicationcontract.NewWorkflowJournalV2("journal-integration", plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	journalStore := applicationfakes.NewFactStoreV2()
	journalStore.Clock = func() time.Time { return now.Add(time.Second) }
	if _, err := journalStore.CreateWorkflowJournalV2(context.Background(), plan, journal); err != nil {
		t.Fatal(err)
	}
	initial, err := applicationcontract.NewGovernedOperationAttemptFactV3("application-attempt-integration", plan, journal, "model-turn", 1, operation, intent, applicationcontract.OperationDispatchPlanV3{PermitID: "permit-integration", AttemptID: "provider-attempt-integration", PermitTTLNanos: int64(30 * time.Second)}, applicationcontract.ExecutionDelegationPlanV3{ContractVersion: applicationcontract.GovernedOperationAttemptContractVersionV3, DelegationID: "delegation-integration", HostAdapter: bindings.harness, RelayHops: []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: bindings.harness}}, EndpointID: candidate.Endpoint.ID, RuntimeSessionRef: candidate.SessionRef, HostBindingExpiresUnixNano: now.Add(45 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: now.Add(45 * time.Second).UnixNano(), DelegationTTLNanos: int64(20 * time.Second)}, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	facts := harnessfakes.NewModelTurnOperationBindingStoreV3()
	domain, err := applicationadapter.NewModelTurnDomainAdapterV3(applicationadapter.ModelTurnDomainAdapterConfigV3{StepKind: integrationModelTurnKindV3, Adapter: bindings.harness, Bindings: facts, Reservations: harnessStore, Sessions: harnessStore, Candidates: harnessStore, Turns: &kernel.GovernedTurnStateCoordinatorV2{Sessions: harnessStore}, Clock: func() time.Time { return now.Add(2 * time.Second) }})
	if err != nil {
		t.Fatal(err)
	}
	runtimeCurrent := control.ProviderBindingCurrentnessAdapterV2{Bindings: bindings.store, Clock: func() time.Time { return now.Add(3 * time.Second) }}
	currentness, err := application.NewRuntimeBindingCurrentnessAdapterV3(runtimeCurrent, func() time.Time { return now.Add(3 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	router, err := application.NewOperationDomainRouterV3(func() time.Time { return now.Add(3 * time.Second) }, currentness)
	if err != nil {
		t.Fatal(err)
	}
	if err := router.RegisterOperationDomainV3(context.Background(), application.OperationDomainAdapterRegistrationV3{StepKind: integrationModelTurnKindV3, Descriptor: descriptor, Adapter: bindings.harness, Port: domain}); err != nil {
		t.Fatal(err)
	}
	expires := now.Add(40 * time.Second).UnixNano()
	governanceRef := func(ref string, digest core.Digest) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: ref, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	current := &integrationCurrentReaderV3{intent: intent, snapshot: runtimeports.OperationGovernanceSnapshotV3{Operation: operation, Active: true, ProjectionWatermark: 1, Identity: governanceRef("identity-integration", integrationDigestV3("identity")), Binding: runtimeports.OperationGovernanceFactRefV3{Ref: bindings.set.ID, Revision: bindings.set.Revision, Digest: integrationDigestV3("binding-current"), ExpiresUnixNano: expires}, CurrentScope: governanceRef(operation.CurrentProjectionRef, operation.CurrentProjectionDigest), Authority: governanceRef(authority.Ref, authority.Digest), Review: runtimeports.OperationReviewAuthorizationV3{Case: governanceRef(review.CaseRef, integrationDigestV3("review-case")), CandidateDigest: review.CandidateDigest, CandidateRevision: review.CandidateRevision, Verdict: governanceRef("verdict-integration", integrationDigestV3("verdict")), ReviewerAuthority: governanceRef("reviewer-integration", integrationDigestV3("reviewer")), PolicyDigest: review.PolicyDigest, ExpiresUnixNano: expires}, Budget: governanceRef(budget.Ref, budget.Digest), Policy: governanceRef(policy.Ref, policy.Digest), Provider: bindings.provider, EnforcementPoint: bindings.provider, CapabilityGrantDigest: integrationDigestV3("capability-grant"), Credentials: []runtimeports.OperationCredentialCurrentFactV3{}, ExpiresUnixNano: expires}}
	operationClock := func() time.Time { return now.Add(4 * time.Second) }
	effectStore := runtimefakes.NewOperationEffectStoreV3(operationClock)
	delegations := runtimefakes.NewExecutionDelegationStoreV2(operationClock)
	dispatch := integrationDispatchReaderV3{effects: effectStore, delegations: delegations}
	evidence := newIntegrationEvidenceReaderV3()
	providerFixture := &integrationProviderV3{current: current, dispatch: dispatch, evidence: evidence, clock: operationClock, preparations: map[string]runtimeports.ProviderPreparationAttestationV2{}, observations: map[string]runtimeports.ProviderAttemptObservationV2{}}
	relay, err := runtimeadapter.NewGovernedRelayV2(providerFixture, operationClock)
	if err != nil {
		t.Fatal(err)
	}
	delegationGateway := control.ExecutionDelegationGovernanceGatewayV2{Effects: effectStore, Delegations: delegations, Current: current, Clock: operationClock}
	governanceGateway := control.OperationGovernanceGatewayV3{Effects: effectStore, Current: current, Clock: operationClock}
	observationStore := runtimefakes.NewProviderAttemptObservationStoreV2()
	observationGateway := control.OperationObservationGovernanceGatewayV3{Effects: effectStore, Observations: observationStore, Delegations: delegations, Current: current, Dispatch: dispatch, Evidence: evidence, Clock: operationClock}
	settlementGateway := control.OperationSettlementGovernanceGatewayV3{Effects: effectStore, Evidence: evidence, Clock: operationClock}
	appAttempts := applicationfakes.NewGovernedOperationAttemptStoreV3()
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{Attempts: appAttempts, Journals: journalStore, Admission: control.OperationEffectAdmissionGatewayV3{Effects: effectStore, Current: current, Clock: operationClock}, Governance: governanceGateway, Delegations: delegationGateway, Execution: relay, Observations: observationGateway, Settlements: settlementGateway, DomainResolver: router, Clock: operationClock})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &integrationFixtureV3{now: now, bindings: bindings, harnessStore: harnessStore, harnessFacts: facts, loop: loop, domain: domain, candidate: candidate, intent: intent, plan: plan, journal: journal, initial: initial, appAttempts: appAttempts, appJournals: journalStore, coordinator: coordinator, effectStore: effectStore, provider: providerFixture, relay: relay, evidence: evidence, operationNow: operationClock}
	fixture.attachCertifiedRunV3(t)
	return fixture
}

func (f *integrationFixtureV3) attachCertifiedRunV3(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	sessionRef, err := runtimeports.DeriveRuntimeExecutionSessionRefV2(f.candidate.Endpoint.ID, f.candidate.Run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: f.candidate.Run.RunID, Scope: f.candidate.Run.Scope, Status: core.RunPending, Revision: 1, SessionRef: sessionRef}
	runIdentity, _ := runtimeports.RunIdentityDigestV2(run)
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(run.Scope)
	setDigest, err := control.BindingSetDigestV2(f.bindings.set)
	if err != nil {
		t.Fatal(err)
	}
	setSemantic, err := control.BindingSetSemanticDigestV2(f.bindings.set)
	if err != nil {
		t.Fatal(err)
	}
	settlementOwner := f.bindings.host
	settlementOwner.Capability = "runtime/settle-run"
	execution := runtimeports.RunExecutionSubjectV2{EndpointID: f.candidate.Endpoint.ID, EndpointDigest: f.candidate.Endpoint.IdentityDigest, SessionRef: run.SessionRef, Binding: settlementOwner}
	execution.SubjectDigest, err = execution.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	schema := integrationSettlementSchemaV3()
	reserved := []struct {
		kind  runtimeports.NamespacedNameV2
		phase runtimeports.RunSettlementRequirementPhaseV2
	}{
		{runtimeports.RunRequirementExecutionTruth, runtimeports.RunSettlementPhaseCompletion},
		{runtimeports.RunRequirementEffects, runtimeports.RunSettlementPhaseCompletion},
		{runtimeports.RunRequirementRemoteContinuations, runtimeports.RunSettlementPhaseCompletion},
		{runtimeports.RunRequirementDomainCommits, runtimeports.RunSettlementPhaseCompletion},
		{runtimeports.RunRequirementBudget, runtimeports.RunSettlementPhaseCompletion},
		{runtimeports.RunRequirementCleanup, runtimeports.RunSettlementPhaseTerminationReport},
		{runtimeports.RunRequirementResidual, runtimeports.RunSettlementPhaseTerminationReport},
		{runtimeports.RunRequirementProviderRetention, runtimeports.RunSettlementPhaseTerminationReport},
	}
	requirements := make([]runtimeports.RunSettlementRequirementV2, 0, len(reserved))
	policies := make(map[runtimeports.NamespacedNameV2]runtimeports.RunSettlementPolicyFactV2, len(reserved))
	for _, item := range reserved {
		subject := integrationDigestV3("run-subject/" + string(item.kind))
		if item.kind == runtimeports.RunRequirementExecutionTruth {
			subject = execution.SubjectDigest
		}
		requirement := runtimeports.RunSettlementRequirementV2{ID: item.kind, Kind: item.kind, Phase: item.phase, Owner: settlementOwner, Schema: schema, SubjectSelector: "runtime/run", SubjectDigest: subject, EvidenceTrust: runtimeports.EvidenceTrustAttestation, EvidenceKind: "runtime/settlement-attestation"}
		policy, sealErr := runtimeports.SealRunSettlementPolicyFactV2(runtimeports.RunSettlementPolicyFactV2{Ref: "policy-" + string(item.kind[8:]), Revision: 1, RunID: run.ID, PlanID: "run-plan-integration", PlanRevision: 1, RequirementID: requirement.ID, ExecutionScopeDigest: scopeDigest, ExecutionScope: run.Scope, ActionScopeDigest: integrationDigestV3("settlement-action/" + string(requirement.ID)), PolicyOwner: requirement.Owner, PolicyAuthority: runtimeports.AuthorityBindingRefV2{Ref: "settlement-authority-" + string(requirement.ID[8:]), Digest: integrationDigestV3("settlement-authority/" + string(requirement.ID)), Revision: 1, Epoch: run.Scope.AuthorityEpoch}, UnknownMode: runtimeports.RunUnknownBlock, FailureMode: runtimeports.RunClosedFailureBlock, NotAppliedMode: runtimeports.RunClosedFailureBlock, AllowOperationNotRequired: true, AllowSelfPolicy: true, State: runtimeports.RunSettlementPolicyActive, ExpiresUnixNano: f.now.Add(time.Hour).UnixNano()})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		requirement.Policy = runtimeports.RunSettlementPolicyBindingRefV2{Ref: policy.Ref, Revision: policy.Revision, Digest: policy.Digest, SemanticDigest: policy.SemanticDigest}
		requirements = append(requirements, requirement)
		policies[item.kind] = policy
	}
	runtimeports.SortRunSettlementRequirementsV2(requirements)
	plan := runtimeports.RunSettlementPlanFactV2{ContractVersion: runtimeports.RunSettlementContractVersionV2, ID: "run-plan-integration", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, SessionRef: run.SessionRef, LineagePlanDigest: run.Scope.Lineage.PlanDigest, BindingSet: runtimeports.RunBindingSetRefV2{ID: f.bindings.set.ID, Revision: f.bindings.set.Revision, Digest: setDigest, SemanticDigest: setSemantic}, Execution: execution, Claim: runtimeports.RunClaimRequirementV2{Mode: runtimeports.RunClaimRequiredV2}, Requirements: requirements, CreatedUnixNano: f.now.UnixNano()}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	baseline, err := runtimeports.SealRunSettlementBaselinePolicyFactV3(runtimeports.RunSettlementBaselinePolicyFactV3{ContractVersion: runtimeports.RunSettlementPlanAdmissionContractVersionV3, ID: "run-baseline-integration", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Requirements: append([]runtimeports.RunSettlementRequirementV2{}, requirements...), PolicyOwner: f.bindings.host, ExpiresUnixNano: f.now.Add(40 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	planStore := runtimefakes.NewRunPlanAdmissionStoreV3(func() time.Time { return f.now.Add(3 * time.Second) })
	if _, err := planStore.CreateRunSettlementBaselinePolicyV3(ctx, baseline); err != nil {
		t.Fatal(err)
	}
	for _, member := range f.bindings.set.Members {
		fact := f.bindings.boundByID[member.BindingID]
		declaration, sealErr := runtimeports.SealRunSettlementDeclarationFactV3(runtimeports.RunSettlementDeclarationFactV3{ContractVersion: runtimeports.RunSettlementPlanAdmissionContractVersionV3, ID: "declaration-" + string(member.ComponentID), Revision: 1, BindingSetID: f.bindings.set.ID, BindingSetRevision: f.bindings.set.Revision, BindingRevision: member.BindingRevision, ComponentID: member.ComponentID, BindingID: member.BindingID, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, Requirements: []runtimeports.RunSettlementRequirementV2{}, ExpiresUnixNano: fact.ExpiresUnixNano})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if _, err := planStore.CreateRunSettlementDeclarationV3(ctx, declaration); err != nil {
			t.Fatal(err)
		}
	}
	planGateway := control.RunSettlementPlanAdmissionGatewayV3{Bindings: f.bindings.store, Declarations: planStore, Baselines: planStore, Certifications: planStore, Clock: func() time.Time { return f.now.Add(3 * time.Second) }}
	baselineRef, _ := baseline.RefV3()
	certification, err := planGateway.CertifyRunSettlementPlanV3(ctx, runtimeports.CertifyRunSettlementPlanRequestV3{CertificationID: "certification-integration", Run: run, Plan: plan, BaselinePolicy: baselineRef, Owner: f.bindings.host, TTL: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	certRef, _ := certification.RefV3()
	association, err := runtimeports.NewRunSettlementPlanCertificationAssociationV3(run, plan, certRef)
	if err != nil {
		t.Fatal(err)
	}
	runFacts := runtimefakes.NewRunSettlementStoreV2(func() time.Time { return f.now.Add(5 * time.Second) })
	runEffects := runtimefakes.NewEffectStoreV2(func() time.Time { return f.now.Add(5 * time.Second) })
	runEffects.SetRunFacts(runFacts)
	runGateway := runtimekernel.RunSettlementGatewayV2{Facts: runFacts, Effects: runEffects, PlanAdmissions: planGateway, Clock: func() time.Time { return f.now.Add(5 * time.Second) }}
	runFacts.LoseNextBundleReply()
	lifecycle, err := runGateway.CreatePendingRunV3(ctx, runtimeports.CreatePendingRunRequestV3{Run: run, Plan: plan, Certification: association, EffectIndexID: "run-effects-integration"})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.Phase != runtimeports.RunLifecyclePendingPreparedV3 {
		t.Fatalf("certified pending Run did not converge: %#v", lifecycle)
	}
	running := run
	running.Status, running.Revision, running.StartedAt = core.RunRunning, 2, f.now.Add(4*time.Second)
	running, err = runFacts.CompareAndSwapRun(ctx, control.RunFactCASRequest{ExpectedRevision: 1, Next: running})
	if err != nil {
		t.Fatal(err)
	}
	claimEvidence := newIntegrationClaimEvidenceV3(f.now.Add(5 * time.Second))
	f.evidence.mu.Lock()
	f.evidence.claim = claimEvidence
	f.evidence.mu.Unlock()
	claimStore := runtimefakes.NewRunClaimAssociationStoreV2()
	legacyClaim, err := runtimekernel.NewRunClaimGatewayV2(claimEvidence, claimStore, runFacts, func() time.Time { return f.now.Add(5 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	claimGateway := runtimekernel.RunClaimGatewayV3{Legacy: legacyClaim, Bundles: runFacts, PlanAdmissions: planGateway}
	f.run, f.runPlan, f.runPolicies, f.certification, f.runGateway, f.runFacts, f.runEffects, f.planStore, f.planGateway, f.claimEvidence, f.claimStore, f.claimGateway = running, plan, policies, certification, runGateway, runFacts, runEffects, planStore, planGateway, claimEvidence, claimStore, claimGateway
}

type integrationClaimEvidenceV3 struct {
	mu       sync.Mutex
	now      time.Time
	records  map[string]runtimeports.EvidenceLedgerRecordV2
	next     uint64
	last     core.Digest
	loseNext bool
}

func newIntegrationClaimEvidenceV3(now time.Time) *integrationClaimEvidenceV3 {
	return &integrationClaimEvidenceV3{now: now, records: map[string]runtimeports.EvidenceLedgerRecordV2{}}
}
func (s *integrationClaimEvidenceV3) RegisterGovernedSource(context.Context, runtimeports.EvidenceSourceRegistrationFactV2) (runtimeports.EvidenceSourceRegistrationFactV2, error) {
	panic("unused")
}
func (s *integrationClaimEvidenceV3) RenewGovernedSource(context.Context, runtimeports.EvidenceSourceCASRequestV2) (runtimeports.EvidenceSourceRegistrationFactV2, error) {
	panic("unused")
}
func (s *integrationClaimEvidenceV3) AppendLateGoverned(context.Context, runtimeports.EvidenceAppendLateRequestV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	panic("unused")
}
func integrationClaimKeyV3(candidate runtimeports.EvidenceEventCandidateV2) string {
	return candidate.RegistrationID + "/" + strconv.FormatUint(uint64(candidate.SourceEpoch), 10) + "/" + strconv.FormatUint(candidate.SourceSequence, 10)
}
func (s *integrationClaimEvidenceV3) AppendGoverned(_ context.Context, request runtimeports.EvidenceAppendRequestV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := integrationClaimKeyV3(request.Candidate)
	if existing, ok := s.records[key]; ok {
		digest, _ := request.Candidate.DigestV2()
		if digest == existing.CandidateDigest {
			return existing, nil
		}
		return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "claim source content changed")
	}
	s.next++
	previous := s.last
	if s.next == 1 {
		previous = runtimeports.EvidenceGenesisDigestV2
	}
	record, err := control.NewEvidenceLedgerRecordV2(request.Candidate, s.next, previous, s.now)
	if err != nil {
		return runtimeports.EvidenceLedgerRecordV2{}, err
	}
	s.records[key], s.last = record, record.Ref.RecordDigest
	if s.loseNext {
		s.loseNext = false
		return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected claim append reply loss")
	}
	return record, nil
}
func (s *integrationClaimEvidenceV3) InspectGovernedBySource(_ context.Context, key runtimeports.EvidenceSourceKeyV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key.RegistrationID+"/"+strconv.FormatUint(uint64(key.SourceEpoch), 10)+"/"+strconv.FormatUint(key.SourceSequence, 10)]
	if !ok {
		return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "claim source missing")
	}
	return record, nil
}
func (s *integrationClaimEvidenceV3) InspectGovernedRecord(_ context.Context, ref runtimeports.EvidenceRecordRefV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.Ref == ref {
			return record, nil
		}
	}
	return runtimeports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "claim record missing")
}

func integrationClaimCandidateV3(run core.AgentRunRecord, kind core.RunCompletionClaimKind, sequence uint64) runtimeports.EvidenceEventCandidateV2 {
	schema := runtimeports.SchemaRefV2{Namespace: "runtime", Name: "run-claim", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: integrationDigestV3("claim-schema")}
	return runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: run.Scope.Identity.TenantID, IdentityID: run.Scope.Identity.ID, LineageID: run.Scope.Lineage.ID, InstanceID: run.Scope.Instance.ID, RunID: run.ID}, EventID: "harness-terminal-integration", RegistrationID: "harness-claim-registration", RegistrationRevision: 1, SourceConfigurationDigest: integrationDigestV3("claim-config"), SourcePolicy: runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "claim-policy", Digest: integrationDigestV3("claim-policy"), Revision: 1}, SourceID: "praxis.harness/run-claim", SourceEpoch: run.Scope.Instance.Epoch, SourceSequence: sequence, TrustClass: runtimeports.EvidenceTrustClaim, ClaimKind: kind, EventKind: "runtime/run-completion", CustomClass: "runtime/claim", ExecutionScope: run.Scope, Payload: runtimeports.EvidencePayloadRefV2{Schema: schema, ContentDigest: integrationDigestV3("terminal-claim"), Revision: 1, Length: 1, Ref: "memory://terminal-claim"}, Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: string(run.ID), Producer: runtimeports.EvidenceProducerBindingRefV2{BindingSetID: "claim-binding-integration", BindingSetRevision: 1, ComponentID: "praxis.harness/claim-source", ManifestDigest: integrationDigestV3("claim-manifest"), ArtifactDigest: integrationDigestV3("claim-artifact"), Capability: "runtime/claim"}, Authority: runtimeports.AuthorityBindingRefV2{Ref: "claim-authority", Digest: integrationDigestV3("claim-authority"), Revision: 1, Epoch: run.Scope.AuthorityEpoch}, ObservedUnixNano: fTimeAfterStartV3(run)}
}

func fTimeAfterStartV3(run core.AgentRunRecord) int64 {
	return run.StartedAt.Add(time.Nanosecond).UnixNano()
}
