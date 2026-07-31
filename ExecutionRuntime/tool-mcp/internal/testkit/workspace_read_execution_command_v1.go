package testkit

import (
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func WorkspaceReadExecutionCommandV1(
	now time.Time,
	label string,
) toolcontract.WorkspaceReadExecutionCommandV1 {
	boundary := BoundaryFixture(now)
	schema := Schema("workspace-read-" + label)
	payloadDigest := Digest("workspace-read-payload-" + label)
	prepared := PreparedAttemptFor(now, boundary, ProviderBinding(), schema, payloadDigest, 1)

	key := applicationcontract.SingleCallToolActionInspectKeyV2{
		ContractVersion:        applicationcontract.SingleCallToolActionContractVersionV2,
		RequestID:              "workspace-read-request-" + label,
		RequestRevision:        1,
		RequestDigest:          Digest("workspace-read-request-" + label),
		ActionCoordinateDigest: Digest("workspace-read-action-" + label),
		ScopeDigest:            boundary.Operation.ExecutionScopeDigest,
	}
	var err error
	key.Digest, err = key.DigestV2()
	if err != nil {
		panic(err)
	}
	claimID, err := toolcontract.StableID(
		"tool-owner-single-call-claim-v2",
		key.RequestID,
		string(key.RequestDigest),
		string(key.ScopeDigest),
	)
	if err != nil {
		panic(err)
	}
	inputDigest := Digest("workspace-read-execution-input-" + label)
	stateID, err := toolcontract.StableID("tool-owner-execution-state-v2", claimID, string(inputDigest))
	if err != nil {
		panic(err)
	}
	toolAttemptID, err := toolcontract.StableID("tool-owner-execution-attempt-v2", claimID, string(inputDigest))
	if err != nil {
		panic(err)
	}
	tool := toolcontract.ObjectRef{
		ID: "tool-workspace-read-" + label, Revision: 1, Digest: Digest("tool-workspace-read-" + label),
	}
	source, err := toolcontract.SealWorkspaceReadExecutionCommandSourceV1(
		toolcontract.WorkspaceReadExecutionCommandSourceV1{
			RequestKey: key,
			ClaimRef: toolcontract.ObjectRef{
				ID: claimID, Revision: 1, Digest: Digest("workspace-read-claim-" + label),
			},
			ExecutionStateRef: toolcontract.ObjectRef{
				ID: stateID, Revision: 1, Digest: Digest("workspace-read-state-" + label),
			},
			ExecutionStateKind:     toolcontract.WorkspaceReadExecutionStartCommittedV1,
			ExecutionInputDigest:   inputDigest,
			ToolExecutionAttemptID: toolAttemptID,
			BindingCurrent: toolcontract.SingleCallToolActionBindingCurrentRefV2{
				ID: "workspace-read-binding-" + label, Revision: 1, Digest: Digest("workspace-read-binding-" + label),
			},
			Candidate: toolcontract.ObjectRef{
				ID: "workspace-read-candidate-" + label, Revision: 1, Digest: Digest("workspace-read-candidate-" + label),
			},
			CandidateClosureDigest: Digest("workspace-read-candidate-closure-" + label),
			InputContractCurrent: toolcontract.ToolInputContractCurrentRefV1{
				ID: "workspace-read-input-contract-" + label, Revision: 1, Digest: Digest("workspace-read-input-contract-" + label),
			},
			Tool: tool,
			ToolCurrent: toolcontract.ToolRegistryObjectCurrentRefV1{
				Kind: toolcontract.ToolRegistryDescriptorCurrentKindV1,
				ID:   "workspace-read-tool-current-" + label, Revision: 1, Digest: Digest("workspace-read-tool-current-" + label),
			},
			Owner: runtimeports.EffectOwnerRefV2{
				Role: runtimeports.OwnerSettlement, ComponentID: prepared.Provider.ComponentID,
				ManifestDigest: prepared.Provider.ManifestDigest,
			},
		},
	)
	if err != nil {
		panic(err)
	}
	expires := now.Add(10 * time.Second).UnixNano()
	lower := now.Add(-2 * time.Second).UnixNano()
	ttl, err := toolcontract.SealWorkspaceReadExecutionCommandTTLClosureV1(
		toolcontract.WorkspaceReadExecutionCommandTTLClosureV1{
			ClaimCreatedUnixNano: lower, StateUpdatedUnixNano: lower,
			BindingCheckedUnixNano: lower, CandidateCreatedUnixNano: lower,
			InputCheckedUnixNano: lower, PreparedUnixNano: prepared.PreparedUnixNano,
			RequestedNotAfterUnixNano: expires, RequestExpiresUnixNano: expires,
			StateExpiresUnixNano: expires, BindingExpiresUnixNano: expires,
			CandidateExpiresUnixNano: expires, InputExpiresUnixNano: expires,
			EffectIntentExpiresUnixNano: expires, PreparedExpiresUnixNano: prepared.ExpiresUnixNano,
		},
	)
	if err != nil {
		panic(err)
	}
	operationDigest, err := boundary.Operation.DigestV3()
	if err != nil {
		panic(err)
	}
	attemptDigest, err := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(boundary.Attempt)
	if err != nil {
		panic(err)
	}
	fact, err := toolcontract.SealWorkspaceReadExecutionCommandV1(
		toolcontract.WorkspaceReadExecutionCommandV1{
			Source: source, Operation: boundary.Operation, OperationDigest: operationDigest,
			Prepared: prepared, PreparedSemanticDigest: Digest("workspace-read-prepared-semantic-" + label),
			RuntimeAttempt: boundary.Attempt, RuntimeAttemptDigest: attemptDigest,
			RuntimeEffectIntentDigest: boundary.Attempt.IntentDigest,
			RuntimeEffectFactRevision: 1, RuntimeEffectState: toolcontract.WorkspaceReadExecutionDispatchIntentV1,
			PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: 1,
			TTL: ttl, CreatedUnixNano: now.UnixNano(), NotAfterUnixNano: ttl.EffectiveNotAfterUnixNano,
		},
	)
	if err != nil {
		panic(err)
	}
	return fact
}

func WorkspaceReadExecutionCommandCurrentV1(
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	checked time.Time,
) toolcontract.WorkspaceReadExecutionCommandCurrentV1 {
	expires := checked.Add(toolcontract.MaxWorkspaceReadExecutionCommandCurrentTTLV1).UnixNano()
	if fact.NotAfterUnixNano < expires {
		expires = fact.NotAfterUnixNano
	}
	current := toolcontract.WorkspaceReadExecutionCommandCurrentV1{
		ContractVersion:                toolcontract.WorkspaceReadExecutionCommandContractVersionV1,
		Fact:                           toolcontract.CloneWorkspaceReadExecutionCommandV1(fact),
		ToolCurrentProjectionDigest:    Digest("workspace-read-tool-current-proof"),
		ToolCurrentCheckedUnixNano:     checked.UnixNano(),
		RuntimeEffectCurrentDigest:     Digest("workspace-read-effect-current-proof"),
		RuntimeEffectCheckedUnixNano:   checked.UnixNano(),
		RuntimePreparedCurrentDigest:   Digest("workspace-read-prepared-current-proof"),
		RuntimePreparedCheckedUnixNano: checked.UnixNano(),
		CheckedUnixNano:                checked.UnixNano(),
		ExpiresUnixNano:                expires,
	}
	digest, err := current.ComputeDigestV1()
	if err != nil {
		panic(err)
	}
	current.Digest = digest
	if err = current.ValidateCurrent(fact.Ref, checked); err != nil {
		panic(err)
	}
	return current
}

func WorkspaceReadExecutionCommandWithCreatedV1(
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	created time.Time,
) toolcontract.WorkspaceReadExecutionCommandV1 {
	fact = toolcontract.CloneWorkspaceReadExecutionCommandV1(fact)
	fact.CreatedUnixNano = created.UnixNano()
	fact.Ref.Digest = ""
	sealed, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact)
	if err != nil {
		panic(err)
	}
	return sealed
}
