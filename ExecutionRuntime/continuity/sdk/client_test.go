package sdk_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type sdkClock func() time.Time

func (c sdkClock) Now() time.Time { return c() }

func TestClientReadOnlyTimelineCheckpointRestoreAndRetention(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	timeline, err := domain.NewReferenceTimeline(backend, sdkClock(func() time.Time { return now }), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	record, _, err := timeline.Project(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domain.NewCheckpointManifestControllerV2(backend)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := checkpoint.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: manifest, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	restore, err := domain.NewRestorePlanControllerV2(backend, sdkClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	if _, _, err := restore.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: plan, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	rewind, err := domain.NewRewindPlanControllerV2(backend, sdkClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	rewindPlan := testkit.RewindPlanV2(now)
	if _, _, err := rewind.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: rewindPlan, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	retention, err := domain.NewRetentionManager(backend, sdkClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retention.Create(ctx, "object-sdk-1", "retention-policy-1", "internal"); err != nil {
		t.Fatal(err)
	}
	artifactSource := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)
	artifactFact, err := contract.NewArtifactRelationFactV1("artifact-relation-sdk-1", "artifact-request-sdk-1", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), artifactSource, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateArtifactRelationFactV1(ctx, artifactFact); err != nil {
		t.Fatal(err)
	}
	integrityFact := testkit.ContentIntegrityAuditFactV1(contract.ContentIntegrityMetadataAbsent, "object_metadata_absent", now)
	if _, _, err := backend.CreateContentIntegrityAuditFactV1(ctx, integrityFact); err != nil {
		t.Fatal(err)
	}
	deltaFact, err := contract.NewContentDeltaFactV1("content-delta-sdk-1", "content-delta-request-sdk-1", "content-delta-request-digest", testkit.Scope(), testkit.ContentDeltaOwnerV1(), testkit.ContentDeltaSourceV1(testkit.Scope()), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateContentDeltaFactV1(ctx, deltaFact); err != nil {
		t.Fatal(err)
	}
	derivationFact, err := contract.NewHistoryDerivationCandidateFactV1(
		"history-derivation-sdk-1", "history-derivation-request-sdk-1", "history-derivation-request-digest",
		testkit.Scope(), testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationSummary,
		[]contract.HistoryDerivationEventRefV1{contract.HistoryDerivationEventRefFromRecordV1(record)},
		testkit.ContentDeltaSourceV1(testkit.Scope()).Target, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateHistoryDerivationCandidateFactV1(ctx, derivationFact); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.New(sdk.Config{
		Timeline: timeline, Checkpoints: checkpoint, RestorePlans: restore, RewindPlans: rewind,
		Artifacts: backend, Integrity: backend, Deltas: backend, Derivations: backend, Retention: retention, Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	gotRecord, err := client.InspectEvent(ctx, record.EvidenceRecordRef)
	if err != nil || gotRecord.Candidate.Digest != record.Candidate.Digest {
		t.Fatalf("InspectEvent = (%s,%v)", gotRecord.Candidate.Digest, err)
	}
	page, err := client.QueryTimeline(ctx, contract.TimelineQuery{
		LedgerScopeDigest:  record.LedgerScopeDigest,
		AuthorityWatermark: "authority-watermark", PolicyWatermark: "policy-watermark", PageLimit: 10,
	})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("QueryTimeline = (%d,%v)", len(page.Records), err)
	}
	gotManifest, err := client.InspectCheckpointManifest(ctx, manifest.Ref())
	if err != nil || gotManifest.Ref() != manifest.Ref() {
		t.Fatalf("InspectCheckpointManifest = (%v,%v)", gotManifest.Ref(), err)
	}
	gotPlan, err := client.InspectRestorePlan(ctx, plan.Ref())
	if err != nil || gotPlan.Ref() != plan.Ref() {
		t.Fatalf("InspectRestorePlan = (%v,%v)", gotPlan.Ref(), err)
	}
	gotRewind, err := client.InspectRewindPlan(ctx, rewindPlan.Ref())
	if err != nil || gotRewind.Ref() != rewindPlan.Ref() {
		t.Fatalf("InspectRewindPlan = (%v,%v)", gotRewind.Ref(), err)
	}
	if _, err := client.InspectRetention(ctx, "object-sdk-1"); err != nil {
		t.Fatalf("InspectRetention = %v", err)
	}
	gotArtifact, err := client.InspectArtifactRelation(ctx, artifactFact.Ref())
	if err != nil || gotArtifact.Ref() != artifactFact.Ref() {
		t.Fatalf("InspectArtifactRelation = (%v,%v)", gotArtifact.Ref(), err)
	}
	artifactRelations, err := client.ListArtifactRelations(ctx, artifactSource.Artifact.ArtifactFactRef)
	if err != nil || len(artifactRelations) != 1 || artifactRelations[0].Ref() != artifactFact.Ref() {
		t.Fatalf("ListArtifactRelations = (%#v,%v)", artifactRelations, err)
	}
	relatedRelations, err := client.ListRelatedArtifactRelations(ctx, artifactSource.RelatedFactRef)
	if err != nil || len(relatedRelations) != 1 || relatedRelations[0].Ref() != artifactFact.Ref() {
		t.Fatalf("ListRelatedArtifactRelations = (%#v,%v)", relatedRelations, err)
	}
	artifactRelations[0].SourceProjection.Artifact.ParentRevisionRef.Digest = "mutated"
	againArtifact, err := client.InspectArtifactRelation(ctx, artifactFact.Ref())
	if err != nil || againArtifact.SourceProjection.Artifact.ParentRevisionRef.Digest == "mutated" {
		t.Fatal("SDK Artifact Relation result aliases stored Fact")
	}
	gotIntegrity, err := client.InspectContentIntegrityAudit(ctx, integrityFact.Ref())
	if err != nil || gotIntegrity.Ref() != integrityFact.Ref() {
		t.Fatalf("InspectContentIntegrityAudit = (%v,%v)", gotIntegrity.Ref(), err)
	}
	gotIntegrity.Findings[0].DetailCode = "mutated"
	againIntegrity, err := client.InspectContentIntegrityAudit(ctx, integrityFact.Ref())
	if err != nil || againIntegrity.Findings[0].DetailCode == "mutated" {
		t.Fatal("SDK Content Integrity Audit result aliases stored Fact")
	}
	gotDelta, err := client.InspectContentDelta(ctx, deltaFact.Ref())
	if err != nil || gotDelta.Ref() != deltaFact.Ref() {
		t.Fatalf("InspectContentDelta = (%v,%v)", gotDelta.Ref(), err)
	}
	gotDelta.TargetRecipe[0].Kind = contract.ContentDeltaAdd
	againDelta, err := client.InspectContentDelta(ctx, deltaFact.Ref())
	if err != nil || againDelta.TargetRecipe[0].Kind == contract.ContentDeltaAdd {
		t.Fatal("SDK Content Delta result aliases stored Fact")
	}
	gotDerivation, err := client.InspectHistoryDerivationCandidate(ctx, derivationFact.Ref())
	if err != nil || gotDerivation.Ref() != derivationFact.Ref() {
		t.Fatalf("InspectHistoryDerivationCandidate=(%v,%v)", gotDerivation.Ref(), err)
	}
	gotDerivation.Sources[0].ProjectionDigest = "mutated"
	againDerivation, err := client.InspectHistoryDerivationCandidate(ctx, derivationFact.Ref())
	if err != nil || againDerivation.Sources[0].ProjectionDigest == "mutated" {
		t.Fatal("SDK History Derivation result aliases stored Fact")
	}

	page.Records[0].Candidate.ParentRefs = append(page.Records[0].Candidate.ParentRefs, "mutated")
	again, err := client.InspectEvent(ctx, record.EvidenceRecordRef)
	if err != nil || contract.Contains(again.Candidate.ParentRefs, "mutated") {
		t.Fatal("SDK result aliases stored Event")
	}
}

func TestClientFailsClosedForMissingTypedNilAndExpiredReaders(t *testing.T) {
	var typedNil *domain.CheckpointManifestControllerV2
	if _, err := sdk.New(sdk.Config{Checkpoints: typedNil, Clock: time.Now}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil New error = %v", err)
	}
	var typedNilIntegrity *domain.ContentIntegrityAuditControllerV1
	if _, err := sdk.New(sdk.Config{Integrity: typedNilIntegrity, Clock: time.Now}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil Content Integrity Reader error = %v", err)
	}
	var typedNilDelta *domain.ContentDeltaControllerV1
	if _, err := sdk.New(sdk.Config{Deltas: typedNilDelta, Clock: time.Now}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil Content Delta Reader error = %v", err)
	}
	var typedNilDerivation *domain.HistoryDerivationCandidateControllerV1
	if _, err := sdk.New(sdk.Config{Derivations: typedNilDerivation, Clock: time.Now}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil History Derivation Reader error = %v", err)
	}
	client, err := sdk.New(sdk.Config{Timeline: unavailableTimeline{}, Clock: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.InspectRetention(context.Background(), "object-1"); !contract.HasCode(err, contract.ErrUnsupported) {
		t.Fatalf("missing capability error = %v", err)
	}
	invalid, err := sdk.New(sdk.Config{Retention: invalidRetentionReader{}, Clock: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalid.InspectRetention(context.Background(), "object-1"); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("invalid Retention Reader result was accepted: %v", err)
	}
}

func TestClientPublicSurfaceHasNoDirectGovernedWrite(t *testing.T) {
	typeOf := reflect.TypeOf((*sdk.Client)(nil))
	for i := 0; i < typeOf.NumMethod(); i++ {
		name := strings.ToLower(typeOf.Method(i).Name)
		for _, forbidden := range []string{"createcheckpoint", "createcontentintegrity", "createcontentdelta", "createhistoryderivation", "restoreexecute", "purge", "compact", "publishderivation", "dispatch", "activate", "putobject"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("SDK exposes direct governed write %s", typeOf.Method(i).Name)
			}
		}
	}
}

type unavailableTimeline struct{}

type invalidRetentionReader struct{}

func (invalidRetentionReader) Inspect(context.Context, string) (contract.RetentionFact, error) {
	return contract.RetentionFact{ObjectID: "object-1", Revision: 1, UpdatedUnixNano: 1}, nil
}

func (unavailableTimeline) Inspect(context.Context, string) (contract.TimelineEventRecord, error) {
	return contract.TimelineEventRecord{}, contract.NewError(contract.ErrUnavailable, "timeline", "unavailable")
}
func (unavailableTimeline) Query(context.Context, contract.TimelineQuery) (contract.TimelinePage, error) {
	return contract.TimelinePage{}, contract.NewError(contract.ErrUnavailable, "timeline", "unavailable")
}
func (unavailableTimeline) Watch(context.Context, contract.TimelineQuery) (contract.TimelinePage, error) {
	return contract.TimelinePage{}, contract.NewError(contract.ErrUnavailable, "timeline", "unavailable")
}
