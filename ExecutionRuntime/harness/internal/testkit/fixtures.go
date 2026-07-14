package testkit

import (
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func Digest(value any) core.Digest {
	digest, err := core.DigestJSON(value)
	if err != nil {
		panic(err)
	}
	return digest
}

func Payload(schema string, value any) runtimeports.OpaquePayload {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return runtimeports.OpaquePayload{Schema: schema, Digest: Digest(value), Payload: encoded}
}

func Scope(planDigest core.Digest) core.ExecutionScope {
	return core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance:     core.InstanceRef{ID: "instance-1", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1,
	}
}

func Manifest(now time.Time, conformance runtimeports.ConformanceLevel) contract.Manifest {
	planDigest := Digest("resolved-plan")
	return contract.Manifest{
		ContractVersion: contract.Version, ID: "praxis/harness-test", Version: "0.1.0",
		ArtifactDigest: Digest("harness-artifact"), Conformance: conformance,
		Bootstrap: contract.BootstrapPlan{
			ID: "bootstrap-1", Version: "1", ResolvedPlanDigest: planDigest,
			ProfileDigest: Digest("profile"), RuntimePolicyDigest: Digest("runtime-policy"),
			HarnessStackDigest: Digest("harness-stack"), SemanticRouteDigest: Digest("semantic-route"),
			ExpectedInjectionManifestDigest: Digest("injection-manifest"), ContextPlanDigest: Digest("context-plan"),
			ToolSurfaceDigest: Digest("tool-surface"), CapabilityGrantDigest: Digest("capability-grant"),
			MinimumConformance: conformance,
			Controls:           contract.ControlCapabilities{Cancel: true, ProvideInput: true, ProvideActionResult: true},
			EvidenceExpiresAt:  now.Add(2 * time.Hour),
		},
		Capabilities: []string{"interaction_loop"}, OpaqueBoundaries: []string{"provider_session"},
		EvidenceDigest: Digest("harness-evidence"), EvidenceExpiresAt: now.Add(2 * time.Hour),
	}
}

func BindingManifest(manifest contract.Manifest) runtimeports.ComponentManifestV2 {
	return runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2,
		ComponentID:     runtimeports.ComponentIDV2(manifest.ID), Kind: "praxis/harness-adapter", GovernanceCategory: "praxis/execution",
		SemanticVersion: manifest.Version, ArtifactDigest: manifest.ArtifactDigest,
		Contract: runtimeports.ContractBindingV2{Name: "praxis/harness-execution", Version: "2.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}},
		Schemas:  []runtimeports.SchemaRefV2{}, Locality: runtimeports.LocalityHostControlPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{},
		ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: "praxis/harness-execution", TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{}}},
		Conformance:          manifest.Conformance, ResidualClass: runtimeports.ResidualInspectable,
		Owners:      []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: runtimeports.ComponentIDV2(manifest.ID)}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: runtimeports.ComponentIDV2(manifest.ID)}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: runtimeports.ComponentIDV2(manifest.ID)}},
		Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied,
		Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
}

func HarnessCatalog() runtimeports.GovernanceCatalogV2 {
	return runtimeports.GovernanceCatalogV2{Registrations: []runtimeports.GovernanceRegistrationV2{{
		Kind: "praxis/harness-adapter", Category: "praxis/execution",
		Capabilities: []runtimeports.CapabilityNameV2{"praxis/harness-execution"}, Schemas: []runtimeports.SchemaRefV2{}, ExtensionPolicies: []runtimeports.ExtensionPolicyV2{},
		AllowedLocalities:  []runtimeports.LocalityV2{runtimeports.LocalityHostControlPlane, runtimeports.LocalityInstanceDataPlane, runtimeports.LocalityRemoteProvider},
		AllowedConformance: []runtimeports.ConformanceLevel{runtimeports.ConformanceFullyControlled, runtimeports.ConformanceRestrictedControlled},
	}}}
}

func Context(now time.Time) ports.ContextSnapshot {
	return ports.ContextSnapshot{
		Ref: "context-1", Payload: Payload("test.context/v1", map[string]string{"context": "ready"}),
		EvidenceDigest: Digest("context-evidence"), ObservedAt: now,
	}
}

func IntentFence(now time.Time, scope core.ExecutionScope, capabilityDigest core.Digest, id string) (core.EffectIntent, core.ExecutionFence) {
	payloadDigest := Digest(id)
	intent := core.EffectIntent{
		ID: core.EffectIntentID("intent-" + id), Revision: 1, Kind: core.EffectKindHostedExecution,
		RiskClass: "test", CanonicalPayloadDigest: payloadDigest, Target: id,
		ConflictEffectDomain: "harness/test", Ownership: core.EffectOwnership{
			IntentOwner: core.OwnerRef{Domain: "runtime", ID: "test"}, SettlementOwner: core.OwnerRef{Domain: "runtime", ID: "test"},
		},
		AuthorizationRef: "authorization-1", IdempotencyClass: core.IdempotencyQueryable, PersistedAt: now.Add(-time.Minute),
	}
	fence := core.ExecutionFence{
		BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: capabilityDigest,
		EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: payloadDigest,
		ExpiresAt: now.Add(time.Hour),
	}
	return intent, fence
}

func GovernedFactsV2(now time.Time) (contract.GovernedSessionV2, contract.ModelTurnCandidateV2) {
	scope := Scope(Digest("resolved-plan"))
	harnessBinding := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-governed", BindingSetRevision: 1, ComponentID: "custom/combined", ManifestDigest: Digest("governed-manifest"), ArtifactDigest: Digest("governed-artifact"), Capability: "praxis/harness-execution"}
	endpoint, err := contract.NewEndpointRefV2("endpoint-governed", scope, harnessBinding)
	if err != nil {
		panic(err)
	}
	run := contract.RunRef{Scope: scope, RunID: "run-governed"}
	session := contract.GovernedSessionV2{ContractVersion: contract.GovernedContractVersionV2, ID: "session-governed", Revision: 1, Run: run, Endpoint: endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	payload := []byte(`{"input":"governed"}`)
	provider := harnessBinding
	provider.Capability = "praxis/model-turn"
	candidate := contract.ModelTurnCandidateV2{ContractVersion: contract.GovernedContractVersionV2, ID: "candidate-governed", Revision: 1, Run: run, Endpoint: endpoint, SessionRef: session.ID, ExpectedSessionRevision: 1, Turn: 1, Kind: contract.CandidateInitialTurnV2, Input: runtimeports.OpaquePayloadV2{Schema: runtimeports.SchemaRefV2{Namespace: "praxis", Name: "model-input", Version: "2.0.0", MediaType: "application/json", ContentDigest: Digest("governed-schema")}, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis/default-limit", Digest: Digest("governed-limit")}}, ContextRef: "context-governed", ContextDigest: Digest("governed-context"), Provider: provider, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	return session, candidate
}

// GovernedAttemptRefsV2 returns structurally exact public Runtime refs for
// Harness state-machine tests. They are fixtures only and grant no authority.
func GovernedAttemptRefsV2(now time.Time, candidate contract.ModelTurnCandidateV2, state runtimeports.ProviderAttemptStateV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	operationDigest := Digest("operation-subject")
	intentDigest := Digest("operation-intent")
	permitDigest := Digest("operation-permit")
	declared := runtimeports.ExecutionDelegationRefV2{ID: "delegation-governed", Revision: 1, Digest: Digest("delegation-declared")}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, "permit-governed", "attempt-governed")
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: operationDigest,
		IntentID: "effect-governed", IntentRevision: 1, IntentDigest: intentDigest,
		PermitID: "permit-governed", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-governed",
		Provider: candidate.Provider, PayloadSchema: candidate.Input.Schema, PayloadDigest: candidate.Input.ContentDigest, PayloadRevision: 1,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: declared.ID, Revision: 2, Digest: Digest("delegation-prepared")}
	result := runtimeports.GovernedExecutionAttemptRefsV2{
		Admission: runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: "effect-governed", IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 2, State: "accepted"},
		PermitID:  "permit-governed", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-governed",
		Delegation: delegation, Prepared: prepared,
		Enforcement: runtimeports.PersistedOperationEnforcementRefV3{PermitID: "permit-governed", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-governed", OperationDigest: operationDigest, Provider: candidate.Provider, ReceiptDigest: Digest("enforcement-receipt"), RecordedRevision: 3},
	}
	if state != "" {
		result.Observation = &runtimeports.ProviderAttemptObservationRefV2{
			Delegation: delegation, PreparedAttemptID: prepared.ID, ProviderOperationRef: "provider-operation-governed", Revision: 1, State: state,
			Digest: Digest("provider-observation"), PayloadDigest: candidate.Input.ContentDigest, PayloadRevision: 1,
			SourceRegistrationID: "provider-source-governed", SourceEpoch: 1, SourceSequence: 1,
			Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: Digest("evidence-scope"), Sequence: 1, RecordDigest: Digest("evidence-record")}, ObservedUnixNano: now.Add(time.Second).UnixNano(),
		}
	}
	if err := result.ValidatePrepared(); err != nil {
		panic(err)
	}
	return result
}

func GovernedSettledAttemptRefsV2(now time.Time, candidate contract.ModelTurnCandidateV2, turn contract.SettledTurnResultV2, disposition runtimeports.OperationSettlementDispositionV3) (runtimeports.GovernedExecutionAttemptRefsV2, runtimeports.OpaquePayloadV2) {
	attempt := GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	domainResult, err := contract.NewSettledTurnDomainResultV2(turn)
	if err != nil {
		panic(err)
	}
	delegation := attempt.Delegation
	observation := *attempt.Observation
	attempt.Settlement = &runtimeports.OperationSettlementRefV3{
		ID: "settlement-governed", Revision: 1, Digest: Digest("operation-settlement"),
		Attempt: runtimeports.OperationDispatchAttemptRefV3{
			OperationDigest: attempt.Admission.OperationDigest, EffectID: attempt.Admission.EffectID,
			IntentRevision: attempt.Admission.IntentRevision, IntentDigest: attempt.Admission.IntentDigest,
			PermitID: attempt.PermitID, PermitRevision: attempt.PermitRevision, PermitDigest: attempt.PermitDigest,
			AttemptID: attempt.AttemptID, Delegation: &delegation,
		},
		Disposition:        disposition,
		Owner:              runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "custom.model/settlement-owner", ManifestDigest: Digest("settlement-owner-manifest")},
		Observation:        &observation,
		Evidence:           []runtimeports.EvidenceRecordRefV2{observation.Evidence},
		DomainResultSchema: &domainResult.Schema, DomainResultDigest: domainResult.ContentDigest,
	}
	if err := attempt.ValidatePrepared(); err != nil {
		panic(err)
	}
	return attempt, domainResult
}

// AddUnknownInspectProvenanceV2 converts an exact post-prepared Settlement
// fixture into the independently inspected unknown-outcome form required by
// Runtime. It never treats the original provider as its own inspector.
func AddUnknownInspectProvenanceV2(attempt *runtimeports.GovernedExecutionAttemptRefsV2) {
	if attempt == nil || attempt.Settlement == nil {
		panic("settled Runtime attempt is required")
	}
	inspectionAttempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: Digest("inspect-operation"), EffectID: "inspect-effect-governed", IntentRevision: 1,
		IntentDigest: Digest("inspect-intent"), PermitID: "inspect-permit-governed", PermitRevision: 1,
		PermitDigest: Digest("inspect-permit"), AttemptID: "inspect-attempt-governed",
	}
	inspectionSettlement := runtimeports.OperationSettlementRefV3{
		ID: "inspect-settlement-governed", Revision: 1, Digest: Digest("inspect-settlement"), Attempt: inspectionAttempt,
		Disposition: runtimeports.OperationSettlementAppliedV3,
		Owner:       runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "custom.inspect/settlement-owner", ManifestDigest: Digest("inspect-owner-manifest")},
		Evidence:    []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: Digest("inspect-ledger"), Sequence: 1, RecordDigest: Digest("inspect-record")}},
	}
	inspectionRef, err := inspectionSettlement.InspectionRefV3()
	if err != nil {
		panic(err)
	}
	attempt.Observation = nil
	attempt.Settlement.Observation = nil
	attempt.Settlement.InspectionEffect = &inspectionAttempt
	attempt.Settlement.InspectionSettlement = &inspectionRef
	if err := attempt.ValidatePrepared(); err != nil {
		panic(err)
	}
}
