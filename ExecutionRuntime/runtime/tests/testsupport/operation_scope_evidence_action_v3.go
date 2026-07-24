package testsupport

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationScopeEvidenceActionFixtureV3 struct {
	Now         time.Time
	Operation   ports.OperationSubjectV3
	ScopeDigest core.Digest
	Attempt     ports.OperationDispatchAttemptRefV3
	Enforcement ports.OperationDispatchEnforcementPhaseRefV4
	Handoff     ports.OperationScopeEvidenceProviderHandoffFactV3
	Boundary    ports.OperationProviderBoundaryCurrentProjectionV1
	Call        ports.ControlledOperationProviderCallRequestV1
}

func OperationScopeEvidenceActionFixture() OperationScopeEvidenceActionFixtureV3 {
	now := time.Unix(500_000, 0)
	expires := now.Add(20 * time.Second).UnixNano()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-action-test", ID: "identity-action-test", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-action-test", PlanDigest: core.DigestBytes([]byte("plan-action-test"))},
		Instance: core.InstanceRef{ID: "instance-action-test", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		panic(err)
	}
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-action-test",
		SubjectRevision: 1, CurrentProjectionRef: "run-projection-action-test", CurrentProjectionDigest: core.DigestBytes([]byte("run-projection-action-test")), CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		panic(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-action-test", IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent-action-test")),
		PermitID: "permit-action-test", PermitRevision: 2, PermitDigest: core.DigestBytes([]byte("permit-action-test")), AttemptID: "attempt-action-test",
	}
	sandbox := ports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-action-test")), ExpiresUnixNano: expires}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "authorization-action-test", Revision: 1, Digest: core.DigestBytes([]byte("authorization-action-test"))}
	enforcement := ports.OperationDispatchEnforcementPhaseRefV4{
		OperationDigest: operationDigest, EffectID: attempt.EffectID, PermitID: attempt.PermitID, PermitFactRevision: attempt.PermitRevision,
		PermitDigest: attempt.PermitDigest, AdmissionDigest: core.DigestBytes([]byte("admission-action-test")), ReviewAuthorization: authorization,
		AttemptID: attempt.AttemptID, SandboxAttempt: sandbox, Phase: ports.OperationDispatchEnforcementExecuteV4,
		ReceiptDigest: core.DigestBytes([]byte("receipt-action-test")), JournalRevision: 2, ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		PrepareReceiptDigest: core.DigestBytes([]byte("prepare-receipt-action-test")), PreparedAttemptDigest: core.DigestBytes([]byte("prepared-action-test")),
	}
	qualification := ports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-action-test", Revision: 1, Digest: core.DigestBytes([]byte("qualification-action-test")), ExpiresUnixNano: expires}
	handoff, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{
		ID: "handoff-action-test", Revision: 1, Qualification: qualification, Phase: enforcement, CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: expires,
	})
	if err != nil {
		panic(err)
	}
	boundaryRef := ports.OperationProviderBoundaryRefV1{ID: "boundary-action-test", Revision: 1, Digest: core.DigestBytes([]byte("boundary-action-test"))}
	boundary, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(ports.OperationProviderBoundaryCurrentProjectionV1{
		Ref: boundaryRef, Operation: operation, OperationDigest: operationDigest, OperationScopeDigest: core.DigestBytes([]byte("operation-scope-action-test")),
		Attempt: attempt, ExecuteEnforcement: enforcement, ExecuteEvidenceHandoff: handoff.RefV3(), Stage: ports.OperationProviderBoundaryCrossedV1,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		panic(err)
	}
	call := ports.ControlledOperationProviderCallRequestV1{
		Operation: operation, OperationScopeDigest: boundary.OperationScopeDigest, Attempt: attempt, ExecuteEnforcement: enforcement,
		ExecuteEvidenceHandoff: handoff.RefV3(), Boundary: boundaryRef,
	}
	return OperationScopeEvidenceActionFixtureV3{Now: now, Operation: operation, ScopeDigest: boundary.OperationScopeDigest, Attempt: attempt, Enforcement: enforcement, Handoff: handoff, Boundary: boundary, Call: call}
}
