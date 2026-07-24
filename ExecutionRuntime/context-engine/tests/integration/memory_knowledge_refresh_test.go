package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	contextadapter "github.com/Proview-China/rax/ExecutionRuntime/context-engine/applicationadapter"
	contextcontract "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	mkcontract "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	knowledgeadapter "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge/contextadapter"
	knowledgesource "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge/contextsource"
	memoryadapter "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory/contextadapter"
	memorysource "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory/contextsource"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestLiveV2OwnersThroughApplicationPublishExactContextFrame(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureWithOwnerSourcesV1()
	if err != nil {
		t.Fatal(err)
	}
	base := coordinationRequest(t, fixture)
	memoryReader, memoryOwnerRequest := memoryV2Fixture(t, fixture.Now, base)
	knowledgeReader, knowledgeOwnerRequest := knowledgeV2Fixture(t, fixture.Now, base)
	memoryPort, err := memoryadapter.NewAdapterV1(memoryReader, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	knowledgePort, err := knowledgeadapter.NewAdapterV1(knowledgeReader, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	base.Memory = ownerSourceRequest(t, applicationcontract.ContextOwnerMemoryV1, base, memoryOwnerRequest)
	base.Knowledge = ownerSourceRequest(t, applicationcontract.ContextOwnerKnowledgeV1, base, knowledgeOwnerRequest)
	contextPort, err := contextadapter.NewContextTurnRefreshAdapterV1(fixture.Service, fixture.Store, fixture.Store, fixture.Parent.Content, memoryPort, knowledgePort, fixture.Clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewContextTurnRefreshCoordinatorV1(application.ContextTurnRefreshCoordinatorConfigV1{Context: contextPort, Memory: memoryPort, Knowledge: knowledgePort, Clock: fixture.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CoordinateContextTurnRefreshV1(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != applicationcontract.ContextTurnRefreshAppliedStateV1 || result.S2AssociationSetDigest.Validate() != nil || result.ApplySettlementRef == nil || result.CurrentPointerRef == nil {
		t.Fatalf("incomplete applied association: %+v", result)
	}
	frameRef := contextcontract.FactRef{ID: result.FrameRef.ID, Revision: uint64(result.FrameRef.Revision), Digest: contextcontract.Digest(result.FrameRef.Digest)}
	frame, err := fixture.Store.FrameByExactRef(context.Background(), frameRef, fixture.Request.ExpectedCurrent.ExecutionScopeDigest)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := fixture.Store.ManifestByExactRef(context.Background(), frame.ManifestRef, frame.Execution.ScopeDigest)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[contextcontract.FragmentKind]bool{}
	for _, fragment := range manifest.Fragments {
		kinds[fragment.Kind] = true
	}
	if !kinds[contextcontract.FragmentMemoryRecall] || !kinds[contextcontract.FragmentKnowledgeReference] || !kinds[contextcontract.FragmentToolResult] {
		t.Fatalf("live Owner frame lacks exact source kinds: %#v", kinds)
	}
}

func coordinationRequest(t *testing.T, fixture *testfixture.RefreshFixtureV1) application.ContextTurnRefreshCoordinationRequestV1 {
	t.Helper()
	sessionDigest := core.Digest(fixture.Request.ExpectedCurrent.SessionRef.Digest)
	session := applicationcontract.SingleCallSessionCoordinateV1{ID: fixture.Request.ExpectedCurrent.SessionRef.ID, Revision: core.Revision(fixture.Request.ExpectedCurrent.SessionRef.Revision), Digest: sessionDigest, Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: fixture.Now.UnixNano(), ExpiresUnixNano: fixture.Now.Add(10 * time.Second).UnixNano()}
	sessionSource := applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: session.Revision, Digest: sessionDigest}
	turnDigest := core.DigestBytes([]byte("turn-current"))
	turn := applicationcontract.SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: fixture.Request.ExpectedCurrent.Turn, Revision: 1, Digest: turnDigest}
	turnSource := applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: turn.ID, Revision: turn.Revision, Digest: turn.Digest}
	payload, err := json.Marshal(fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	return application.ContextTurnRefreshCoordinationRequestV1{ID: "application-context-refresh-live-v2", ExecutionScopeDigest: core.Digest(fixture.Request.ExpectedCurrent.ExecutionScopeDigest), RunID: core.AgentRunID(fixture.Request.ExpectedCurrent.RunID), SourceSession: session, SessionApplicability: sessionSource, SourceTurn: turn, TurnApplicability: turnSource, OpaqueContextRequest: payload, RequestedNotAfterNano: fixture.Request.NotAfterUnixNano}
}

func ownerSourceRequest(t *testing.T, owner applicationcontract.ContextOwnerKindV1, base application.ContextTurnRefreshCoordinationRequestV1, body []byte) *applicationcontract.ContextOwnerSourceRequestV1 {
	t.Helper()
	request, err := applicationcontract.SealContextOwnerSourceRequestV1(applicationcontract.ContextOwnerSourceRequestV1{Owner: owner, SourceSession: base.SourceSession, SessionApplicability: base.SessionApplicability, SourceTurn: base.SourceTurn, TurnApplicability: base.TurnApplicability, OwnerRequest: body, Phase: applicationcontract.ContextSourceCheckS1V1, RequestedNotAfterNano: base.RequestedNotAfterNano})
	if err != nil {
		t.Fatal(err)
	}
	return &request
}

func memoryV2Fixture(t *testing.T, now time.Time, base application.ContextTurnRefreshCoordinationRequestV1) (*memorysource.CurrentReaderV2, []byte) {
	t.Helper()
	binding, err := memorysource.NewStatePlaneBinding("memory/live-v2-binding", 1, "memory-owner-state-plane", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	content, err := memorysource.NewStatePlaneContentStore(binding)
	if err != nil {
		t.Fatal(err)
	}
	contentRef, err := content.PutExact([]byte("live owner memory content"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	store, err := memorysource.NewStore(mkcontract.ClockFunc(func() time.Time { return now }), content)
	if err != nil {
		t.Fatal(err)
	}
	resultRef := exactRef("memory/result")
	domainResult, association, settlement := settledOwnerResult(t, mkcontract.OwnerMemory, "memory", resultRef, now)
	recordRef, projectionRef := exactRef("memory/record"), exactRef("memory/projection")
	viewRef, watermarkRef := exactRef("memory/view"), exactRef("memory/watermark")
	attempt, err := store.PutAttempt(memorysource.LocalAttempt{Ref: mkcontract.Ref{ID: "memory/live-v2-attempt", Revision: 1}, TenantID: "tenant-a", IdentityID: "identity-a", ExecutionScopeDigest: string(base.ExecutionScopeDigest), RunID: string(base.RunID), TurnID: base.SourceTurn.ID, RequestDigest: digest("memory/request"), IdempotencyKey: "memory-live-v2", ObservationRef: exactRef("memory/observation"), ResultRef: resultRef, QueryRef: exactRef("memory/query"), ViewRef: viewRef, WatermarkRef: watermarkRef, AuthorityRef: exactRef("authority"), AuthorityEpoch: 7, PolicyRef: exactRef("policy"), Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", Coverage: mkcontract.Coverage{Status: mkcontract.CoverageComplete, Expected: 1, Available: 1, ProjectionRefs: []mkcontract.Ref{projectionRef}}, Items: []memorysource.StoredItem{{Rank: 0, Score: 10, RecordRef: recordRef, ContentRef: contentRef, SourceRefs: []mkcontract.Ref{exactRef("memory/source")}, EvidenceRefs: []mkcontract.Ref{exactRef("memory/evidence")}, ProjectionRefs: []mkcontract.Ref{projectionRef}, CitationDigest: digest("memory/citation"), RecordExpiresAt: now.Add(time.Hour), ProjectionExpires: now.Add(time.Hour)}}, DomainResult: domainResult, Association: association, Application: settlement, ObservedAt: now, ExpiresAt: now.Add(time.Hour)}, mkcontract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.PublishCurrent(memorysource.CurrentState{Ref: mkcontract.Ref{ID: "memory/live-current", Revision: 1}, TenantID: "tenant-a", IdentityID: "identity-a", AuthorityRef: exactRef("authority"), AuthorityEpoch: 7, PolicyRef: exactRef("policy"), Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", ViewRef: viewRef, WatermarkRef: watermarkRef, Items: []memorysource.CurrentItem{{RecordRef: recordRef, ContentRef: contentRef, ProjectionRefs: []mkcontract.Ref{projectionRef}, Active: true, PoisoningCleared: true, RecordExpiresAt: now.Add(time.Hour), ProjectionExpires: now.Add(time.Hour)}}, ExpiresAt: now.Add(time.Hour)}, mkcontract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Ref = mkcontract.Ref{ID: attempt.Ref.ID, Revision: 2}
	coordinate := memorysource.AttemptCoordinateV2{TenantID: "tenant-a", IdentityRef: exactRef("identity-a"), IdentityEpoch: 7, ExecutionScopeDigest: string(base.ExecutionScopeDigest), RunID: string(base.RunID), SessionRef: appRef(base.SourceSession.ID, uint64(base.SourceSession.Revision), base.SourceSession.Digest), SessionEvidenceRef: appRef(base.SessionApplicability.ID, uint64(base.SessionApplicability.Revision), base.SessionApplicability.Digest), SessionCheckedAt: time.Unix(0, base.SourceSession.CheckedUnixNano), SessionExpiresAt: time.Unix(0, base.SourceSession.ExpiresUnixNano), SourceTurnOrdinal: base.SourceTurn.Ordinal, SourceTurnRef: appRef(base.SourceTurn.ID, uint64(base.SourceTurn.Revision), base.SourceTurn.Digest), TurnEvidenceRef: appRef(base.TurnApplicability.ID, uint64(base.TurnApplicability.Revision), base.TurnApplicability.Digest), TurnCheckedAt: now, TurnExpiresAt: now.Add(10 * time.Second), LegacyTurnID: base.SourceTurn.ID, RequestDigest: attempt.RequestDigest, IdempotencyKey: attempt.IdempotencyKey}
	attempt, coordinate, err = store.PutAttemptV2(attempt, coordinate, mkcontract.ExpectRevision(1))
	if err != nil {
		t.Fatal(err)
	}
	request, err := memorysource.SealCurrentRequestV2(memorysource.CurrentRequestV2{Coordinate: coordinate, CurrentStateRef: state.Ref, ExpectedQueryRef: attempt.QueryRef, ExpectedViewRef: attempt.ViewRef, ExpectedWatermarkRef: attempt.WatermarkRef, AuthorityRef: attempt.AuthorityRef, AuthorityEpoch: attempt.AuthorityEpoch, PolicyRef: attempt.PolicyRef, Purpose: attempt.Purpose, Scopes: append([]string(nil), attempt.Scopes...), SensitivityMax: attempt.SensitivityMax, CheckPhase: memorysource.CheckPhaseS1V2, MaxItems: 8, MaxBytes: 4096, MaxTokens: 1024, PerItemMaxBytes: 1024, EstimatorRef: exactRef("memory/estimator"), CheckedUpperBound: now, NotAfter: now.Add(9 * time.Second), ProjectionID: "memory/live-contribution/s1", ProjectionRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := memorysource.NewCurrentReaderV2(store)
	if err != nil {
		t.Fatal(err)
	}
	return reader, mustJSON(t, request)
}

func knowledgeV2Fixture(t *testing.T, now time.Time, base application.ContextTurnRefreshCoordinationRequestV1) (*knowledgesource.CurrentReaderV2, []byte) {
	t.Helper()
	binding, err := knowledgesource.NewStatePlaneBinding("knowledge/live-v2-binding", 1, "knowledge-owner-state-plane", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	content, err := knowledgesource.NewStatePlaneContentStore(binding)
	if err != nil {
		t.Fatal(err)
	}
	contentRef, err := content.PutExact([]byte("live owner knowledge content"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	store, err := knowledgesource.NewStore(mkcontract.ClockFunc(func() time.Time { return now }), content)
	if err != nil {
		t.Fatal(err)
	}
	resultRef := exactRef("knowledge/result")
	domainResult, association, settlement := settledOwnerResult(t, mkcontract.OwnerKnowledge, "knowledge", resultRef, now)
	recordRef, packageRef, snapshotRef := exactRef("knowledge/record"), exactRef("knowledge/package"), exactRef("knowledge/snapshot")
	pointerRef, projectionRef, sourceRef := exactRef("knowledge/pointer"), exactRef("knowledge/projection"), exactRef("knowledge/source")
	viewRef := exactRef("knowledge/view")
	attempt, err := store.PutAttempt(knowledgesource.LocalAttempt{Ref: mkcontract.Ref{ID: "knowledge/live-v2-attempt", Revision: 1}, TenantID: "tenant-a", ExecutionScopeDigest: string(base.ExecutionScopeDigest), RunID: string(base.RunID), TurnID: base.SourceTurn.ID, RequestDigest: digest("knowledge/request"), IdempotencyKey: "knowledge-live-v2", ObservationRef: exactRef("knowledge/observation"), ResultRef: resultRef, QueryRef: exactRef("knowledge/query"), ViewRef: viewRef, SnapshotRef: snapshotRef, PointerRef: pointerRef, AuthorityRef: exactRef("authority"), PolicyRef: exactRef("policy"), Purpose: "answer", Scopes: []string{"project-a"}, AllowedLicenses: []string{"internal-use"}, SensitivityMax: "internal", Coverage: mkcontract.Coverage{Status: mkcontract.CoverageComplete, Expected: 1, Available: 1, ProjectionRefs: []mkcontract.Ref{projectionRef}}, Items: []knowledgesource.StoredItem{{Rank: 0, Score: 20, RecordRef: recordRef, PackageRef: packageRef, SnapshotRef: snapshotRef, ContentRef: contentRef, SourceRefs: []mkcontract.Ref{sourceRef}, EvidenceRefs: []mkcontract.Ref{exactRef("knowledge/evidence")}, ProjectionRefs: []mkcontract.Ref{projectionRef}, CitationDigest: digest("knowledge/citation"), License: "internal-use", TrustState: "source_supported", ConflictGroup: "group-a", RecordExpiresAt: now.Add(time.Hour), SourceExpiresAt: now.Add(time.Hour), ProjectionExpires: now.Add(time.Hour)}}, DomainResult: domainResult, Association: association, Application: settlement, ObservedAt: now, ExpiresAt: now.Add(time.Hour)}, mkcontract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.PublishCurrent(knowledgesource.CurrentState{Ref: mkcontract.Ref{ID: "knowledge/live-current", Revision: 1}, TenantID: "tenant-a", AuthorityRef: exactRef("authority"), PolicyRef: exactRef("policy"), Purpose: "answer", Scopes: []string{"project-a"}, AllowedLicenses: []string{"internal-use"}, SensitivityMax: "internal", ViewRef: viewRef, SnapshotRef: snapshotRef, PointerRef: pointerRef, Items: []knowledgesource.CurrentItem{{RecordRef: recordRef, PackageRef: packageRef, SnapshotRef: snapshotRef, ContentRef: contentRef, SourceRefs: []mkcontract.Ref{sourceRef}, ProjectionRefs: []mkcontract.Ref{projectionRef}, License: "internal-use", TrustState: "source_supported", ConflictGroup: "group-a", Active: true, PoisoningCleared: true, RecordExpiresAt: now.Add(time.Hour), SourceExpiresAt: now.Add(time.Hour), ProjectionExpires: now.Add(time.Hour)}}, ExpiresAt: now.Add(time.Hour)}, mkcontract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Ref = mkcontract.Ref{ID: attempt.Ref.ID, Revision: 2}
	coordinate := knowledgesource.AttemptCoordinateV2{TenantID: "tenant-a", IdentityRef: exactRef("identity-a"), IdentityEpoch: 7, ExecutionScopeDigest: string(base.ExecutionScopeDigest), RunID: string(base.RunID), SessionRef: appRef(base.SourceSession.ID, uint64(base.SourceSession.Revision), base.SourceSession.Digest), SessionEvidenceRef: appRef(base.SessionApplicability.ID, uint64(base.SessionApplicability.Revision), base.SessionApplicability.Digest), SessionCheckedAt: time.Unix(0, base.SourceSession.CheckedUnixNano), SessionExpiresAt: time.Unix(0, base.SourceSession.ExpiresUnixNano), SourceTurnOrdinal: base.SourceTurn.Ordinal, SourceTurnRef: appRef(base.SourceTurn.ID, uint64(base.SourceTurn.Revision), base.SourceTurn.Digest), TurnEvidenceRef: appRef(base.TurnApplicability.ID, uint64(base.TurnApplicability.Revision), base.TurnApplicability.Digest), TurnCheckedAt: now, TurnExpiresAt: now.Add(10 * time.Second), LegacyTurnID: base.SourceTurn.ID, RequestDigest: attempt.RequestDigest, IdempotencyKey: attempt.IdempotencyKey}
	attempt, coordinate, err = store.PutAttemptV2(attempt, coordinate, mkcontract.ExpectRevision(1))
	if err != nil {
		t.Fatal(err)
	}
	request, err := knowledgesource.SealCurrentRequestV2(knowledgesource.CurrentRequestV2{Coordinate: coordinate, CurrentStateRef: state.Ref, ExpectedQueryRef: attempt.QueryRef, ExpectedViewRef: attempt.ViewRef, ExpectedSnapshotRef: attempt.SnapshotRef, ExpectedPointerRef: attempt.PointerRef, AuthorityRef: attempt.AuthorityRef, AuthorityEpoch: 7, PolicyRef: attempt.PolicyRef, Purpose: attempt.Purpose, Scopes: append([]string(nil), attempt.Scopes...), AllowedLicenses: append([]string(nil), attempt.AllowedLicenses...), SensitivityMax: attempt.SensitivityMax, CheckPhase: knowledgesource.CheckPhaseS1V2, MaxItems: 8, MaxBytes: 4096, MaxTokens: 1024, PerItemMaxBytes: 1024, EstimatorRef: exactRef("knowledge/estimator"), CheckedUpperBound: now, NotAfter: now.Add(9 * time.Second), ProjectionID: "knowledge/live-contribution/s1", ProjectionRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := knowledgesource.NewCurrentReaderV2(store)
	if err != nil {
		t.Fatal(err)
	}
	return reader, mustJSON(t, request)
}

func settledOwnerResult(t *testing.T, owner mkcontract.OwnerDomain, prefix string, result mkcontract.Ref, now time.Time) (mkcontract.DomainResultFact, mkcontract.DomainResultAssociation, mkcontract.SettlementApplication) {
	t.Helper()
	domainResult, err := mkcontract.NewDomainResultFact(owner, prefix+"/domain-result", prefix+"/live-v2-attempt", exactRef(prefix+"/operation"), result, exactRef(prefix+"/inspection"), 0, 1, nil, mkcontract.Coverage{Status: mkcontract.CoverageComplete, Expected: 1, Available: 1}, "local_complete", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	association, err := mkcontract.AssociateDomainResult(domainResult)
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := mkcontract.NewSettlementApplication(owner, prefix+"/application", 1, domainResult, association, mkcontract.RuntimeSettlementRef{Ref: exactRef(prefix + "/runtime-settlement")}, now)
	if err != nil {
		t.Fatal(err)
	}
	return domainResult, association, settlement
}

func exactRef(id string) mkcontract.Ref {
	return mkcontract.Ref{ID: id, Revision: 1, Digest: digest(id)}
}

func appRef(id string, revision uint64, digestValue core.Digest) mkcontract.Ref {
	return mkcontract.Ref{ID: id, Revision: revision, Digest: string(digestValue)}
}

func digest(value string) string {
	return string(core.DigestBytes([]byte(value)))
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
