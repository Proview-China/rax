package testkit

// This file contains a test-only, fully sealed Runtime/Sandbox closure for the
// post-actual V2 capability tests. Production packages must never import
// internal/testkit.

import (
	"context"
	"encoding/json"
	"time"

	runtimecontrol "github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceReadPostActualFixtureV2 contains only authoritative owner shapes.
// It does not expose a repository writer or a shortcut around the owner
// capability constructors.
type WorkspaceReadPostActualFixtureV2 struct {
	Now            time.Time
	Publication    WorkspaceReadCommandPublicationFixtureV2
	RuntimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4
}

type WorkspaceReadPostActualQueryBaseV2 struct {
	Query       sandboxports.WorkspaceReadCurrentQueryV1
	Association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	Domain      runtimeports.OperationDomainCommandRefV1
}

func WorkspaceReadPostActualV2(now time.Time, suffix string) WorkspaceReadPostActualFixtureV2 {
	workspaceID := "workspace-post-actual-" + suffix
	workspaceDigest := RuntimeDigest("workspace-post-actual-" + suffix)
	payloadBody := contract.WorkspaceReadCanonicalPayloadV2{
		WorkspaceRoot: contract.WorkspaceReadSourceWorkspaceRefV2{
			ID: workspaceID, Revision: 1, Digest: workspaceDigest,
		},
		RelativePath: "src/main.go", StartByte: 2, MaxBytes: 64,
		RequestedNotAfter: now.Add(40 * time.Second).UnixNano(),
	}
	canonical, err := json.Marshal(payloadBody)
	if err != nil {
		panic(err)
	}
	schema := runtimeports.SchemaRefV2{
		Namespace: "praxis.tool", Name: "workspace-read", Version: "1.0.0",
		MediaType: "application/json", ContentDigest: RuntimeDigest("workspace-read-schema-" + suffix),
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema: schema, ContentDigest: runtimecore.DigestBytes(canonical), Length: uint64(len(canonical)),
		Inline: append([]byte(nil), canonical...),
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{
			Policy: "praxis.tool/workspace-read-limit", Digest: RuntimeDigest("limit-policy-" + suffix),
		},
	}
	runtimeFixture := buildWorkspaceReadPostActualRuntimeV2(now, suffix, payload)
	current := runtimeFixture.current
	expires := time.Unix(0, current.ExpiresUnixNano)
	scope := current.Sandbox.Operation.ExecutionScope
	workspace := contract.WorkspaceView{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily, ID: workspaceID, Revision: 1,
			Digest: string(workspaceDigest)[len("sha256:"):], CreatedUnixNano: now.Add(-time.Second).UnixNano(),
			UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		BaseArtifactRef: Ref("base-" + suffix), BaseRevision: "main",
		OverlayRef: Ref("overlay-" + suffix), PolicyRef: Ref("policy-" + suffix),
		Lease: contract.RuntimeLeaseBinding{
			TenantID: string(scope.Identity.TenantID), InstanceID: string(scope.Instance.ID), InstanceEpoch: uint64(scope.Instance.Epoch),
			LeaseID: string(scope.SandboxLease.ID), LeaseEpoch: uint64(scope.SandboxLease.Epoch),
			FenceEpoch: uint64(current.Sandbox.RuntimeLease.FenceEpoch), ScopeDigest: string(current.Sandbox.RuntimeLease.ScopeDigest)[len("sha256:"):],
			ObservedRevision: uint64(current.Sandbox.RuntimeLease.ObservedRevision), ExpiresUnixNano: expires.UnixNano(),
		},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src/generated"}, HiddenScopes: []string{"src/secret"},
		FileScopeDigest: Ref("file-scope-" + suffix).Digest,
	}
	legacy := current.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		panic(err)
	}
	declaredDelegation := runtimeports.ExecutionDelegationRefV2{
		ID: "delegation-post-actual-" + suffix, Revision: 2, Digest: RuntimeDigest("delegation-declared-" + suffix),
	}
	attemptDelegation := runtimeports.ExecutionDelegationRefV2{
		ID: declaredDelegation.ID, Revision: 3, Digest: RuntimeDigest("delegation-attempt-" + suffix),
	}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declaredDelegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		panic(err)
	}
	preparedRef, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: declaredDelegation,
		OperationDigest: current.Sandbox.OperationDigest, IntentID: legacy.IntentID,
		IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: legacy.AttemptID, Provider: legacy.EnforcementPoint,
		PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: current.Sandbox.OperationDigest, EffectID: current.Sandbox.EffectID,
		IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: current.Sandbox.AttemptID, Delegation: &attemptDelegation,
	}
	persisted := runtimeports.PersistedOperationEnforcementRefV3{
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: legacy.AttemptID, OperationDigest: current.Sandbox.OperationDigest,
		Provider: legacy.EnforcementPoint, ReceiptDigest: RuntimeDigest("persisted-enforcement-" + suffix), RecordedRevision: 1,
	}
	snapshot, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
		Prepared: preparedRef, Delegation: attemptDelegation, PersistedEnforcement: persisted,
		OperationDigest: current.Sandbox.OperationDigest, EffectID: current.Sandbox.EffectID,
		IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		Attempt: attempt, ProviderBinding: current.Sandbox.ProviderBinding,
		PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
	})
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealControlledOperationPreparedCurrentProjectionV2(runtimeports.ControlledOperationPreparedCurrentProjectionV2{
		Snapshot: snapshot, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	intent := runtimeFixture.intent
	intentDigest, err := intent.DigestV3()
	if err != nil {
		panic(err)
	}
	effect, err := runtimeports.SealControlledOperationEffectCurrentProjectionV2(runtimeports.ControlledOperationEffectCurrentProjectionV2{
		Intent: intent, IntentDigest: intentDigest, FactRevision: 2,
		State: contract.WorkspaceReadEffectDispatchIntentV2, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	sourceOwner := runtimeports.EffectOwnerRefV2{
		Role: runtimeports.OwnerSettlement, ComponentID: current.Sandbox.ProviderBinding.ComponentID,
		ManifestDigest: current.Sandbox.ProviderBinding.ManifestDigest,
	}
	attemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(attempt)
	if err != nil {
		panic(err)
	}
	source, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(contract.WorkspaceReadSourceCurrentProjectionV2{
		SourceCommand: contract.WorkspaceReadSourceCommandRefV2{
			Owner: sourceOwner, Kind: contract.WorkspaceReadSourceCommandKindV2,
			ID: "tool-workspace-read-command-" + suffix, Revision: 1, Digest: Ref("tool-workspace-read-command-" + suffix).Digest,
		},
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest,
		Prepared: preparedRef, PreparedSemanticDigest: snapshot.SemanticDigest,
		RuntimeAttempt: attempt, RuntimeAttemptDigest: attemptDigest,
		RuntimeEffectIntentDigest: intentDigest, RuntimeEffectFactRevision: effect.FactRevision,
		RuntimeEffectState: contract.WorkspaceReadEffectDispatchIntentV2,
		PayloadSchema:      legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		CanonicalInline: append([]byte(nil), canonical...), CanonicalInlineLength: uint64(len(canonical)),
		WorkspaceView: workspace.Meta.Ref(), RelativePath: payloadBody.RelativePath,
		StartByte: payloadBody.StartByte, MaxBytes: payloadBody.MaxBytes,
		RequestedNotAfterUnixNano: payloadBody.RequestedNotAfter,
		SourceCreatedUnixNano:     now.UnixNano(), SourceNotAfterUnixNano: expires.UnixNano(),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return WorkspaceReadPostActualFixtureV2{
		Now: now,
		Publication: WorkspaceReadCommandPublicationFixtureV2{
			Now: now, Source: source, Effect: effect, Prepared: prepared, Workspace: workspace,
		},
		RuntimeCurrent: current,
	}
}

func (f WorkspaceReadPostActualFixtureV2) QueryBaseV2(
	command contract.WorkspaceReadCommandV1,
	publishedCurrent contract.Ref,
) WorkspaceReadPostActualQueryBaseV2 {
	current := f.RuntimeCurrent
	legacy := current.Dispatch.Record.Permit.LegacyPermit
	provider := current.Sandbox.ProviderBinding
	domain := runtimeports.OperationDomainCommandRefV1{
		Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		Kind:  runtimeports.NamespacedNameV2(contract.WorkspaceReadCommandKindV1),
		ID:    command.Meta.ID, Revision: runtimecore.Revision(command.Meta.Revision), Digest: runtimecore.Digest("sha256:" + command.Meta.Digest),
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest,
		EffectID: current.Sandbox.EffectID, EffectRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		Prepared: f.Publication.Prepared.Snapshot.Prepared, Attempt: f.Publication.Source.RuntimeAttempt,
		Provider: provider, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest,
		PayloadRevision: legacy.PayloadRevision, DomainCommand: domain,
		CheckedUnixNano: f.Now.UnixNano(), ExpiresUnixNano: current.ExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	runtimeInspect := runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect: runtimeports.InspectOperationDispatchEnforcementRequestV4{
			Operation: current.Sandbox.Operation, EffectID: current.Sandbox.EffectID,
			PermitID: current.Phase.PermitID, Phase: current.Phase.Phase,
		},
		PermitDigest: current.Phase.PermitDigest, AdmissionDigest: current.Phase.AdmissionDigest,
		ReviewAuthorization: current.Phase.ReviewAuthorization, SandboxAttempt: current.Phase.SandboxAttempt,
		SandboxProjectionDigest: current.Sandbox.ProjectionDigest,
	}
	transport := runtimeports.ProviderBindingRefV2{
		BindingSetID: "transport-post-actual", BindingSetRevision: 1,
		ComponentID: "praxis.runtime/transport", ManifestDigest: RuntimeDigest("transport-manifest"),
		ArtifactDigest: RuntimeDigest("transport-artifact"), Capability: runtimeports.ControlledOperationProviderTransportCapabilityV2,
	}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{
		UnifiedNotAfterUnixNano: current.ExpiresUnixNano, ProviderTransport: transport, Provider: provider,
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest,
		OperationScopeDigest: current.Sandbox.Operation.ExecutionScopeDigest,
		EffectKind:           runtimeports.OperationScopeEvidenceActionEffectKindV3,
		Prepared:             f.Publication.Prepared.Snapshot.Prepared, Attempt: f.Publication.Source.RuntimeAttempt,
		ExecuteEnforcement: current.Phase,
		ExecuteEvidenceHandoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{
			ID: "handoff-post-actual", Revision: 1, Digest: RuntimeDigest("handoff-post-actual"), ExpiresUnixNano: current.ExpiresUnixNano,
		},
		Boundary: runtimeports.OperationProviderBoundaryRefV1{
			ID: "boundary-post-actual", Revision: 1, Digest: RuntimeDigest("boundary-post-actual"),
		},
		Association: association.Ref, DomainCommand: domain,
	})
	if err != nil {
		panic(err)
	}
	query, err := sandboxports.SealWorkspaceReadCurrentQueryV1(sandboxports.WorkspaceReadCurrentQueryV1{
		RuntimeInspect: runtimeInspect, Authorization: authorization,
		StableKeyDigest: authorization.StableKeyDigest, AuthorizationDigest: authorization.AuthorizationDigest,
		Association: association.Ref, DomainCommand: domain, Command: command.Meta.Ref(),
		PublishedCommandCurrent: &publishedCurrent, WorkspaceView: f.Publication.Workspace.Meta.Ref(),
		FileScopeDigest: f.Publication.Workspace.FileScopeDigest, RelativePath: command.RelativePath,
		CheckedUnixNano: f.Now.UnixNano(), ExpiresUnixNano: current.ExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return WorkspaceReadPostActualQueryBaseV2{Query: query, Association: association, Domain: domain}
}

func (f WorkspaceReadPostActualFixtureV2) QueryV2(
	base WorkspaceReadPostActualQueryBaseV2,
	reservation contract.Ref,
	attempt contract.WorkspaceReadAttemptRefV1,
	admission contract.WorkspaceReadReceiptBindingV1,
) sandboxports.WorkspaceReadCurrentQueryV2 {
	query, err := sandboxports.SealWorkspaceReadCurrentQueryV2(sandboxports.WorkspaceReadCurrentQueryV2{
		Base: base.Query, Reservation: reservation, Attempt: attempt, AdmissionReceipt: admission,
	})
	if err != nil {
		panic(err)
	}
	return query
}

type workspaceReadPostActualRuntimeFixtureV2 struct {
	current runtimeports.CurrentOperationDispatchEnforcementV4
	intent  runtimeports.OperationEffectIntentV3
}

type workspaceReadPostActualGovernanceReaderV2 struct {
	snapshot runtimeports.OperationGovernanceSnapshotV3
}

func (r workspaceReadPostActualGovernanceReaderV2) InspectOperationGovernance(context.Context, runtimeports.OperationSubjectV3) (runtimeports.OperationGovernanceSnapshotV3, error) {
	return r.snapshot, nil
}

type workspaceReadPostActualReviewReaderV2 struct {
	value runtimeports.OperationReviewCurrentProjectionV4
}

func (r workspaceReadPostActualReviewReaderV2) InspectOperationReviewCurrentV4(context.Context, runtimeports.OperationEffectIntentV3) (runtimeports.OperationReviewCurrentProjectionV4, error) {
	return r.value, nil
}

type workspaceReadPostActualSandboxReaderV2 struct {
	value runtimeports.OperationDispatchSandboxCurrentProjectionV4
}

func (r workspaceReadPostActualSandboxReaderV2) InspectOperationDispatchSandboxCurrentV4(context.Context, runtimeports.OperationSubjectV3, runtimecore.EffectIntentID, runtimeports.OperationDispatchSandboxFactRefV4) (runtimeports.OperationDispatchSandboxCurrentProjectionV4, error) {
	return r.value, nil
}

func buildWorkspaceReadPostActualRuntimeV2(now time.Time, suffix string, payload runtimeports.OpaquePayloadV2) workspaceReadPostActualRuntimeFixtureV2 {
	clock := func() time.Time { return now }
	expires := now.Add(45 * time.Second).UnixNano()
	scope := runtimecore.ExecutionScope{
		Identity:     runtimecore.AgentIdentityRef{TenantID: "tenant-post-actual", ID: "identity-post-actual", Epoch: 1},
		Lineage:      runtimecore.LineageRef{ID: "lineage-post-actual", PlanDigest: RuntimeDigest("lineage-post-actual")},
		Instance:     runtimecore.InstanceRef{ID: "instance-post-actual", Epoch: 1},
		SandboxLease: &runtimecore.SandboxLeaseRef{ID: "lease-post-actual", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		panic(err)
	}
	subject := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		RunID: "run-post-actual-" + runtimecore.AgentRunID(suffix), SubjectRevision: 1,
		CurrentProjectionRef: "operation-current-post-actual", CurrentProjectionDigest: RuntimeDigest("operation-current-post-actual"), CurrentProjectionRevision: 1,
	}
	subjectDigest, err := subject.DigestV3()
	if err != nil {
		panic(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-post-actual", BindingSetRevision: 1,
		ComponentID: "praxis.sandbox/workspace-read", ManifestDigest: RuntimeDigest("manifest-post-actual"),
		ArtifactDigest: RuntimeDigest("artifact-post-actual"), Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "effect-post-actual-" + runtimecore.EffectIntentID(suffix), Revision: 1,
		Operation: subject, Kind: runtimeports.OperationScopeEvidenceActionEffectKindV3, RiskClass: "praxis.tool/controlled",
		ActionScopeDigest: RuntimeDigest("action-scope-post-actual"), Payload: payload, PayloadRevision: 1,
		Target:         "praxis.sandbox/workspace-read",
		ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "praxis.sandbox/workspace-read", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID)},
		Owners: []runtimeports.EffectOwnerRefV2{
			{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		},
		Provider:         provider,
		Authority:        runtimeports.AuthorityBindingRefV2{Ref: "authority-post-actual", Digest: RuntimeDigest("authority-post-actual"), Revision: 1, Epoch: 1},
		Review:           runtimeports.OperationReviewBindingRefV3{CaseRef: "review-post-actual", CandidateDigest: RuntimeDigest("candidate-post-actual"), CandidateRevision: 1, PolicyDigest: RuntimeDigest("review-policy-post-actual")},
		Budget:           runtimeports.OperationBudgetBindingRefV3{Ref: "budget-post-actual", Digest: RuntimeDigest("budget-post-actual"), Revision: 1, PolicyDigest: RuntimeDigest("budget-policy-post-actual"), SubjectDigest: subjectDigest},
		Policy:           runtimeports.OperationPolicyBindingRefV3{Ref: "policy-post-actual", Digest: RuntimeDigest("policy-post-actual"), Revision: 1, SubjectDigest: subjectDigest},
		Idempotency:      runtimeports.IdempotencyBindingV2{Key: "post-actual-key", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: runtimecore.IdempotencyQueryable},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	governanceRef := func(id string, digest runtimecore.Digest) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	snapshot := runtimeports.OperationGovernanceSnapshotV3{
		Operation: subject, Active: true, ProjectionWatermark: 1,
		Identity:     governanceRef("identity-post-actual", RuntimeDigest("identity-post-actual")),
		Binding:      governanceRef(provider.BindingSetID, RuntimeDigest("binding-post-actual")),
		CurrentScope: governanceRef(subject.CurrentProjectionRef, subject.CurrentProjectionDigest),
		Authority:    governanceRef(intent.Authority.Ref, intent.Authority.Digest),
		Review: runtimeports.OperationReviewAuthorizationV3{
			Case:            governanceRef(intent.Review.CaseRef, RuntimeDigest("review-case-post-actual")),
			CandidateDigest: intent.Review.CandidateDigest, CandidateRevision: intent.Review.CandidateRevision,
			Verdict:           governanceRef("review-verdict-post-actual", RuntimeDigest("review-verdict-post-actual")),
			ReviewerAuthority: governanceRef("reviewer-authority-post-actual", RuntimeDigest("reviewer-authority-post-actual")),
			PolicyDigest:      intent.Review.PolicyDigest, ExpiresUnixNano: expires,
		},
		Budget: governanceRef(intent.Budget.Ref, intent.Budget.Digest), Policy: governanceRef(intent.Policy.Ref, intent.Policy.Digest),
		Provider: provider, EnforcementPoint: provider, CapabilityGrantDigest: RuntimeDigest("capability-grant-post-actual"),
		Credentials: []runtimeports.OperationCredentialCurrentFactV3{}, ExpiresUnixNano: expires,
	}
	currentReader := workspaceReadPostActualGovernanceReaderV2{snapshot: snapshot}
	effectStore := runtimefakes.NewOperationEffectStoreV3(clock)
	proposed, err := runtimecontrol.NewProposedOperationEffectFactV3(intent, now)
	if err != nil {
		panic(err)
	}
	if _, err = effectStore.CreateOperationEffectV3(context.Background(), proposed); err != nil {
		panic(err)
	}
	accepted := proposed
	accepted.State = runtimecontrol.OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	if _, err = effectStore.CompareAndSwapOperationEffectV3(context.Background(), subject, runtimecontrol.OperationEffectCASRequestV3{ExpectedRevision: proposed.Revision, Next: accepted}); err != nil {
		panic(err)
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		panic(err)
	}
	reviewCurrent, err := runtimeports.SealOperationReviewCurrentProjectionV4(runtimeports.OperationReviewCurrentProjectionV4{
		Operation: subject, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision,
		Target: runtimeports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.Review.CandidateRevision, Digest: intent.Review.CandidateDigest},
		Case:   snapshot.Review.Case, Verdict: snapshot.Review.Verdict, Basis: runtimeports.OperationReviewBasisAcceptedV4,
		Policy:            runtimeports.OperationGovernanceFactRefV3{Ref: "review-policy-post-actual", Revision: 1, Digest: intent.Review.PolicyDigest, ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()},
		ReviewerAuthority: snapshot.Review.ReviewerAuthority, Scope: snapshot.CurrentScope, Binding: snapshot.Binding,
		DecisionEvidence: []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: RuntimeDigest("review-ledger-post-actual"), Sequence: 1, RecordDigest: RuntimeDigest("review-evidence-post-actual")}},
		Current:          true, CurrentnessDigest: RuntimeDigest("review-currentness-post-actual"), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}, now)
	if err != nil {
		panic(err)
	}
	reviewStore := runtimefakes.NewOperationReviewAuthorizationStoreV4(clock)
	reviewGateway := runtimekernel.OperationReviewAuthorizationGatewayV4{Facts: reviewStore, Effects: effectStore, Governance: currentReader, Reviews: workspaceReadPostActualReviewReaderV2{value: reviewCurrent}, Clock: clock}
	effectStore.BindOperationReviewAuthorizationFactsV4(reviewStore)
	reviewAuthorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), runtimeports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-post-actual", Operation: subject, EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	admissions := runtimecontrol.OperationEffectAdmissionGatewayV3{Effects: effectStore}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), subject, intent.ID)
	if err != nil {
		panic(err)
	}
	dispatchGateway := runtimecontrol.OperationGovernanceGatewayV4{Effects: effectStore, Admissions: admissions, Reviews: reviewGateway, Current: currentReader, Clock: clock}
	issue := runtimeports.IssueGovernedOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision,
		Admission: admission, ReviewAuthorization: reviewAuthorization.RefV4(), PermitID: "permit-post-actual", AttemptID: "attempt-post-actual", PermitTTL: 10 * time.Second,
	}
	issued, err := dispatchGateway.IssueOperationDispatchV4(context.Background(), issue)
	if err != nil {
		panic(err)
	}
	begun, err := dispatchGateway.BeginOperationDispatchV4(context.Background(), runtimeports.BeginGovernedOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, ExpectedEffectRevision: issued.Record.EffectFactRevision,
		PermitID: issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision,
		AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: reviewAuthorization.RefV4(),
	})
	if err != nil {
		panic(err)
	}
	sandboxExpires := now.Add(8 * time.Second).UnixNano()
	sandboxRef := func(id string) runtimeports.OperationDispatchSandboxFactRefV4 {
		return runtimeports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: RuntimeDigest(id), ExpiresUnixNano: sandboxExpires}
	}
	sandboxProjection, err := runtimeports.SealOperationDispatchSandboxCurrentProjectionV4(runtimeports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: subject, OperationDigest: subjectDigest, EffectID: intent.ID, IntentRevision: intent.Revision,
		IntentDigest: begun.Record.Permit.LegacyPermit.IntentDigest, AttemptID: issue.AttemptID,
		Attempt: sandboxRef(issue.AttemptID), Reservation: sandboxRef("sandbox-reservation-post-actual"), SandboxLease: *scope.SandboxLease,
		RuntimeLease: runtimeports.OperationDispatchRuntimeLeaseBindingV4{
			Ref: sandboxRef("runtime-lease-post-actual"), Lease: *scope.SandboxLease, Instance: scope.Instance,
			FenceEpoch: 1, ScopeDigest: scopeDigest, ObservedRevision: 1,
		},
		Generation: runtimeports.GenerationBindingAssociationRefV1{ID: "generation-post-actual", Revision: 1, Digest: RuntimeDigest("generation-post-actual")},
		Placement:  sandboxRef("placement-post-actual"), Backend: sandboxRef("backend-post-actual"), Slot: sandboxRef("slot-post-actual"),
		ProviderBinding: begun.Record.Permit.LegacyPermit.EnforcementPoint, Current: true, ProjectionRevision: 1, ExpiresUnixNano: sandboxExpires,
	})
	if err != nil {
		panic(err)
	}
	enforcementGateway := runtimecontrol.OperationDispatchEnforcementGatewayV4{Dispatch: dispatchGateway, Sandbox: workspaceReadPostActualSandboxReaderV2{value: sandboxProjection}, Facts: effectStore, Clock: clock}
	prepareRequest := runtimeports.EnforceCurrentOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, PermitID: issue.PermitID,
		ExpectedPermitFactRevision: begun.Record.Revision, PermitDigest: begun.Record.PermitDigest,
		AdmissionDigest: begun.Record.Permit.Admission.Digest, ReviewAuthorization: reviewAuthorization.RefV4(), AttemptID: issue.AttemptID,
		Phase: runtimeports.OperationDispatchEnforcementPrepareV4, SandboxAttempt: sandboxProjection.Attempt,
		SandboxReservation: sandboxProjection.Reservation, SandboxProjectionDigest: sandboxProjection.ProjectionDigest,
		Verifier: begun.Record.Permit.LegacyPermit.EnforcementPoint,
	}
	prepared, err := enforcementGateway.EnforceCurrentOperationDispatchV4(context.Background(), prepareRequest)
	if err != nil {
		panic(err)
	}
	legacy := prepared.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		panic(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-runtime-post-actual", Revision: 1, Digest: RuntimeDigest("delegation-runtime-post-actual")}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		panic(err)
	}
	preparedAttempt, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: delegation, OperationDigest: subjectDigest,
		IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
		Provider: legacy.EnforcementPoint, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest,
		PayloadRevision: legacy.PayloadRevision, PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	executeRequest := prepareRequest
	executeRequest.Phase = runtimeports.OperationDispatchEnforcementExecuteV4
	executeRequest.ExpectedJournalRevision = 1
	executeRequest.Prepare = &prepared.Phase
	executeRequest.PreparedAttempt = &preparedAttempt
	executed, err := enforcementGateway.EnforceCurrentOperationDispatchV4(context.Background(), executeRequest)
	if err != nil {
		panic(err)
	}
	return workspaceReadPostActualRuntimeFixtureV2{current: executed, intent: intent}
}
