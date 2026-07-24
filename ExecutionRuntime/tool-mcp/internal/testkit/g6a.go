package testkit

import (
	"encoding/json"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func ModelProjection(callCount int) modelinvoker.ToolCallCandidateObservationProjectionV1 {
	response := modelinvoker.Response{ID: "response-g6a", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall}
	for i := 0; i < callCount; i++ {
		call := modelinvoker.FunctionCall{ID: "call-" + string(rune('a'+i)), Name: "tool.example", Arguments: json.RawMessage(`{"value":1}`)}
		response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call})
	}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(Digest("model-invocation"), response)
	if err != nil {
		panic(err)
	}
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("invocation-g6a", 1, "response-g6a", observation)
	if err != nil {
		panic(err)
	}
	return projection
}

func CurrentRef(kind runtimeports.NamespacedNameV2, ref contract.ObjectRef, owner runtimeports.EffectOwnerRefV2, now time.Time) contract.OwnerCurrentRefV1 {
	return contract.OwnerCurrentRefV1{Kind: kind, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest, Owner: owner, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}
}

func CandidateV2(now time.Time) contract.ActionCandidateV2 {
	owner := SettlementOwner()
	capability, tool := Capability(), Tool()
	pending := contract.PendingActionExactRefV2{ID: "pending-v2", Revision: 1, RequestDigest: Digest("pending-v2")}
	source := contract.ObjectRef{ID: "source-v2", Revision: 1, Digest: Digest("source-v2")}
	capRef := contract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolRef := contract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	schema := Schema("payload")
	schemaRef := contract.ObjectRef{ID: "schema-v2", Revision: 1, Digest: schema.ContentDigest}
	surface := contract.ObjectRef{ID: "surface-v2", Revision: 1, Digest: Digest("surface-v2")}
	c, err := contract.SealActionCandidateV2(contract.ActionCandidateV2{ID: "action-v2", TenantID: "tenant-v2", RunID: "run-v2", SessionID: "session-v2", TurnID: "turn-v2", PendingAction: pending, SourceCandidate: source, Capability: capRef, Tool: toolRef, InputSchema: schema, Payload: Payload(`{"value":1}`), PayloadRevision: 1, OperationScopeDigest: Digest("scope-v2"), EffectKind: "praxis.tool/execute", ExpectedOwner: owner, ConflictDomain: "tenant/tenant-v2/tool/example", IdempotencyKey: "action-v2", CreatedUnixNano: now.UnixNano(), RequestedExpiresUnixNano: now.Add(25 * time.Second).UnixNano(), PendingActionCurrent: CurrentRef("praxis.harness/pending-action", contract.ObjectRef{ID: pending.ID, Revision: pending.Revision, Digest: pending.RequestDigest}, owner, now), SurfaceCurrent: CurrentRef("praxis.tool/surface", surface, owner, now), CapabilityCurrent: CurrentRef("praxis.tool/capability", capRef, owner, now), ToolCurrent: CurrentRef("praxis.tool/descriptor", toolRef, owner, now), InputSchemaCurrent: CurrentRef("praxis.tool/input-schema", schemaRef, owner, now), SourceCandidateCurrent: CurrentRef("praxis.model/source-candidate", source, owner, now)})
	if err != nil {
		panic(err)
	}
	return c
}

type BoundaryFixtureV1 struct {
	Operation   runtimeports.OperationSubjectV3
	Attempt     runtimeports.OperationDispatchAttemptRefV3
	Enforcement runtimeports.OperationDispatchEnforcementPhaseRefV4
	Handoff     runtimeports.OperationScopeEvidenceProviderHandoffFactV3
}

func BoundaryFixture(now time.Time) BoundaryFixtureV1 {
	execution := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-v2", ID: "identity-v2", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-v2", PlanDigest: Digest("plan-v2")}, Instance: core.InstanceRef{ID: "instance-v2", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-v2", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(execution)
	if err != nil {
		panic(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: execution, ExecutionScopeDigest: scopeDigest, RunID: "run-v2", SubjectRevision: 1, CurrentProjectionRef: "run-current-v2", CurrentProjectionDigest: Digest("run-current-v2"), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		panic(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-v2", Revision: 2, Digest: Digest("delegation-current-v2")}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: "effect-v2", IntentRevision: 1, IntentDigest: Digest("intent-v2"), PermitID: "permit-v2", PermitRevision: 2, PermitDigest: Digest("permit-v2"), AttemptID: "attempt-v2", Delegation: &delegation}
	expires := now.Add(20 * time.Second).UnixNano()
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: attempt.EffectID, PermitID: attempt.PermitID, PermitFactRevision: attempt.PermitRevision, PermitDigest: attempt.PermitDigest, AdmissionDigest: Digest("admission-v2"), ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{ID: "review-v2", Revision: 1, Digest: Digest("review-v2")}, AttemptID: attempt.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: Digest("sandbox-attempt-v2"), ExpiresUnixNano: expires}, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: Digest("execute-receipt-v2"), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires, PrepareReceiptDigest: Digest("prepare-receipt-v2"), PreparedAttemptDigest: Digest("prepared-v2")}
	qualification := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-v2", Revision: 1, Digest: Digest("qualification-v2"), ExpiresUnixNano: expires}
	handoff, err := runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(runtimeports.OperationScopeEvidenceProviderHandoffFactV3{ID: "handoff-v2", Revision: 1, Qualification: qualification, Phase: enforcement, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: expires})
	if err != nil {
		panic(err)
	}
	return BoundaryFixtureV1{operation, attempt, enforcement, handoff}
}

func ProviderObservation(now time.Time) runtimeports.ProviderAttemptObservationRefV2 {
	boundary := BoundaryFixture(now)
	prepared := PreparedAttempt(now, boundary, ProviderBinding())
	return runtimeports.ProviderAttemptObservationRefV2{Delegation: *boundary.Attempt.Delegation, PreparedAttemptID: prepared.ID, ProviderOperationRef: "provider-operation-v2", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: Digest("observation-v2"), PayloadDigest: Digest("provider-payload-v2"), PayloadRevision: 1, SourceRegistrationID: "source-registration-v2", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: Digest("ledger-v2"), Sequence: 1, RecordDigest: Digest("record-v2")}, ObservedUnixNano: now.UnixNano()}
}

func ProviderBinding() runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "tool-binding-app", BindingSetRevision: 1, ComponentID: "tool-mcp/engine", ManifestDigest: Digest("manifest"), ArtifactDigest: Digest("tool-artifact-app"), Capability: "praxis.tool/execute"}
}

func PreparedAttempt(now time.Time, boundary BoundaryFixtureV1, provider runtimeports.ProviderBindingRefV2) runtimeports.PreparedProviderAttemptRefV2 {
	return PreparedAttemptFor(now, boundary, provider, Schema("payload"), Digest("arguments-app"), 1)
}

func PreparedAttemptFor(now time.Time, boundary BoundaryFixtureV1, provider runtimeports.ProviderBindingRefV2, schema runtimeports.SchemaRefV2, payloadDigest core.Digest, payloadRevision core.Revision) runtimeports.PreparedProviderAttemptRefV2 {
	declared := runtimeports.ExecutionDelegationRefV2{ID: boundary.Attempt.Delegation.ID, Revision: boundary.Attempt.Delegation.Revision - 1, Digest: Digest("delegation-declared-v2")}
	id, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, boundary.Attempt.PermitID, boundary.Attempt.AttemptID)
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: id, Revision: 1, DeclaredDelegation: declared, OperationDigest: boundary.Attempt.OperationDigest, IntentID: boundary.Attempt.EffectID, IntentRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest, PermitID: boundary.Attempt.PermitID, PermitRevision: boundary.Attempt.PermitRevision, PermitDigest: boundary.Attempt.PermitDigest, AttemptID: boundary.Attempt.AttemptID, Provider: provider, PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: payloadRevision, PreparedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	return prepared
}
