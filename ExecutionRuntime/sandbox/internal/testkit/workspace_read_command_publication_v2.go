package testkit

import (
	"encoding/json"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

type WorkspaceReadCommandPublicationFixtureV2 struct {
	Now       time.Time
	Source    contract.WorkspaceReadSourceCurrentProjectionV2
	Effect    runtimeports.ControlledOperationEffectCurrentProjectionV2
	Prepared  runtimeports.ControlledOperationPreparedCurrentProjectionV2
	Workspace contract.WorkspaceView
}

func WorkspaceReadCommandPublicationV2(now time.Time, suffix string) WorkspaceReadCommandPublicationFixtureV2 {
	scope := runtimecore.ExecutionScope{
		Identity: runtimecore.AgentIdentityRef{
			TenantID: "tenant-" + runtimecore.TenantID(suffix),
			ID:       "identity-" + runtimecore.AgentIdentityID(suffix),
			Epoch:    1,
		},
		Lineage: runtimecore.LineageRef{
			ID:         "lineage-" + runtimecore.InstanceLineageID(suffix),
			PlanDigest: RuntimeDigest("lineage-plan-" + suffix),
		},
		Instance: runtimecore.InstanceRef{
			ID:    "instance-" + runtimecore.AgentInstanceID(suffix),
			Epoch: 7,
		},
		SandboxLease: &runtimecore.SandboxLeaseRef{
			ID:    "lease-" + runtimecore.SandboxLeaseID(suffix),
			Epoch: 11,
		},
		AuthorityEpoch: 3,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		panic(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind:                      runtimeports.OperationScopeRunV3,
		ExecutionScope:            scope,
		ExecutionScopeDigest:      scopeDigest,
		RunID:                     "run-" + runtimecore.AgentRunID(suffix),
		SubjectRevision:           1,
		CurrentProjectionRef:      "operation-current-" + suffix,
		CurrentProjectionDigest:   RuntimeDigest("operation-current-" + suffix),
		CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		panic(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID:       "binding-" + suffix,
		BindingSetRevision: 1,
		ComponentID:        "praxis.sandbox/workspace-read",
		ManifestDigest:     RuntimeDigest("manifest-" + suffix),
		ArtifactDigest:     RuntimeDigest("artifact-" + suffix),
		Capability:         "praxis.sandbox/workspace-read",
	}
	sourceOwner := runtimeports.EffectOwnerRefV2{
		Role:           runtimeports.OwnerSettlement,
		ComponentID:    provider.ComponentID,
		ManifestDigest: provider.ManifestDigest,
	}
	workspaceMeta, err := contract.NewMeta(
		"workspace-"+suffix,
		1,
		now.Add(-4*time.Second),
		now.Add(50*time.Second),
		"workspace-read-publication-test-workspace",
		suffix,
	)
	if err != nil {
		panic(err)
	}
	workspace := contract.WorkspaceView{
		Meta:            workspaceMeta,
		BaseArtifactRef: Ref("base-" + suffix),
		BaseRevision:    "base-revision-" + suffix,
		OverlayRef:      Ref("overlay-" + suffix),
		PolicyRef:       Ref("policy-" + suffix),
		Lease: contract.RuntimeLeaseBinding{
			TenantID:         string(scope.Identity.TenantID),
			InstanceID:       string(scope.Instance.ID),
			InstanceEpoch:    uint64(scope.Instance.Epoch),
			LeaseID:          string(scope.SandboxLease.ID),
			LeaseEpoch:       uint64(scope.SandboxLease.Epoch),
			FenceEpoch:       uint64(scope.AuthorityEpoch),
			ScopeDigest:      string(scopeDigest)[len("sha256:"):],
			ObservedRevision: 5,
			ExpiresUnixNano:  now.Add(45 * time.Second).UnixNano(),
		},
		ReadScopes:      []string{"src"},
		WriteScopes:     []string{"src/generated"},
		HiddenScopes:    []string{"src/secret"},
		FileScopeDigest: Ref("file-scope-" + suffix).Digest,
	}
	payloadBody := contract.WorkspaceReadCanonicalPayloadV2{
		WorkspaceRoot: contract.WorkspaceReadSourceWorkspaceRefV2{
			ID:       workspace.Meta.ID,
			Revision: runtimecore.Revision(workspace.Meta.Revision),
			Digest:   runtimecore.Digest("sha256:" + workspace.Meta.Digest),
		},
		RelativePath:      "src/main.go",
		StartByte:         2,
		MaxBytes:          64,
		RequestedNotAfter: now.Add(40 * time.Second).UnixNano(),
	}
	canonical, err := json.Marshal(payloadBody)
	if err != nil {
		panic(err)
	}
	schema := runtimeports.SchemaRefV2{
		Namespace:     "praxis.tool",
		Name:          "workspace-read",
		Version:       "1.0.0",
		MediaType:     "application/json",
		ContentDigest: RuntimeDigest("workspace-read-schema"),
	}
	payloadDigest := runtimecore.DigestBytes(canonical)
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion:   runtimeports.OperationEffectContractVersionV3,
		ID:                "workspace-read-effect-" + runtimecore.EffectIntentID(suffix),
		Revision:          1,
		Operation:         operation,
		Kind:              "praxis.tool/workspace-read",
		RiskClass:         "praxis.tool/controlled",
		ActionScopeDigest: RuntimeDigest("action-scope-" + suffix),
		Payload: runtimeports.OpaquePayloadV2{
			Schema:        schema,
			ContentDigest: payloadDigest,
			Length:        uint64(len(canonical)),
			Inline:        append([]byte(nil), canonical...),
			LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{
				Policy: "praxis.tool/workspace-read-limit",
				Digest: RuntimeDigest("limit-policy-" + suffix),
			},
		},
		PayloadRevision: 1,
		Target:          "praxis.sandbox/workspace-read",
		ConflictDomain: runtimeports.ConflictDomainBindingV2{
			Domain:      "praxis.sandbox/workspace-read",
			ScopeClass:  runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID),
		},
		Owners: []runtimeports.EffectOwnerRefV2{
			{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			sourceOwner,
		},
		Provider: provider,
		Authority: runtimeports.AuthorityBindingRefV2{
			Ref: "authority-" + suffix, Digest: RuntimeDigest("authority-" + suffix), Revision: 1, Epoch: 3,
		},
		Review: runtimeports.OperationReviewBindingRefV3{
			CaseRef: "review-" + suffix, CandidateDigest: RuntimeDigest("candidate-" + suffix),
			CandidateRevision: 1, PolicyDigest: RuntimeDigest("review-policy-" + suffix),
		},
		Budget: runtimeports.OperationBudgetBindingRefV3{
			Ref: "budget-" + suffix, Digest: RuntimeDigest("budget-" + suffix), Revision: 1,
			PolicyDigest: RuntimeDigest("budget-policy-" + suffix), SubjectDigest: operationDigest,
		},
		Policy: runtimeports.OperationPolicyBindingRefV3{
			Ref: "dispatch-policy-" + suffix, Digest: RuntimeDigest("dispatch-policy-" + suffix),
			Revision: 1, SubjectDigest: operationDigest,
		},
		Idempotency: runtimeports.IdempotencyBindingV2{
			Key: "workspace-read-" + suffix, ScopeClass: runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID),
			Class:       runtimecore.IdempotencyQueryable,
		},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{},
		ExpiresUnixNano:  now.Add(40 * time.Second).UnixNano(),
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		panic(err)
	}
	effect, err := runtimeports.SealControlledOperationEffectCurrentProjectionV2(
		runtimeports.ControlledOperationEffectCurrentProjectionV2{
			Intent:          intent,
			IntentDigest:    intentDigest,
			FactRevision:    2,
			State:           contract.WorkspaceReadEffectDispatchIntentV2,
			CheckedUnixNano: now.UnixNano(),
			ExpiresUnixNano: now.Add(12 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	declaredDelegation := runtimeports.ExecutionDelegationRefV2{
		ID: "delegation-" + suffix, Revision: 2, Digest: RuntimeDigest("delegation-declared-" + suffix),
	}
	attemptDelegation := runtimeports.ExecutionDelegationRefV2{
		ID: declaredDelegation.ID, Revision: 3, Digest: RuntimeDigest("delegation-prepared-" + suffix),
	}
	permitDigest := RuntimeDigest("permit-" + suffix)
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(
		declaredDelegation.ID,
		"permit-"+suffix,
		"attempt-"+suffix,
	)
	if err != nil {
		panic(err)
	}
	preparedRef, err := runtimeports.SealPreparedProviderAttemptRefV2(
		runtimeports.PreparedProviderAttemptRefV2{
			ID:                 preparedID,
			Revision:           1,
			DeclaredDelegation: declaredDelegation,
			OperationDigest:    operationDigest,
			IntentID:           intent.ID,
			IntentRevision:     intent.Revision,
			IntentDigest:       intentDigest,
			PermitID:           "permit-" + suffix,
			PermitRevision:     1,
			PermitDigest:       permitDigest,
			AttemptID:          "attempt-" + suffix,
			Provider:           provider,
			PayloadSchema:      schema,
			PayloadDigest:      payloadDigest,
			PayloadRevision:    1,
			PreparedUnixNano:   now.Add(-time.Second).UnixNano(),
			ExpiresUnixNano:    now.Add(35 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest,
		EffectID:        intent.ID,
		IntentRevision:  intent.Revision,
		IntentDigest:    intentDigest,
		PermitID:        preparedRef.PermitID,
		PermitRevision:  preparedRef.PermitRevision,
		PermitDigest:    preparedRef.PermitDigest,
		AttemptID:       preparedRef.AttemptID,
		Delegation:      &attemptDelegation,
	}
	persisted := runtimeports.PersistedOperationEnforcementRefV3{
		PermitID:         preparedRef.PermitID,
		PermitRevision:   preparedRef.PermitRevision,
		PermitDigest:     preparedRef.PermitDigest,
		AttemptID:        preparedRef.AttemptID,
		OperationDigest:  operationDigest,
		Provider:         provider,
		ReceiptDigest:    RuntimeDigest("enforcement-receipt-" + suffix),
		RecordedRevision: 1,
	}
	snapshot, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(
		runtimeports.ControlledOperationPreparedSemanticSnapshotV2{
			Prepared:             preparedRef,
			Delegation:           attemptDelegation,
			PersistedEnforcement: persisted,
			OperationDigest:      operationDigest,
			EffectID:             intent.ID,
			IntentRevision:       intent.Revision,
			IntentDigest:         intentDigest,
			Attempt:              attempt,
			ProviderBinding:      provider,
			PayloadSchema:        schema,
			PayloadDigest:        payloadDigest,
			PayloadRevision:      1,
		},
	)
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealControlledOperationPreparedCurrentProjectionV2(
		runtimeports.ControlledOperationPreparedCurrentProjectionV2{
			Snapshot:        snapshot,
			CheckedUnixNano: now.UnixNano(),
			ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	attemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(attempt)
	if err != nil {
		panic(err)
	}
	source, err := contract.SealWorkspaceReadSourceCurrentProjectionV2(
		contract.WorkspaceReadSourceCurrentProjectionV2{
			SourceCommand: contract.WorkspaceReadSourceCommandRefV2{
				Owner: sourceOwner,
				Kind:  contract.WorkspaceReadSourceCommandKindV2,
				ID:    "tool-workspace-read-command-" + suffix, Revision: 1,
				Digest: Ref("tool-workspace-read-command-" + suffix).Digest,
			},
			Operation: operation, OperationDigest: operationDigest,
			Prepared: preparedRef, PreparedSemanticDigest: snapshot.SemanticDigest,
			RuntimeAttempt: attempt, RuntimeAttemptDigest: attemptDigest,
			RuntimeEffectIntentDigest: intentDigest, RuntimeEffectFactRevision: effect.FactRevision,
			RuntimeEffectState: contract.WorkspaceReadEffectDispatchIntentV2,
			PayloadSchema:      schema, PayloadDigest: payloadDigest, PayloadRevision: 1,
			CanonicalInline: append([]byte(nil), canonical...), CanonicalInlineLength: uint64(len(canonical)),
			WorkspaceView: workspace.Meta.Ref(), RelativePath: payloadBody.RelativePath,
			StartByte: payloadBody.StartByte, MaxBytes: payloadBody.MaxBytes,
			RequestedNotAfterUnixNano: payloadBody.RequestedNotAfter,
			SourceCreatedUnixNano:     now.UnixNano(),
			SourceNotAfterUnixNano:    now.Add(30 * time.Second).UnixNano(),
			CheckedUnixNano:           now.UnixNano(),
			ExpiresUnixNano:           now.Add(9 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	return WorkspaceReadCommandPublicationFixtureV2{
		Now: now, Source: source, Effect: effect, Prepared: prepared, Workspace: workspace,
	}
}
