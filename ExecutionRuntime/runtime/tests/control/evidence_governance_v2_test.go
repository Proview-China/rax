package control_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceGovernanceV2RegistersAndAppendsOnlyPolicyGrantedTrust(t *testing.T) {
	fixture := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionInstance, ports.EvidenceTrustObservation)
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), fixture.source); err != nil {
		t.Fatal(err)
	}
	candidate := governedEvidenceCandidateV2(t, fixture.source, "event-governed", 1, ports.EvidenceTrustObservation)
	record, err := fixture.gateway.AppendGoverned(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1})
	if err != nil || record.Ref.Sequence != 1 {
		t.Fatalf("governed observation append failed: %v %+v", err, record)
	}
	selfGranted := fixture.source
	selfGranted.ID = "source-self-grant"
	selfGranted.SourceEpoch++
	selfGranted.ClassMappings = []ports.EvidenceClassMappingV2{{Class: "custom/observation", Trust: ports.EvidenceTrustClaim}}
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), selfGranted); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("binding+authority cannot self-grant claim trust: %v", err)
	}
}

func TestEvidenceGovernanceV2PolicyDriftAndRawFactInspectUnavailableFailClosed(t *testing.T) {
	fixture := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionInstance, ports.EvidenceTrustObservation)
	unavailable := &unavailableEvidenceFactPortV2{EvidenceLedgerFactPortV2: fixture.ledger, inspectSource: true}
	fixture.gateway.Ledger = unavailable
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), fixture.source); !core.HasCategory(err, core.ErrorUnavailable) || unavailable.createCalls != 0 {
		t.Fatalf("unavailable idempotency inspect must not redispatch Create: %v calls=%d", err, unavailable.createCalls)
	}
	fixture.gateway.Ledger = fixture.ledger
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), fixture.source); err != nil {
		t.Fatal(err)
	}
	fixture.policy.fact.State = ports.EvidenceSourcePolicyRevoked
	candidate := governedEvidenceCandidateV2(t, fixture.source, "event-policy-revoked", 1, ports.EvidenceTrustObservation)
	if _, err := fixture.gateway.AppendGoverned(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1}); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("revoked independent policy must block append: %v", err)
	}
}

func TestEvidenceSourceRenewKeepsGovernanceIdentityAndAllowsRepeatedTTLWindows(t *testing.T) {
	fixture := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionInstance, ports.EvidenceTrustObservation)
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), fixture.source); err != nil {
		t.Fatal(err)
	}
	current := fixture.source
	for range 3 {
		fixture.now = fixture.now.Add(10 * time.Second)
		next := current
		next.Revision++
		next.UpdatedUnixNano = fixture.now.UnixNano()
		next.ExpiresUnixNano = fixture.now.Add(30 * time.Second).UnixNano()
		renewed, err := fixture.gateway.RenewGovernedSource(context.Background(), ports.EvidenceSourceCASRequestV2{ExpectedRevision: current.Revision, Next: next})
		if err != nil {
			t.Fatal(err)
		}
		current = renewed
	}
	drift := current
	drift.Revision++
	drift.UpdatedUnixNano = fixture.now.UnixNano()
	drift.ExpiresUnixNano = fixture.now.Add(35 * time.Second).UnixNano()
	drift.ActionScopeDigest = controlEffectDigestV2(t, "drifted-action")
	if _, err := fixture.ledger.CompareAndSwapSource(context.Background(), ports.EvidenceSourceCASRequestV2{ExpectedRevision: current.Revision, Next: drift}); !core.HasReason(err, core.ReasonEvidenceSourceStale) {
		t.Fatalf("renew cannot replace action scope: %v", err)
	}
}

func TestEvidenceSourceRenewAcceptsExactCurrentScopeWatermarkAdvance(t *testing.T) {
	fixture := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionInstance, ports.EvidenceTrustObservation)
	if _, err := fixture.gateway.RegisterGovernedSource(context.Background(), fixture.source); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	fixture.current.fact.Revision++
	fixture.current.fact.ProjectionWatermark++
	fixture.current.redigest(t)
	next := fixture.source
	next.Revision++
	next.UpdatedUnixNano = fixture.now.UnixNano()
	next.ExpiresUnixNano = fixture.now.Add(30 * time.Second).UnixNano()
	next.CurrentScope, _ = fixture.current.fact.BindingRefV2()
	next.CurrentScopeWatermark = fixture.current.fact.ProjectionWatermark
	if _, err := fixture.gateway.RenewGovernedSource(context.Background(), ports.EvidenceSourceCASRequestV2{ExpectedRevision: fixture.source.Revision, Next: next}); err != nil {
		t.Fatal(err)
	}
	sameRevisionDrift := next
	sameRevisionDrift.Revision++
	sameRevisionDrift.UpdatedUnixNano = fixture.now.UnixNano()
	sameRevisionDrift.ExpiresUnixNano = fixture.now.Add(31 * time.Second).UnixNano()
	sameRevisionDrift.CurrentScope.Digest = controlEffectDigestV2(t, "same-revision-drift")
	if err := control.ValidateEvidenceSourceTransitionV2(next, sameRevisionDrift, fixture.now); !core.HasReason(err, core.ReasonEvidenceSourceStale) {
		t.Fatalf("same current-scope revision cannot change digest: %v", err)
	}
}

func TestEvidenceClaimPolicyControlsInstanceEpochRequirement(t *testing.T) {
	strict := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionRun, ports.EvidenceTrustClaim)
	strict.policy.fact.RequireInstanceEpoch = true
	strict.policy.redigest(t)
	strict.sourcePolicyRef(&strict.source)
	strict.source.SourceEpoch = 9
	if _, err := strict.gateway.RegisterGovernedSource(context.Background(), strict.source); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("policy requiring instance epoch must reject independent source epoch: %v", err)
	}
	independent := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionRun, ports.EvidenceTrustClaim)
	independent.source.SourceEpoch = 9
	if _, err := independent.gateway.RegisterGovernedSource(context.Background(), independent.source); err != nil {
		t.Fatal(err)
	}
	candidate := governedEvidenceCandidateV2(t, independent.source, "event-independent-epoch", 1, ports.EvidenceTrustClaim)
	if _, err := independent.gateway.AppendGoverned(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1}); err != nil {
		t.Fatalf("policy may explicitly allow independent source epoch: %v", err)
	}
}

type evidenceGovernanceFixtureV2 struct {
	now       time.Time
	ledger    *fakes.EvidenceLedgerStoreV2
	runs      *fakes.FactStore
	source    ports.EvidenceSourceRegistrationFactV2
	policy    *mutableEvidencePolicyReaderV2
	authority *mutableEvidenceAuthorityReaderV2
	current   *mutableEvidenceScopeReaderV2
	binding   *mutableEvidenceBindingPortV2
	gateway   control.EvidenceGovernanceGatewayV2
}

func newEvidenceGovernanceFixtureV2(t *testing.T, partition ports.EvidencePartitionV2, trust ports.EvidenceTrustClassV2) *evidenceGovernanceFixtureV2 {
	t.Helper()
	f := &evidenceGovernanceFixtureV2{now: time.Unix(100_000, 0)}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-evidence", ID: "identity-evidence", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-evidence", PlanDigest: controlEffectDigestV2(t, "evidence-plan")}, Instance: core.InstanceRef{ID: "instance-evidence", Epoch: 1}, AuthorityEpoch: 1}
	runID := core.AgentRunID("run-evidence")
	ledgerScope := ports.EvidenceLedgerScopeV2{Partition: partition, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID}
	if partition == ports.EvidencePartitionRun || partition == ports.EvidencePartitionEffect {
		ledgerScope.RunID = runID
	}
	if partition == ports.EvidencePartitionEffect {
		ledgerScope.EffectID = "effect-evidence"
	}
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "evidence-binding-set", BindingSetRevision: 1, ComponentID: "custom/evidence-producer", ManifestDigest: controlEffectDigestV2(t, "evidence-manifest"), ArtifactDigest: controlEffectDigestV2(t, "evidence-artifact"), Capability: "runtime/evidence-append"}
	grant := ports.CapabilityGrantV2{Capability: producer.Capability, EvidenceDigest: controlEffectDigestV2(t, "evidence-grant"), ObservedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()}
	owners := []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: producer.ComponentID}, {Role: ports.OwnerSettlement, OwnerComponentID: producer.ComponentID}, {Role: ports.OwnerCleanup, OwnerComponentID: producer.ComponentID}}
	set := control.BindingSetFactV2{ID: producer.BindingSetID, PlanID: "evidence-plan", PlanDigest: controlEffectDigestV2(t, "binding-plan"), GovernanceDigest: controlEffectDigestV2(t, "binding-governance"), State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: "binding-evidence", BindingRevision: 1, ComponentID: producer.ComponentID, Kind: "custom/evidence-source", ManifestDigest: producer.ManifestDigest, ArtifactDigest: producer.ArtifactDigest, Contract: ports.ContractBindingV2{Name: "custom/evidence-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Owners: owners, Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{producer.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()}
	f.binding = &mutableEvidenceBindingPortV2{set: set}
	capability, _ := set.CapabilityGrantDigestV2()
	sourceRef := ports.GovernanceSourceFactRefV2{Ref: "source-watermark", Revision: 1, Digest: controlEffectDigestV2(t, "source-watermark")}
	current := ports.ExecutionScopeCurrentFactV2{Ref: "evidence-current-scope", Revision: 1, Scope: scope, CapabilityGrantDigest: capability, ActivationSource: sourceRef, InstanceSource: sourceRef, AuthoritySource: sourceRef, BindingSource: sourceRef, RunSource: sourceRef, ActiveRunID: runID, RunState: string(core.RunRunning), ProjectionWatermark: 1, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()}
	current.Digest, _ = current.DigestV2()
	f.current = &mutableEvidenceScopeReaderV2{fact: current}
	authorityRef := ports.AuthorityBindingRefV2{Ref: "evidence-authority", Digest: controlEffectDigestV2(t, "authority-digest"), Revision: 1, Epoch: scope.AuthorityEpoch}
	authority := ports.DispatchAuthorityFactV2{Ref: authorityRef.Ref, Digest: authorityRef.Digest, Revision: authorityRef.Revision, Scope: scope, ActionScopeDigest: controlEffectDigestV2(t, "evidence-action"), State: ports.AuthorityFactActive, ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()}
	f.authority = &mutableEvidenceAuthorityReaderV2{fact: authority}
	mapping := ports.EvidenceClassMappingV2{Class: "custom/observation", Trust: trust}
	policy := ports.EvidenceSourcePolicyFactV2{Ref: "evidence-source-policy", Revision: 1, Producer: producer, PolicyOwner: producer, PolicyAuthority: authorityRef, PolicyScope: scope, ActionScopeDigest: authority.ActionScopeDigest, AllowedPartitions: []ports.EvidencePartitionV2{partition}, ClassMappings: []ports.EvidenceClassMappingV2{mapping}, AllowedKinds: []ports.NamespacedNameV2{"custom/event"}, OwnerFactRules: []ports.EvidenceOwnerFactRuleV2{}, ClaimKinds: []ports.EvidenceClaimKindMappingV2{}, MaximumSourceTTL: 30 * time.Second, State: ports.EvidenceSourcePolicyActive, ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()}
	if trust == ports.EvidenceTrustClaim {
		policy.ClaimKinds = []ports.EvidenceClaimKindMappingV2{{EventKind: "custom/event", CustomClass: "custom/observation", ClaimKind: core.RunClaimCompleted}}
	}
	policy.Digest, _ = policy.DigestV2()
	f.policy = &mutableEvidencePolicyReaderV2{fact: policy}
	currentBinding, _ := current.BindingRefV2()
	f.source = ports.EvidenceSourceRegistrationFactV2{ContractVersion: ports.EvidenceContractVersionV2, ID: "evidence-registration", Revision: 1, SourceID: "custom/source", SourceEpoch: 1, LedgerScope: ledgerScope, ExecutionScope: scope, CurrentScope: currentBinding, Producer: producer, Authority: authorityRef, ActionScopeDigest: authority.ActionScopeDigest, Policy: ports.EvidenceSourcePolicyBindingRefV2{Ref: policy.Ref, Digest: policy.Digest, Revision: policy.Revision}, ClassMappings: []ports.EvidenceClassMappingV2{mapping}, AllowedKinds: []ports.NamespacedNameV2{"custom/event"}, GapPolicy: ports.EvidenceGapStrictV2, NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: f.now.UnixNano(), UpdatedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(30 * time.Second).UnixNano()}
	f.source.CurrentScopeWatermark = current.ProjectionWatermark
	f.ledger = fakes.NewEvidenceLedgerStoreV2(func() time.Time { return f.now })
	f.runs = fakes.NewFactStore(func() time.Time { return f.now })
	_, _ = f.runs.CreateRun(context.Background(), core.AgentRunRecord{ID: runID, Scope: scope, Status: core.RunRunning, Revision: 1, SessionRef: "session-evidence", StartedAt: f.now.Add(-time.Second)})
	f.gateway = control.EvidenceGovernanceGatewayV2{Ledger: f.ledger, Bindings: f.binding, Authority: f.authority, CurrentScopes: f.current, Policies: f.policy, Runs: f.runs, Effects: fakes.NewEffectStoreV2(func() time.Time { return f.now }), Clock: func() time.Time { return f.now }}
	return f
}

func (f *evidenceGovernanceFixtureV2) sourcePolicyRef(source *ports.EvidenceSourceRegistrationFactV2) {
	source.Policy = ports.EvidenceSourcePolicyBindingRefV2{Ref: f.policy.fact.Ref, Digest: f.policy.fact.Digest, Revision: f.policy.fact.Revision}
}
func governedEvidenceCandidateV2(t *testing.T, source ports.EvidenceSourceRegistrationFactV2, event string, sequence uint64, trust ports.EvidenceTrustClassV2) ports.EvidenceEventCandidateV2 {
	t.Helper()
	configuration, _ := source.ConfigurationDigestV2()
	payload := controlEffectDigestV2(t, event+"-payload")
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: source.LedgerScope, EventID: event, RegistrationID: source.ID, RegistrationRevision: source.Revision, SourceConfigurationDigest: configuration, SourcePolicy: source.Policy, SourceID: source.SourceID, SourceEpoch: source.SourceEpoch, SourceSequence: sequence, TrustClass: trust, EventKind: "custom/event", CustomClass: "custom/observation", ExecutionScope: source.ExecutionScope, Payload: ports.EvidencePayloadRefV2{Schema: ports.SchemaRefV2{Namespace: "custom", Name: "evidence", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: controlEffectDigestV2(t, "evidence-schema")}, ContentDigest: payload, Revision: 1, Length: 1, Ref: "memory://" + event}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "correlation-evidence", Producer: source.Producer, Authority: source.Authority, ObservedUnixNano: source.UpdatedUnixNano}
	if trust == ports.EvidenceTrustClaim {
		candidate.ClaimKind = core.RunClaimCompleted
	}
	return candidate
}

type mutableEvidencePolicyReaderV2 struct {
	fact ports.EvidenceSourcePolicyFactV2
}

func (r *mutableEvidencePolicyReaderV2) InspectEvidenceSourcePolicy(context.Context, string) (ports.EvidenceSourcePolicyFactV2, error) {
	return r.fact, nil
}
func (r *mutableEvidencePolicyReaderV2) redigest(t *testing.T) {
	t.Helper()
	r.fact.Digest = ""
	r.fact.Digest, _ = r.fact.DigestV2()
}

type mutableEvidenceAuthorityReaderV2 struct{ fact ports.DispatchAuthorityFactV2 }

func (r *mutableEvidenceAuthorityReaderV2) InspectDispatchAuthority(context.Context, string) (ports.DispatchAuthorityFactV2, error) {
	return r.fact, nil
}

type mutableEvidenceScopeReaderV2 struct {
	fact ports.ExecutionScopeCurrentFactV2
}

func (r *mutableEvidenceScopeReaderV2) InspectCurrentExecutionScope(context.Context, string) (ports.ExecutionScopeCurrentFactV2, error) {
	return r.fact, nil
}
func (r *mutableEvidenceScopeReaderV2) redigest(t *testing.T) {
	t.Helper()
	r.fact.Digest = ""
	r.fact.Digest, _ = r.fact.DigestV2()
}

type mutableEvidenceBindingPortV2 struct {
	staticBindingPortV2
	set control.BindingSetFactV2
}

func (r *mutableEvidenceBindingPortV2) InspectBindingSet(context.Context, string) (control.BindingSetFactV2, error) {
	return r.set, nil
}

type unavailableEvidenceFactPortV2 struct {
	ports.EvidenceLedgerFactPortV2
	inspectSource bool
	createCalls   int
}

func (r *unavailableEvidenceFactPortV2) InspectSource(context.Context, string) (ports.EvidenceSourceRegistrationFactV2, error) {
	return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected inspect unavailable")
}
func (r *unavailableEvidenceFactPortV2) CreateSource(ctx context.Context, f ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	r.createCalls++
	return r.EvidenceLedgerFactPortV2.CreateSource(ctx, f)
}
