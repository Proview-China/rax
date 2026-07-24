package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewBindingAuthoritativeCurrentV1ReusableConformance(t *testing.T) {
	fixture := newReviewBindingConformanceFixtureV1(t)
	report, err := conformance.CheckReviewBindingAuthoritativeCurrentV1(context.Background(), conformance.ReviewBindingAuthoritativeCurrentCaseV1{
		Reader: fixture.store, Publisher: fixture.store, CompoundPublisher: fixture.store, Create: fixture.create,
		BeforeCreate: func() { fixture.store.LoseNextReviewBindingPublishReplyV1() },
		PrepareCAS: func(_ context.Context, expected ports.ReviewBindingProjectionRefV1) (control.CompareAndSwapReviewBindingAssociationProjectionRequestV1, error) {
			next := fixture.association
			next.Ref.Revision++
			next.Ref.Digest, next.ProjectionDigest = "", ""
			next.ExpiresUnixNano += int64(10 * time.Second)
			sealed, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(next)
			if err != nil {
				return control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{}, err
			}
			input := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: expected, Source: fixture.source, Subject: fixture.subject, Association: sealed.Ref}
			publishRef, err := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(input)
			return control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{ExpectedAssociation: fixture.association.Ref, NextAssociation: sealed, Projection: ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}, err
		},
		Now: func() time.Time { return fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CreateRecoveredExactly || !report.ResolvedCurrent || !report.HistoricalImmutable || report.ConcurrentCASCalls != 64 || report.LogicalCASRevisions != 1 || !report.CurrentClosureObserved || report.MutationRetryObserved || report.ProductionClaimEligible {
		t.Fatalf("Review Binding conformance widened or missed its proof: %+v", report)
	}
}

func TestReviewBindingAuthoritativeCurrentV1ConformanceRejectsTypedNil(t *testing.T) {
	var reader *fakes.ReviewBindingCurrentStoreV1
	_, err := conformance.CheckReviewBindingAuthoritativeCurrentV1(context.Background(), conformance.ReviewBindingAuthoritativeCurrentCaseV1{Reader: reader, Publisher: reader, CompoundPublisher: reader, PrepareCAS: func(context.Context, ports.ReviewBindingProjectionRefV1) (control.CompareAndSwapReviewBindingAssociationProjectionRequestV1, error) {
		return control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{}, nil
	}, Now: time.Now})
	if !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil conformance dependency was accepted: %v", err)
	}
}

type reviewBindingConformanceFixtureV1 struct {
	now         time.Time
	store       *fakes.ReviewBindingCurrentStoreV1
	source      ports.ReviewComponentBindingRefV2
	subject     ports.ReviewBindingSubjectV1
	association ports.ReviewBindingConsumerAssociationCurrentProjectionV1
	create      ports.CreateReviewBindingProjectionRequestV1
}

func newReviewBindingConformanceFixtureV1(t *testing.T) *reviewBindingConformanceFixtureV1 {
	t.Helper()
	base := time.Unix(2_200_000_000, 0)
	now := base.Add(2 * time.Second)
	store := fakes.NewReviewBindingCurrentStoreV1(func() time.Time { return now })
	sourceSet, sourceFact := commitReviewBindingConformanceComponentV1(t, store, base, "source-set", "source-binding", "review/auto-worker", "review/attest")
	consumerSet, consumerFact := commitReviewBindingConformanceComponentV1(t, store, base, "consumer-set", "consumer-binding", "review/verdict-owner", "runtime/read-review-binding-current")
	source := ports.ReviewComponentBindingRefV2{BindingSetID: sourceSet.ID, BindingSetRevision: sourceSet.Revision, ComponentID: sourceFact.ComponentID, ManifestDigest: sourceFact.ManifestDigest, ArtifactDigest: sourceFact.Manifest.ArtifactDigest, Capability: "review/attest"}
	consumer := ports.ProviderBindingRefV2{BindingSetID: consumerSet.ID, BindingSetRevision: consumerSet.Revision, ComponentID: consumerFact.ComponentID, ManifestDigest: consumerFact.ManifestDigest, ArtifactDigest: consumerFact.Manifest.ArtifactDigest, Capability: "runtime/read-review-binding-current"}
	association, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(ports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: ports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumer, Source: source, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: base.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReviewBindingConsumerAssociationV1(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	subject := ports.ReviewBindingSubjectV1{TenantID: "tenant-conformance", AssignmentID: "assignment-conformance", AssignmentRevision: 1, AssignmentDigest: reviewBindingConformanceDigestV1(t, "assignment"), ReviewerID: "reviewer-conformance", TargetID: "target-conformance", TargetRevision: 1, TargetDigest: reviewBindingConformanceDigestV1(t, "target")}
	input := ports.CreateReviewBindingProjectionCommandInputV1{Source: source, Subject: subject, Association: association.Ref}
	publishRef, err := ports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return &reviewBindingConformanceFixtureV1{now: now, store: store, source: source, subject: subject, association: association, create: ports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}
}

func commitReviewBindingConformanceComponentV1(t *testing.T, store *fakes.ReviewBindingCurrentStoreV1, base time.Time, setID, bindingID string, component ports.ComponentIDV2, capability ports.CapabilityNameV2) (control.BindingSetFactV2, control.BindingFactV2) {
	t.Helper()
	artifact := reviewBindingConformanceDigestV1(t, "artifact-"+string(component))
	manifest := ports.ComponentManifestV2{ContractVersion: ports.BindingContractVersionV2, ComponentID: component, Kind: "runtime/component", GovernanceCategory: "runtime/review", SemanticVersion: "1.0.0", ArtifactDigest: artifact, Contract: ports.ContractBindingV2{Name: "runtime/review-binding", Version: "1.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{}, ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: capability, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: component}, {Role: ports.OwnerSettlement, OwnerComponentID: component}, {Role: ports.OwnerCleanup, OwnerComponentID: component}}, Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{}}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	governance := reviewBindingConformanceDigestV1(t, "governance-"+setID)
	expires := base.Add(5 * time.Minute).UnixNano()
	grant := ports.CapabilityGrantV2{Capability: capability, EvidenceDigest: reviewBindingConformanceDigestV1(t, "grant-"+bindingID), ObservedUnixNano: base.UnixNano(), ExpiresUnixNano: expires}
	certified := control.BindingFactV2{ID: bindingID, ComponentID: component, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance, State: control.BindingCertified, Revision: 3, Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: base.UnixNano(), CertifiedUnixNano: base.Add(time.Second).UnixNano(), ConformanceEvidenceDigest: reviewBindingConformanceDigestV1(t, "conformance-"+bindingID), ExpiresUnixNano: expires}
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
	set := control.BindingSetFactV2{ID: setID, PlanID: "plan-" + setID, PlanDigest: reviewBindingConformanceDigestV1(t, "plan-"+setID), GovernanceDigest: governance, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: bindingID, BindingRevision: certified.Revision, ComponentID: component, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Contract: manifest.Contract, Owners: append([]ports.OwnerAssignmentV2(nil), manifest.Owners...), Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{component}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: base.Add(time.Second).UnixNano(), ExpiresUnixNano: expires}
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

func reviewBindingConformanceDigestV1(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
