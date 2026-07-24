package fakes_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationScopeEvidenceActionV3IssuesOnlyAfterFiveCurrentOwnerReadsAndKeepsCursor(t *testing.T) {
	now := time.Unix(930_000, 0)
	store := fakes.NewOperationScopeEvidenceStoreV3(func() time.Time { return now })
	execution := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-action", ID: "identity-action", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-action", PlanDigest: digestV3("lineage-action")}, Instance: core.InstanceRef{ID: "instance-action", Epoch: 1}, AuthorityEpoch: 1}
	executionDigest, _ := ports.ExecutionScopeDigestV2(execution)
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeRunV3, ExecutionScope: execution, ExecutionScopeDigest: executionDigest, RunID: "run-action", SubjectRevision: 1, CurrentProjectionRef: "run-current-action", CurrentProjectionDigest: digestV3("run-current-action"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	applicability := make([]ports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	projections := map[ports.OperationScopeEvidenceApplicabilityDimensionV3]ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}
	for _, route := range ports.OperationScopeEvidenceActionRoutesV3() {
		ref := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "action-source-" + string(route.Dimension), Revision: 1, Digest: digestV3("action-source-" + string(route.Dimension))}
		applicability = append(applicability, ports.OperationScopeEvidenceApplicabilityV3{Dimension: route.Dimension, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &ref})
		projection := ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{Fact: ref, ExecutionScopeDigest: executionDigest, Current: true, ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
		copy := projection
		projection.Digest, _ = core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", copy)
		projections[route.Dimension] = projection
	}
	applicability = ports.NormalizeOperationScopeEvidenceApplicabilityV3(applicability)
	appPolicy, err := ports.SealOperationScopeEvidenceApplicabilityPolicyFactV3(ports.OperationScopeEvidenceApplicabilityPolicyFactV3{ID: "action-app-policy", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, Profile: ports.OperationScopeEvidenceActionPolicyProfileV3, ExecutionScopeDigest: executionDigest, Applicability: applicability, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	schema := ports.SchemaRefV2{Namespace: "praxis.tool", Name: "action-evidence", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV3("action-schema")}
	policy, err := ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{ID: "action-evidence-policy", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, AllowedPhases: []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementPrepareV4}, ExpectedSchema: schema, MaximumPayloadBytes: 1024, MaximumQualificationTTL: 10 * time.Second, MaximumIngestGrace: time.Second, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	generation := ports.GenerationBindingAssociationRefV1{ID: "action-generation", Revision: 1, Digest: digestV3("action-generation")}
	scope := ports.OperationScopeEvidenceScopeV3{LedgerScope: ports.OperationScopeEvidenceLedgerScopeV3{TenantID: execution.Identity.TenantID, OperationDigest: operationDigest, ChainID: "action-chain"}, Operation: operation, OperationDigest: operationDigest, EffectID: "action-effect", EffectRevision: 1, EffectDigest: digestV3("action-effect"), EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, AttemptID: "action-attempt", Phase: ports.OperationDispatchEnforcementPrepareV4, ApplicabilityPolicy: appPolicy.RefV3(), Applicability: applicability, Generation: generation}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "action-authorization", Revision: 1, Digest: digestV3("action-authorization")}
	phase := ports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: scope.EffectID, PermitID: "action-permit", PermitFactRevision: 2, PermitDigest: digestV3("action-permit"), AdmissionDigest: digestV3("action-admission"), ReviewAuthorization: authorization, AttemptID: scope.AttemptID, SandboxAttempt: ports.OperationDispatchSandboxFactRefV4{ID: scope.AttemptID, Revision: 1, Digest: digestV3("action-sandbox"), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, Phase: ports.OperationDispatchEnforcementPrepareV4, ReceiptDigest: digestV3("action-receipt"), JournalRevision: 1, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}
	runtimeCurrent, err := ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{Scope: scope, PermitID: phase.PermitID, PermitFactRevision: phase.PermitFactRevision, PermitDigest: phase.PermitDigest, AdmissionDigest: phase.AdmissionDigest, Authorization: authorization, Phase: phase, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "action-binding", BindingSetRevision: 1, ComponentID: "praxis.tool/owner", ManifestDigest: digestV3("action-manifest"), ArtifactDigest: digestV3("action-artifact"), Capability: "praxis.tool/evidence"}
	source, err := ports.SealOperationScopeEvidenceSourceRegistrationFactV3(ports.OperationScopeEvidenceSourceRegistrationFactV3{ID: "action-source-registration", Revision: 1, SourceID: "praxis.tool/action-source", SourceEpoch: 1, NextSequence: 1, LedgerScope: scope.LedgerScope, Producer: producer, Policy: policy.RefV3(), State: ports.EvidenceSourceActive, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	for _, create := range []func() error{
		func() error {
			_, err := store.CreateOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), appPolicy)
			return err
		},
		func() error {
			_, err := store.CreateOperationScopeEvidencePolicyV3(context.Background(), policy)
			return err
		},
		func() error {
			_, err := store.CreateOperationScopeEvidenceSourceV3(context.Background(), source)
			return err
		},
	} {
		if err := create(); err != nil {
			t.Fatal(err)
		}
	}
	router := &actionGatewayRouterV3{projections: projections}
	gateway := kernel.OperationScopeEvidenceGatewayV3{Facts: store, Runtime: &operationScopeEvidenceRuntimeStubV3{value: runtimeCurrent}, Generation: operationScopeEvidenceGenerationStubV3{value: ports.OperationScopeEvidenceFactRefV3{ID: generation.ID, Revision: generation.Revision, Digest: generation.Digest, ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}}, ActionApplicability: router, Clock: func() time.Time { return now }}
	request := ports.IssueOperationScopeEvidenceRequestV3{QualificationID: "action-qualification", Scope: scope, PermitID: phase.PermitID, PermitFactRevision: phase.PermitFactRevision, PermitDigest: phase.PermitDigest, AdmissionDigest: phase.AdmissionDigest, Authorization: authorization, PhaseRef: phase, EvidencePolicy: policy.RefV3(), Reservation: ports.OperationScopeEvidenceSourceReservationV3{Registration: ports.OperationScopeEvidenceFactRefV3{ID: source.ID, Revision: source.Revision, Digest: source.Digest, ExpiresUnixNano: source.ExpiresUnixNano}, Source: ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: source.ID, SourceEpoch: 1, SourceSequence: 1}, EventID: "action-event", Schema: schema}, RequestedTTL: 10 * time.Second}
	qualified, err := gateway.IssueOperationScopeEvidenceV3(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if router.calls.Load() != 5 || qualified.ExpiresUnixNano != now.Add(8*time.Second).UnixNano() {
		t.Fatalf("Action Owner current reads/TTL not exact: calls=%d expiry=%d", router.calls.Load(), qualified.ExpiresUnixNano)
	}
	currentSource, err := store.InspectOperationScopeEvidenceSourceV3(context.Background(), source.ID)
	if err != nil || currentSource.NextSequence != 1 || currentSource.Revision != 1 {
		t.Fatalf("Issue advanced source cursor: %v %#v", err, currentSource)
	}
}

type actionGatewayRouterV3 struct {
	projections map[ports.OperationScopeEvidenceApplicabilityDimensionV3]ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3
	calls       atomic.Int64
}

func (r *actionGatewayRouterV3) InspectOperationScopeEvidenceActionApplicabilityCurrentV3(_ context.Context, dimension ports.OperationScopeEvidenceApplicabilityDimensionV3, fact ports.OperationScopeEvidenceApplicabilityFactRefV3, scopeDigest core.Digest) (ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	r.calls.Add(1)
	projection, ok := r.projections[dimension]
	if !ok || projection.Fact != fact || projection.ExecutionScopeDigest != scopeDigest {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "fixture Action source drifted")
	}
	return projection, nil
}
