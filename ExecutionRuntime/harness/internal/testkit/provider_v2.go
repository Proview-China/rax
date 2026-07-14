package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedProviderFixtureV2 builds one structurally exact custom-provider
// chain for Harness black-box and conformance tests. It grants no authority.
func GovernedProviderFixtureV2(now time.Time) (runtimeports.PrepareGovernedExecutionRequestV2, runtimeports.ProviderPreparationAttestationV2, runtimeports.ExecutePreparedRequestV2, runtimeports.ProviderAttemptObservationV2) {
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-provider", ID: "identity-provider", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage-provider", PlanDigest: Digest("provider-lineage")},
		Instance:     core.InstanceRef{ID: "instance-provider", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-provider", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		panic(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		RunID: "run-provider", SubjectRevision: 1, CurrentProjectionRef: "current-provider",
		CurrentProjectionDigest: Digest("provider-current"), CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		panic(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-provider", BindingSetRevision: 1, ComponentID: "custom.provider/model",
		ManifestDigest: Digest("provider-manifest"), ArtifactDigest: Digest("provider-artifact"), Capability: "custom.provider/execute",
	}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-provider", Digest: Digest("provider-authority"), Revision: 1, Epoch: 1}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "review-case-provider", CandidateDigest: Digest("provider-candidate"), CandidateRevision: 1, PolicyDigest: Digest("provider-review-policy")}
	budget := runtimeports.OperationBudgetBindingRefV3{Ref: "budget-provider", Digest: Digest("provider-budget"), Revision: 1, PolicyDigest: Digest("provider-budget-policy"), SubjectDigest: operationDigest}
	policy := runtimeports.OperationPolicyBindingRefV3{Ref: "policy-provider", Digest: Digest("provider-policy"), Revision: 1, SubjectDigest: operationDigest}
	payload := providerOpaqueV2("provider-input")
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "effect-provider", Revision: 1,
		Operation: operation, Kind: "custom.provider/model", RiskClass: "custom.provider/controlled",
		ActionScopeDigest: Digest("provider-action-scope"), Payload: payload, PayloadRevision: 1, Target: "provider/model",
		ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "custom.provider/model", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID)},
		Owners: []runtimeports.EffectOwnerRefV2{
			{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		},
		Provider: provider, Authority: authority, Review: review, Budget: budget, Policy: policy,
		Idempotency:      runtimeports.IdempotencyBindingV2{Key: "provider-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: core.IdempotencyQueryable},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		panic(err)
	}
	fact := func(ref string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: ref, Revision: 1, Digest: Digest(ref), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	}
	reviewAuthorization := runtimeports.OperationReviewAuthorizationV3{
		Case: fact(review.CaseRef), CandidateDigest: review.CandidateDigest, CandidateRevision: review.CandidateRevision,
		Verdict: fact("verdict-provider"), ReviewerAuthority: fact("reviewer-provider"), PolicyDigest: review.PolicyDigest,
		ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	permit := runtimeports.OperationDispatchPermitV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "permit-provider", Revision: 1, AttemptID: "attempt-provider",
		IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, Operation: operation,
		PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1,
		ConflictDomain: intent.ConflictDomain, Provider: provider, EnforcementPoint: provider, Authority: authority,
		Review: review, ReviewAuthorization: reviewAuthorization, Budget: budget, Policy: policy,
		CapabilityGrantDigest: Digest("provider-capability"), CredentialGrantDigest: Digest("provider-credentials"), GovernanceSnapshotDigest: Digest("provider-snapshot"),
		Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	fence := core.ExecutionFence{
		BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: permit.CapabilityGrantDigest,
		EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: payload.ContentDigest,
		ExpiresAt: time.Unix(0, permit.ExpiresUnixNano),
	}
	permit.FenceDigest, err = runtimeports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		panic(err)
	}
	declared := runtimeports.ExecutionDelegationRefV2{ID: "delegation-provider", Revision: 1, Digest: Digest("provider-delegation-declared")}
	prepare := runtimeports.PrepareGovernedExecutionRequestV2{Delegation: declared, Intent: intent, Permit: permit, Fence: fence}
	permitDigest, err := permit.DigestV3()
	if err != nil {
		panic(err)
	}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, permit.ID, permit.AttemptID)
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: operationDigest,
		IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PermitID: permit.ID, PermitRevision: permit.Revision, PermitDigest: permitDigest, AttemptID: permit.AttemptID,
		Provider: provider, PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: permit.ExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	attestation := runtimeports.ProviderPreparationAttestationV2{
		ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: declared, Prepared: prepared,
		Enforcement: runtimeports.OperationEnforcementReceiptV3{
			ContractVersion: runtimeports.OperationEffectContractVersionV3, PermitID: permit.ID, PermitRevision: permit.Revision,
			AttemptID: permit.AttemptID, PermitDigest: permitDigest, Operation: operation, Verifier: provider, ValidatedUnixNano: now.UnixNano(),
		},
		ObservedUnixNano: now.UnixNano(),
	}
	preparedDelegation := runtimeports.ExecutionDelegationRefV2{ID: declared.ID, Revision: 2, Digest: Digest("provider-delegation-prepared")}
	enforcement := runtimeports.PersistedOperationEnforcementRefV3{
		PermitID: permit.ID, PermitRevision: permit.Revision, PermitDigest: permitDigest, AttemptID: permit.AttemptID,
		OperationDigest: operationDigest, Provider: provider, ReceiptDigest: Digest("provider-enforcement"), RecordedRevision: 3,
	}
	execute := runtimeports.ExecutePreparedRequestV2{Delegation: preparedDelegation, Prepared: prepared, Enforcement: enforcement, Intent: intent, Permit: permit, Fence: fence}
	resultPayload := providerOpaqueV2("provider-result")
	observation := runtimeports.ProviderAttemptObservationV2{
		ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: preparedDelegation, Prepared: prepared,
		Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Payload: resultPayload, PayloadRevision: 1,
		ProviderOperationRef: "provider-operation-provider", SourceRegistrationID: "provider-source-provider", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: Digest("provider-ledger-scope"), Sequence: 1, RecordDigest: Digest("provider-record")},
		ObservedUnixNano: now.Add(time.Nanosecond).UnixNano(),
	}
	for _, validate := range []func() error{prepare.Validate, attestation.Validate, execute.Validate, observation.Validate} {
		if err := validate(); err != nil {
			panic(err)
		}
	}
	return prepare, attestation, execute, observation
}

func providerOpaqueV2(value string) runtimeports.OpaquePayloadV2 {
	content := []byte(value)
	return runtimeports.OpaquePayloadV2{
		Schema:        runtimeports.SchemaRefV2{Namespace: "custom.provider", Name: "payload", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: Digest("provider-schema")},
		ContentDigest: core.DigestBytes(content), Length: uint64(len(content)), Inline: content,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom.provider/limit", Digest: Digest("provider-limit")},
	}
}
