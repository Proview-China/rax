package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewBindingCurrentStoreV1CreateInspectLostReplyAndDeepClone(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	fixture.store.LoseNextReviewBindingPublishReplyV1()
	if _, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Create reply did not remain Indeterminate: %v", err)
	}
	receipt, err := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), fixture.create.PublishRef)
	if err != nil {
		t.Fatal(err)
	}
	currentRequest := ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: receipt.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), currentRequest)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.store.ResolveCurrentReviewBindingV1(context.Background(), ports.ResolveReviewBindingCurrentRequestV1{Source: fixture.source, Subject: fixture.subject})
	if err != nil || resolved != current.Ref {
		t.Fatalf("Resolve and exact current Inspect diverged: ref=%+v err=%v", resolved, err)
	}
	historicalRequest := ports.InspectReviewBindingProjectionRequestV1{Ref: current.Ref, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject}
	historical, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), historicalRequest)
	if err != nil {
		t.Fatal(err)
	}
	historical.Members[0].BindingID = "mutated-return"
	again, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), historicalRequest)
	if err != nil || again.Members[0].BindingID == "mutated-return" {
		t.Fatalf("historical Inspect leaked a mutable alias: %+v %v", again, err)
	}

	// Same canonical Create is a read-only receipt replay, not a second write.
	replayed, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil || replayed != receipt {
		t.Fatalf("same canonical Create was not idempotent: %+v %v", replayed, err)
	}
}

func TestReviewBindingCurrentStoreV1ConcurrentCASPublishesOneRevision(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	nextAssociation := fixture.nextAssociationV1(t)
	casInput := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: created.Projection, Source: fixture.source, Subject: fixture.subject, Association: nextAssociation.Ref}
	casRef, err := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(casInput)
	if err != nil {
		t.Fatal(err)
	}
	request := control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{ExpectedAssociation: fixture.association.Ref, NextAssociation: nextAssociation, Projection: ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: casRef, Input: casInput}}

	var wait sync.WaitGroup
	receipts := make(chan ports.ReviewBindingProjectionPublishReceiptV1, 64)
	errors := make(chan error, 64)
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, callErr := fixture.store.CompareAndSwapReviewBindingAssociationProjectionV1(context.Background(), request)
			if callErr != nil {
				errors <- callErr
				return
			}
			receipts <- receipt
		}()
	}
	wait.Wait()
	close(receipts)
	close(errors)
	for callErr := range errors {
		t.Fatalf("same canonical concurrent CAS failed: %v", callErr)
	}
	var canonical ports.ReviewBindingProjectionPublishReceiptV1
	for receipt := range receipts {
		if canonical.PublishRef.ID == "" {
			canonical = receipt
		}
		if receipt != canonical {
			t.Fatalf("concurrent canonical replay returned different receipts: %+v %+v", canonical, receipt)
		}
	}
	if canonical.Projection.Revision != created.Projection.Revision+1 || canonical.HighestRevision != 2 {
		t.Fatalf("64 CAS calls advanced more than one logical revision: %+v", canonical)
	}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: canonical.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || current.ConsumerAssociation.Ref != nextAssociation.Ref {
		t.Fatalf("CAS did not atomically publish the new closure: %+v %v", current, err)
	}
	historical, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || historical.Ref != created.Projection {
		t.Fatalf("CAS overwrote immutable revision one: %+v %v", historical, err)
	}
}

func TestReviewBindingCurrentStoreV1RejectsAssociationOnlyAdvanceWithActiveProjection(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	nextAssociation := fixture.nextAssociationV1(t)
	if _, err := fixture.store.CompareAndSwapReviewBindingConsumerAssociationV1(context.Background(), fixture.association.Ref, nextAssociation); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("association-only advance exposed a half-state: %v", err)
	}
	association, err := fixture.store.InspectCurrentReviewBindingConsumerAssociationV1(context.Background(), fixture.association.Ref)
	if err != nil || association.Ref != fixture.association.Ref {
		t.Fatalf("rejected association-only advance changed association current: %+v %v", association, err)
	}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || current.Ref != created.Projection || current.ConsumerAssociation.Ref != fixture.association.Ref {
		t.Fatalf("rejected association-only advance changed projection current: %+v %v", current, err)
	}
}

func TestReviewBindingCurrentStoreV1CompoundCASStagedFailureLeaksNothing(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	nextAssociation := fixture.nextAssociationV1(t)
	request := fixture.compoundCASV1(t, created.Projection, nextAssociation)
	fixture.store.FailNextReviewBindingCompoundStageV1()
	if _, err := fixture.store.CompareAndSwapReviewBindingAssociationProjectionV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("compound staged failure was not Unavailable: %v", err)
	}
	association, err := fixture.store.InspectCurrentReviewBindingConsumerAssociationV1(context.Background(), fixture.association.Ref)
	if err != nil || association.Ref != fixture.association.Ref {
		t.Fatalf("compound staged failure changed association current: %+v %v", association, err)
	}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || current.Ref != created.Projection {
		t.Fatalf("compound staged failure changed projection current: %+v %v", current, err)
	}
	if _, err := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), request.Projection.PublishRef); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("compound staged failure leaked receipt: %v", err)
	}
}

func TestReviewBindingCurrentStoreV1CompoundCASLostReplyRecoversExactReceipt(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	nextAssociation := fixture.nextAssociationV1(t)
	request := fixture.compoundCASV1(t, created.Projection, nextAssociation)
	fixture.store.LoseNextReviewBindingPublishReplyV1()
	if _, err := fixture.store.CompareAndSwapReviewBindingAssociationProjectionV1(context.Background(), request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("compound lost reply did not remain Indeterminate: %v", err)
	}
	receipt, err := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), request.Projection.PublishRef)
	if err != nil || receipt.Projection.Revision != created.Projection.Revision+1 {
		t.Fatalf("exact receipt did not recover committed compound CAS: %+v %v", receipt, err)
	}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: receipt.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || current.ConsumerAssociation.Ref != nextAssociation.Ref {
		t.Fatalf("compound lost-reply commit is not exact current: %+v %v", current, err)
	}
	historical, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || historical.Ref != created.Projection {
		t.Fatalf("compound commit overwrote historical revision: %+v %v", historical, err)
	}
}

func TestReviewBindingCurrentStoreV1ClosureDriftFailsCurrentButNotHistory(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	consumerSet, err := fixture.store.InspectBindingSet(context.Background(), fixture.consumer.BindingSetID)
	if err != nil {
		t.Fatal(err)
	}
	revoked := consumerSet
	revoked.Revision++
	revoked.State = control.BindingSetRevoked
	revoked.InvalidationReason = core.ReasonBindingDrift
	if _, err := fixture.store.CompareAndSwapBindingSet(context.Background(), control.BindingSetCASRequestV2{ExpectedRevision: consumerSet.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err == nil {
		t.Fatal("Consumer Binding drift passed current inspection")
	}
	historical, historyErr := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if historyErr != nil || historical.Ref != created.Projection {
		t.Fatalf("bad current state contaminated exact history: %+v %v", historical, historyErr)
	}
}

func TestReviewBindingCurrentStoreV1PublishesExplicitTerminalWithoutRewritingHistory(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	sourceSet, err := fixture.store.InspectBindingSet(context.Background(), fixture.source.BindingSetID)
	if err != nil {
		t.Fatal(err)
	}
	revoked := sourceSet
	revoked.Revision++
	revoked.State = control.BindingSetRevoked
	revoked.InvalidationReason = core.ReasonBindingDrift
	if _, err := fixture.store.CompareAndSwapBindingSet(context.Background(), control.BindingSetCASRequestV2{ExpectedRevision: sourceSet.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	input := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: created.Projection, Source: fixture.source, Subject: fixture.subject, Association: fixture.association.Ref}
	publishRef, _ := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(input)
	terminalReceipt, err := fixture.store.CompareAndSwapReviewBindingProjectionV1(context.Background(), ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: terminalReceipt.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || terminal.State != ports.ReviewBindingCurrentRevokedV1 || terminal.Current {
		t.Fatalf("explicit source revoke did not publish immutable terminal history: %+v %v", terminal, err)
	}
	first, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || first.State != ports.ReviewBindingCurrentActiveV1 || !first.Current {
		t.Fatalf("terminal publication rewrote active history: %+v %v", first, err)
	}
}

func TestReviewBindingCurrentStoreV1StagedFailureLeaksNoHistory(t *testing.T) {
	fixture := newReviewBindingStoreFixtureV1(t)
	request := fixture.create
	request.Input.Association.Revision++
	request.Input.Association.Digest = reviewBindingStoreDigestV1(t, "missing-association")
	request.PublishRef, _ = ports.DeriveCreateReviewBindingProjectionPublishRefV1(request.Input)
	if _, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), request); err == nil {
		t.Fatal("Create with absent association succeeded")
	}
	id, _ := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: fixture.source, Subject: fixture.subject})
	missing := ports.ReviewBindingProjectionRefV1{ID: id, Revision: 1, Digest: reviewBindingStoreDigestV1(t, "absent")}
	if _, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: missing, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked projection history: %v", err)
	}
	if _, err := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), request.PublishRef); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked publish receipt: %v", err)
	}
}

type reviewBindingStoreFixtureV1 struct {
	now         time.Time
	store       *fakes.ReviewBindingCurrentStoreV1
	source      ports.ReviewComponentBindingRefV2
	consumer    ports.ProviderBindingRefV2
	association ports.ReviewBindingConsumerAssociationCurrentProjectionV1
	subject     ports.ReviewBindingSubjectV1
	create      ports.CreateReviewBindingProjectionRequestV1
}

func newReviewBindingStoreFixtureV1(t *testing.T) *reviewBindingStoreFixtureV1 {
	t.Helper()
	base := time.Unix(2_100_000_000, 0)
	now := base.Add(2 * time.Second)
	store := fakes.NewReviewBindingCurrentStoreV1(func() time.Time { return now })
	sourceSet, sourceFact := commitReviewBindingComponentV1(t, store, base, "source-set", "source-binding", "review/auto-worker", "review/attest")
	consumerSet, consumerFact := commitReviewBindingComponentV1(t, store, base, "consumer-set", "consumer-binding", "review/verdict-owner", "runtime/read-review-binding-current")
	source := ports.ReviewComponentBindingRefV2{BindingSetID: sourceSet.ID, BindingSetRevision: sourceSet.Revision, ComponentID: sourceFact.ComponentID, ManifestDigest: sourceFact.ManifestDigest, ArtifactDigest: sourceFact.Manifest.ArtifactDigest, Capability: "review/attest"}
	consumer := ports.ProviderBindingRefV2{BindingSetID: consumerSet.ID, BindingSetRevision: consumerSet.Revision, ComponentID: consumerFact.ComponentID, ManifestDigest: consumerFact.ManifestDigest, ArtifactDigest: consumerFact.Manifest.ArtifactDigest, Capability: "runtime/read-review-binding-current"}
	association, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(ports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: ports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumer, Source: source, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: base.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReviewBindingConsumerAssociationV1(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	subject := ports.ReviewBindingSubjectV1{TenantID: "tenant-a", AssignmentID: "assignment-a", AssignmentRevision: 3, AssignmentDigest: reviewBindingStoreDigestV1(t, "assignment"), ReviewerID: "reviewer-a", TargetID: "target-a", TargetRevision: 5, TargetDigest: reviewBindingStoreDigestV1(t, "target")}
	input := ports.CreateReviewBindingProjectionCommandInputV1{Source: source, Subject: subject, Association: association.Ref}
	publishRef, err := ports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return &reviewBindingStoreFixtureV1{now: now, store: store, source: source, consumer: consumer, association: association, subject: subject, create: ports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}
}

func (f *reviewBindingStoreFixtureV1) nextAssociationV1(t *testing.T) ports.ReviewBindingConsumerAssociationCurrentProjectionV1 {
	t.Helper()
	next := f.association
	next.Ref.Revision++
	next.Ref.Digest = ""
	next.ProjectionDigest = ""
	next.ExpiresUnixNano += int64(10 * time.Second)
	sealed, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func (f *reviewBindingStoreFixtureV1) compoundCASV1(t *testing.T, expected ports.ReviewBindingProjectionRefV1, nextAssociation ports.ReviewBindingConsumerAssociationCurrentProjectionV1) control.CompareAndSwapReviewBindingAssociationProjectionRequestV1 {
	t.Helper()
	input := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: expected, Source: f.source, Subject: f.subject, Association: nextAssociation.Ref}
	publishRef, err := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{ExpectedAssociation: f.association.Ref, NextAssociation: nextAssociation, Projection: ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}
}

func commitReviewBindingComponentV1(t *testing.T, store *fakes.ReviewBindingCurrentStoreV1, base time.Time, setID, bindingID string, component ports.ComponentIDV2, capability ports.CapabilityNameV2) (control.BindingSetFactV2, control.BindingFactV2) {
	t.Helper()
	artifact := reviewBindingStoreDigestV1(t, "artifact-"+string(component))
	manifest := ports.ComponentManifestV2{ContractVersion: ports.BindingContractVersionV2, ComponentID: component, Kind: "runtime/component", GovernanceCategory: "runtime/review", SemanticVersion: "1.0.0", ArtifactDigest: artifact, Contract: ports.ContractBindingV2{Name: "runtime/review-binding", Version: "1.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{}, ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: capability, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: component}, {Role: ports.OwnerSettlement, OwnerComponentID: component}, {Role: ports.OwnerCleanup, OwnerComponentID: component}}, Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{}}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	governance := reviewBindingStoreDigestV1(t, "governance-"+setID)
	expires := base.Add(5 * time.Minute).UnixNano()
	grant := ports.CapabilityGrantV2{Capability: capability, EvidenceDigest: reviewBindingStoreDigestV1(t, "grant-"+bindingID), ObservedUnixNano: base.UnixNano(), ExpiresUnixNano: expires}
	certified := control.BindingFactV2{ID: bindingID, ComponentID: component, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance, State: control.BindingCertified, Revision: 3, Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: base.UnixNano(), CertifiedUnixNano: base.Add(time.Second).UnixNano(), ConformanceEvidenceDigest: reviewBindingStoreDigestV1(t, "conformance-"+bindingID), ExpiresUnixNano: expires}
	declared := certified
	declared.State, declared.Revision = control.BindingDeclared, 1
	declared.Grants = []ports.CapabilityGrantV2{}
	declared.ProbedUnixNano, declared.CertifiedUnixNano, declared.ExpiresUnixNano = 0, 0, 0
	declared.ConformanceEvidenceDigest = ""
	if _, err := store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State, probed.Revision = control.BindingProbed, 2
	probed.CertifiedUnixNano, probed.ConformanceEvidenceDigest = 0, ""
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: certified}); err != nil {
		t.Fatal(err)
	}
	set := control.BindingSetFactV2{ID: setID, PlanID: "plan-" + setID, PlanDigest: reviewBindingStoreDigestV1(t, "plan-"+setID), GovernanceDigest: governance, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: bindingID, BindingRevision: certified.Revision, ComponentID: component, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Contract: manifest.Contract, Owners: append([]ports.OwnerAssignmentV2(nil), manifest.Owners...), Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{component}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: base.Add(time.Second).UnixNano(), ExpiresUnixNano: expires}
	committed, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: bindingID, ExpectedRevision: certified.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.InspectBinding(context.Background(), bindingID)
	if err != nil {
		t.Fatal(err)
	}
	return committed, bound
}

func reviewBindingStoreDigestV1(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
