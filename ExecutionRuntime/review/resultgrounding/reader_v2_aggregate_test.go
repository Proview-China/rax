package resultgrounding

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type aggregateFixtureV2 struct {
	now         time.Time
	clock       *testkit.ManualClock
	request     ResultBundleCurrentGroundingRequestV2
	stored      ResultBundleGroundingStoredFactsV2
	context     contract.ReviewerContextEnvelopeV1
	bindings    map[runtimeports.ComponentIDV2]runtimeports.ReviewBindingCurrentProjectionV1
	artifact    runtimeports.ReviewArtifactCurrentProjectionV2
	environment runtimeports.ReviewEnvironmentCurrentProjectionV2
	association runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2
	scope       runtimeports.ReviewValidationScopeCurrentProjectionV2
	evidence    runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	deps        ResultBundleCurrentGroundingDependenciesV2

	storedReader      *storedReaderFakeV2
	contextReader     *contextReaderFakeV2
	bindingReader     *bindingReaderFakeV2
	artifactReader    *artifactReaderFakeV2
	environmentReader *environmentReaderFakeV2
	associationReader *associationReaderFakeV2
	scopeReader       *scopeReaderFakeV2
	evidenceReader    *evidenceReaderFakeV2
}

func newAggregateFixtureV2(t *testing.T) *aggregateFixtureV2 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	clock := testkit.NewClock(now.Add(time.Second))
	target := testkit.Target(now)
	request := testkit.Request(now, target, "case-grounding")
	request.AttachmentEvidence = append([]runtimeports.ReviewEvidenceRefV2(nil), target.Evidence...)
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(request.AttachmentEvidence)
	if err != nil {
		t.Fatal(err)
	}
	request.AttachmentEvidenceDigest = evidenceDigest
	request, err = contract.SealReviewRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	caseValue, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "case-grounding", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		TargetID:       target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Rubric: ptrExactV2(request.Rubric),
		State: contract.CaseReviewingV1, CurrentRoundID: "round-a", CurrentAssignment: "assignment-a", ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	round := testkit.Round(now, caseValue, contract.RouteAutoV1)
	assignment := testkit.Assignment(now, caseValue, round, contract.RouteAutoV1)
	contextSubject := contract.ReviewerContextSubjectV1{TenantID: target.TenantID, Case: exact(caseValue.FactIdentityV1), Round: exact(round.FactIdentityV1), Assignment: exact(assignment.FactIdentityV1), Target: exact(target.FactIdentityV1), Rubric: *round.Rubric, ContextFrameDigest: round.ContextFrameDigest, OutputSchema: target.PayloadSchema}
	materials := contextMaterialsFixtureV2(now)
	envelope, err := contract.SealReviewerContextEnvelopeV1(contract.ReviewerContextEnvelopeV1{
		Ref: contract.ReviewerContextEnvelopeRefV1{TenantID: target.TenantID, Revision: 1}, Subject: contextSubject, Materials: materials,
		AllowedReadCapabilities: []string{"review.read/artifact"}, ReadOnly: true, WorkIdentityRemoved: true,
		State: contract.ReviewerContextEnvelopeActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactOwner := groundingOwnerV2("praxis.artifact/owner", "praxis.artifact/current", "praxis.artifact/current-v2")
	environmentOwner := groundingOwnerV2("praxis.environment/owner", "praxis.environment/current", "praxis.environment/current-v2")
	scopeOwner := groundingOwnerV2("praxis.validation/owner", "praxis.validation/current", "praxis.validation/current-v2")
	locator := locatorFixtureV2(t)
	artifactRef := runtimeports.ReviewArtifactExactSourceRefV2{Kind: "praxis.artifact/code", Owner: artifactOwner, TenantID: target.TenantID, ID: "artifact-a", Revision: 3, Digest: testkit.Digest("artifact-body"), ScopeDigest: target.ActionScopeDigest}
	environmentRef := runtimeports.ReviewEnvironmentExactRefV2{Kind: "praxis.environment/sandbox", Owner: environmentOwner, TenantID: target.TenantID, ID: "environment-a", Revision: 2, Digest: testkit.Digest("environment-body"), ScopeDigest: target.ActionScopeDigest}
	scopeRef := runtimeports.ReviewValidationScopeExactRefV2{Source: runtimeports.ReviewValidationScopeSourceIdentityV2{Kind: "praxis.validation/test", TenantID: target.TenantID, ID: "scope-a"}, Owner: scopeOwner, Revision: 4, Digest: testkit.Digest("scope-body"), ScopeDigest: target.ActionScopeDigest}
	evidence := target.Evidence[0]
	contextSources := make([]contract.ReviewerContextSourceRefV1, len(materials))
	var original contract.ReviewerContextSourceRefV1
	var criteria []contract.ReviewerContextSourceRefV1
	for i, material := range materials {
		contextSources[i] = material.Source
		if material.Kind == contract.ReviewerContextOriginalIntentV1 {
			original = material.Source
		}
		if material.Kind == contract.ReviewerContextAcceptanceCriterionV1 {
			criteria = append(criteria, material.Source)
		}
	}
	bundle, err := contract.SealReviewResultBundleV2(contract.ReviewResultBundleV2{
		FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "bundle-v2", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Request:        exact(request.FactIdentityV1), Target: exact(target.FactIdentityV1), OriginalIntent: original, AcceptanceCriteria: criteria,
		Artifacts:   []contract.ReviewResultArtifactBindingV2{{Source: artifactRef, Anchors: []runtimeports.ReviewArtifactLocatorV2{locator}}},
		Claims:      []contract.ReviewResultClaimV2{{ID: "claim-a", Statement: "artifact satisfies acceptance", Artifact: artifactRef, Anchor: locator, Evidence: []runtimeports.ReviewEvidenceRefV2{evidence}}},
		Environment: environmentRef, ReviewerContext: envelope.Ref, ReviewerContextSources: contextSources, ValidationScope: scopeRef,
		Limitations: []string{}, Uncovered: []string{}, EvidenceSetDigest: target.EvidenceSetDigest, ExpiresUnixNano: now.Add(17 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := ResultBundleGroundingStoredFactsV2{Request: request, Target: target, Bundle: bundle, Case: caseValue, Round: round, Assignment: assignment}
	req := ResultBundleCurrentGroundingRequestV2{TenantID: target.TenantID, Bundle: exact(bundle.FactIdentityV1), Request: exact(request.FactIdentityV1), Target: exact(target.FactIdentityV1), Case: exact(caseValue.FactIdentityV1), Round: exact(round.FactIdentityV1), Assignment: exact(assignment.FactIdentityV1), RunID: target.RunID, ExecutionScope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Evidence: append([]runtimeports.ReviewEvidenceRefV2(nil), target.Evidence...), EvidenceSetDigest: target.EvidenceSetDigest}
	bindingSubject := runtimeports.ReviewBindingSubjectV1{TenantID: target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerID, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	bindings := map[runtimeports.ComponentIDV2]runtimeports.ReviewBindingCurrentProjectionV1{
		artifactOwner.Binding.ComponentID:    bindingProjectionFixtureV2(t, now, artifactOwner.Binding, bindingSubject, now.Add(16*time.Minute)),
		environmentOwner.Binding.ComponentID: bindingProjectionFixtureV2(t, now, environmentOwner.Binding, bindingSubject, now.Add(15*time.Minute)),
		scopeOwner.Binding.ComponentID:       bindingProjectionFixtureV2(t, now, scopeOwner.Binding, bindingSubject, now.Add(14*time.Minute)),
	}
	artifactProjection, err := runtimeports.SealReviewArtifactCurrentProjectionV2(runtimeports.ReviewArtifactCurrentProjectionV2{
		Ref: runtimeports.ReviewArtifactCurrentProjectionRefV2{Revision: 1}, Subject: runtimeports.ReviewArtifactCurrentSubjectV2{Expected: artifactRef, Anchors: []runtimeports.ReviewArtifactLocatorV2{locator}}, Source: artifactRef,
		OwnerBinding: bindings[artifactOwner.Binding.ComponentID], State: runtimeports.ReviewGroundingCurrentActiveV2, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(13 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	environmentProjection, err := runtimeports.SealReviewEnvironmentCurrentProjectionV2(runtimeports.ReviewEnvironmentCurrentProjectionV2{
		Ref: runtimeports.ReviewEnvironmentCurrentProjectionRefV2{Revision: 1}, Subject: runtimeports.ReviewEnvironmentCurrentSubjectV2{Expected: environmentRef}, Source: environmentRef,
		OwnerBinding: bindings[environmentOwner.Binding.ComponentID], OwnerLeaseDigest: testkit.Digest("environment-lease"), State: runtimeports.ReviewGroundingCurrentActiveV2, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(12 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	association := associationFixtureV2(t, now, scopeRef.Source, scopeOwner, now.Add(11*time.Minute))
	locatorDigest, err := digestBundleLocatorsV2(bundle)
	if err != nil {
		t.Fatal(err)
	}
	scopeProjection, err := runtimeports.SealReviewValidationScopeCurrentProjectionV2(runtimeports.ReviewValidationScopeCurrentProjectionV2{
		Ref:     runtimeports.ReviewValidationScopeCurrentProjectionRefV2{Revision: 1},
		Subject: runtimeports.ReviewValidationScopeCurrentSubjectV2{Expected: scopeRef, CoveredArtifactLocatorSetDigest: locatorDigest, EvidenceSetDigest: target.EvidenceSetDigest},
		Source:  scopeRef, OwnerBinding: bindings[scopeOwner.Binding.ComponentID], ValidationMethod: testkit.Schema("validation"),
		CoveredArtifactLocatorSetDigest: locatorDigest, EvidenceSetDigest: target.EvidenceSetDigest,
		State: runtimeports.ReviewGroundingCurrentActiveV2, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceSnapshot := evidenceSnapshotFixtureV2(t, now, target, evidence, now.Add(9*time.Minute))
	storedReader := &storedReaderFakeV2{value: stored}
	contextReader := &contextReaderFakeV2{value: envelope}
	bindingReader := &bindingReaderFakeV2{values: bindings}
	artifactReader := &artifactReaderFakeV2{value: artifactProjection}
	environmentReader := &environmentReaderFakeV2{value: environmentProjection}
	associationReader := &associationReaderFakeV2{value: association}
	scopeReader := &scopeReaderFakeV2{value: scopeProjection}
	evidenceReader := &evidenceReaderFakeV2{value: evidenceSnapshot}
	resolver := routeResolverFakeV2{
		artifact:    routeArtifactFixtureV2(t, artifactRef, artifactReader),
		environment: routeEnvironmentFixtureV2(t, environmentRef, environmentReader),
		scope:       routeScopeFixtureV2(t, scopeRef, scopeReader),
	}
	deps := ResultBundleCurrentGroundingDependenciesV2{Stored: storedReader, Context: contextReader, Binding: bindingReader, Evidence: evidenceReader, ValidationScopeOwnerAssociation: associationReader, Routes: resolver, Clock: clock.Now}
	return &aggregateFixtureV2{now: now, clock: clock, request: req, stored: stored, context: envelope, bindings: bindings, artifact: artifactProjection, environment: environmentProjection, association: association, scope: scopeProjection, evidence: evidenceSnapshot, deps: deps, storedReader: storedReader, contextReader: contextReader, bindingReader: bindingReader, artifactReader: artifactReader, environmentReader: environmentReader, associationReader: associationReader, scopeReader: scopeReader, evidenceReader: evidenceReader}
}

func TestAggregateGroundingHappyPathAndTrueMinimumTTLV2(t *testing.T) {
	f := newAggregateFixtureV2(t)
	for name, err := range map[string]error{
		"request": f.stored.Request.Validate(), "target": f.stored.Target.Validate(),
		"bundle": f.stored.Bundle.Validate(), "case": f.stored.Case.Validate(),
		"round": f.stored.Round.Validate(), "assignment": f.stored.Assignment.Validate(),
	} {
		if err != nil {
			t.Fatalf("%s fixture invalid: %v", name, err)
		}
	}
	cloned := f.stored.Clone()
	for name, err := range map[string]error{
		"request": cloned.Request.Validate(), "target": cloned.Target.Validate(),
		"bundle": cloned.Bundle.Validate(), "case": cloned.Case.Validate(),
		"round": cloned.Round.Validate(), "assignment": cloned.Assignment.Validate(),
	} {
		if err != nil {
			t.Fatalf("%s cloned fixture invalid: %v", name, err)
		}
	}
	if err := cloned.ValidateAgainst(f.request); err != nil {
		t.Fatalf("stored fixture invalid before reader: %v", err)
	}
	reader, err := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(50 * time.Millisecond)}, f.deps)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.InspectResultBundleCurrentGroundingV2(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExpiresUnixNano != f.evidence.Projection.ExpiresUnixNano {
		t.Fatalf("Evidence must be the exact unique minimum TTL: got=%d want=%d", got.ExpiresUnixNano, f.evidence.Projection.ExpiresUnixNano)
	}
	if err := got.ValidateCurrent(f.request.Bundle, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	got.Artifacts[0].Subject.Anchors[0].Payload.Inline[0] ^= 0xff
	again, err := reader.InspectResultBundleCurrentGroundingV2(context.Background(), f.request)
	if err != nil || reflect.DeepEqual(got, again) {
		t.Fatal("aggregate success must be a detached deep clone")
	}
}

func TestAggregateGroundingS2DriftAndBindingDriftFailClosedV2(t *testing.T) {
	tests := map[string]func(*aggregateFixtureV2){
		"stored": func(f *aggregateFixtureV2) {
			f.storedReader.mutateSecond = func(v *ResultBundleGroundingStoredFactsV2) { v.Target.Digest = testkit.Digest("drift") }
		},
		"environment": func(f *aggregateFixtureV2) {
			f.environmentReader.mutateSecond = func(v *runtimeports.ReviewEnvironmentCurrentProjectionV2) { v.Source.Digest = testkit.Digest("drift") }
		},
		"scope": func(f *aggregateFixtureV2) {
			f.scopeReader.mutateSecond = func(v *runtimeports.ReviewValidationScopeCurrentProjectionV2) {
				v.Source.Digest = testkit.Digest("drift")
			}
		},
		"environment_binding": func(f *aggregateFixtureV2) {
			f.bindingReader.mutateSecondFor = f.environment.Source.Owner.Binding.ComponentID
		},
		"scope_binding": func(f *aggregateFixtureV2) {
			f.bindingReader.mutateSecondFor = f.scope.Source.Owner.Binding.ComponentID
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			f := newAggregateFixtureV2(t)
			configure(f)
			reader, _ := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(50 * time.Millisecond)}, f.deps)
			if _, err := reader.InspectResultBundleCurrentGroundingV2(context.Background(), f.request); err == nil {
				t.Fatal("S1/S2 drift must fail closed")
			}
		})
	}
}

func TestAggregateGroundingTTLCrossingAndClockRollbackV2(t *testing.T) {
	f := newAggregateFixtureV2(t)
	f.clock.Set(time.Unix(0, f.evidence.Projection.ExpiresUnixNano))
	reader, _ := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(50 * time.Millisecond)}, f.deps)
	if _, err := reader.InspectResultBundleCurrentGroundingV2(context.Background(), f.request); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing must fail stale: %v", err)
	}
	f = newAggregateFixtureV2(t)
	f.clock = testkit.NewClock(f.now.Add(time.Second))
	f.deps.Clock = func() time.Time {
		value := f.clock.Now()
		f.clock.Set(f.now.Add(-time.Second))
		return value
	}
	reader, _ = NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(50 * time.Millisecond)}, f.deps)
	if _, err := reader.InspectResultBundleCurrentGroundingV2(context.Background(), f.request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("actual-point rollback must fail closed: %v", err)
	}
}

func TestAggregateGroundingCancelledUnknownExactRetryV2(t *testing.T) {
	f := newAggregateFixtureV2(t)
	f.environmentReader.failSecondOnce = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader, _ := NewResultBundleCurrentGroundingReaderV2(ResultBundleGroundingReadRecoveryPolicyV2{ReadRecoveryTimeoutNanos: int64(50 * time.Millisecond)}, f.deps)
	got, err := reader.InspectResultBundleCurrentGroundingV2(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bundle != f.request.Bundle || f.environmentReader.inspectCalls != 3 {
		t.Fatalf("cancelled unknown exact retry must use one same-ref recovery: calls=%d", f.environmentReader.inspectCalls)
	}
}

func ptrExactV2(v contract.ExactResourceRefV1) *contract.ExactResourceRefV1 { return &v }

func contextMaterialsFixtureV2(now time.Time) []contract.ReviewerContextMaterialV1 {
	kinds := []contract.ReviewerContextMaterialKindV1{contract.ReviewerContextOriginalIntentV1, contract.ReviewerContextRequirementV1, contract.ReviewerContextAcceptanceCriterionV1, contract.ReviewerContextStableRuleV1, contract.ReviewerContextCandidateV1, contract.ReviewerContextEvidenceV1, contract.ReviewerContextKnownRiskV1}
	out := make([]contract.ReviewerContextMaterialV1, 0, len(kinds))
	for _, kind := range kinds {
		content := string(kind) + "-content"
		trust := contract.ReviewerContextObservationV1
		switch kind {
		case contract.ReviewerContextOriginalIntentV1, contract.ReviewerContextRequirementV1, contract.ReviewerContextAcceptanceCriterionV1, contract.ReviewerContextStableRuleV1:
			trust = contract.ReviewerContextInstructionV1
		}
		out = append(out, contract.ReviewerContextMaterialV1{Kind: kind, Source: contract.ReviewerContextSourceRefV1{Owner: "praxis.context/owner", ID: string(kind), Revision: 1, Digest: testkit.Digest("source-" + string(kind)), ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()}, MediaType: "text/plain", Content: content, ContentDigest: core.DigestBytes([]byte(content)), Trust: trust})
	}
	return out
}

func groundingOwnerV2(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2, sourceContract runtimeports.NamespacedNameV2) runtimeports.ReviewGroundingOwnerRefV2 {
	return runtimeports.ReviewGroundingOwnerRefV2{Binding: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set-" + string(component), BindingSetRevision: 1, ComponentID: component, ManifestDigest: testkit.Digest("manifest-" + string(component)), ArtifactDigest: testkit.Digest("artifact-" + string(component)), Capability: capability}, SourceContract: sourceContract}
}

func locatorFixtureV2(t *testing.T) runtimeports.ReviewArtifactLocatorV2 {
	t.Helper()
	schema := testkit.Schema("locator")
	body := []byte(`{"line":7}`)
	value, err := runtimeports.SealReviewArtifactLocatorV2(runtimeports.ReviewArtifactLocatorV2{Kind: "praxis.anchor/line", Schema: schema, Payload: runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(body), Length: uint64(len(body)), Inline: body, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.limit/review-anchor", Digest: testkit.Digest("limit")}}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func bindingProjectionFixtureV2(t *testing.T, now time.Time, source runtimeports.ReviewComponentBindingRefV2, subject runtimeports.ReviewBindingSubjectV1, expires time.Time) runtimeports.ReviewBindingCurrentProjectionV1 {
	t.Helper()
	consumerRef := runtimeports.ProviderBindingRefV2{BindingSetID: "host-set", BindingSetRevision: 1, ComponentID: "praxis.review/verdict-owner", ManifestDigest: testkit.Digest("consumer-manifest"), ArtifactDigest: testkit.Digest("consumer-artifact"), Capability: "praxis.runtime/read-review-binding-current"}
	consumer, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2, Ref: consumerRef, State: runtimeports.ProviderBindingCurrentActiveV2, BindingSetDigest: testkit.Digest("consumer-set"), BindingSetSemanticDigest: testkit.Digest("consumer-semantic"), BindingID: "consumer-binding", BindingRevision: 1, GrantDigest: testkit.Digest("consumer-grant"), IssuedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	association, err := runtimeports.SealReviewBindingConsumerAssociationCurrentProjectionV1(runtimeports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: runtimeports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumerRef, Source: source, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	member := runtimeports.ReviewBindingMemberCurrentRefV1{ComponentID: source.ComponentID, BindingID: "source-binding", BindingRevision: 1, BindingFactDigest: testkit.Digest("binding-fact-" + string(source.ComponentID)), ManifestDigest: source.ManifestDigest, ArtifactDigest: source.ArtifactDigest, SetGrantSetDigest: testkit.Digest("grant-set"), FactGrantSetDigest: testkit.Digest("grant-set"), BindingFactExpiresUnixNano: expires.UnixNano(), SetGrantMinExpiresUnixNano: expires.UnixNano(), FactGrantMinExpiresUnixNano: expires.UnixNano()}
	selected := runtimeports.ReviewBindingSelectedGrantRefV1{ComponentID: source.ComponentID, BindingID: member.BindingID, BindingRevision: member.BindingRevision, Capability: source.Capability, SetGrantDigest: testkit.Digest("grant"), FactGrantDigest: testkit.Digest("grant"), ExpiresUnixNano: expires.UnixNano()}
	value, err := runtimeports.SealReviewBindingCurrentProjectionV1(runtimeports.ReviewBindingCurrentProjectionV1{Ref: runtimeports.ReviewBindingProjectionRefV1{Revision: 1}, Source: source, Subject: subject, State: runtimeports.ReviewBindingCurrentActiveV1, Current: true, BindingSetID: source.BindingSetID, BindingSetRevision: source.BindingSetRevision, BindingSetDigest: testkit.Digest("set-digest-" + string(source.ComponentID)), BindingSetSemanticDigest: testkit.Digest("set-semantic-" + string(source.ComponentID)), BindingSetExpiresUnixNano: expires.UnixNano(), Members: []runtimeports.ReviewBindingMemberCurrentRefV1{member}, SelectedGrant: selected, ConsumerAssociation: association, ConsumerBinding: consumer, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func associationFixtureV2(t *testing.T, now time.Time, source runtimeports.ReviewValidationScopeSourceIdentityV2, owner runtimeports.ReviewGroundingOwnerRefV2, expires time.Time) runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2 {
	t.Helper()
	subject := runtimeports.ReviewValidationScopeOwnerAssociationSubjectV2{Source: source}
	id, err := core.CanonicalJSONDigest("praxis.runtime.review-validation-scope-current", runtimeports.ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeOwnerAssociationSubjectV2", subject)
	if err != nil {
		t.Fatal(err)
	}
	value := runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2{ContractVersion: runtimeports.ReviewValidationScopeCurrentContractV2, Ref: runtimeports.ReviewValidationScopeOwnerAssociationRefV2{ID: string(id), Revision: 1}, Subject: subject, Owner: owner, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	copy := value
	digest, err := core.CanonicalJSONDigest("praxis.runtime.review-validation-scope-current", runtimeports.ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeOwnerAssociationCurrentProjectionV2", copy)
	if err != nil {
		t.Fatal(err)
	}
	value.Ref.Digest, value.ProjectionDigest = digest, digest
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func evidenceSnapshotFixtureV2(t *testing.T, now time.Time, target contract.TargetSnapshotV1, evidence runtimeports.ReviewEvidenceRefV2, expires time.Time) runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1 {
	t.Helper()
	record := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("ledger"), Sequence: 1, RecordDigest: testkit.Digest("record")}
	key := runtimeports.EvidenceSubjectKeyV1{Record: record, Source: runtimeports.EvidenceSourceKeyV2{RegistrationID: "source", SourceEpoch: 1, SourceSequence: 1}}
	keyDigest, _ := runtimeports.DigestEvidenceSubjectKeyV1(key)
	absence, err := runtimeports.SealEvidenceTombstoneAbsenceRefV1(runtimeports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: keyDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	evidenceProjection, err := runtimeports.SealEvidenceSubjectCurrentProjectionV1(runtimeports.EvidenceSubjectCurrentProjectionV1{Ref: runtimeports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: key, Causation: []runtimeports.EvidenceCausationRefV2{}, Presence: runtimeports.EvidenceTombstoneAbsentSealedV1, Readability: runtimeports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, ExecutionScope: target.Scope, LedgerScope: runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: target.TenantID, RunID: target.RunID}, ActionScopeDigest: target.ActionScopeDigest, TrustClass: runtimeports.EvidenceTrustObservation, CustomClass: evidence.Classification, CandidateDigest: evidence.Digest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	evidenceIndex, _ := runtimeports.SealEvidenceSubjectCurrentIndexRefV1(runtimeports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: keyDigest, CurrentProjection: evidenceProjection.Ref, OwnerWatermark: 1})
	subjectSnapshot := runtimeports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: runtimeports.EvidenceSubjectCurrentContractVersionV1, Projection: evidenceProjection, CurrentIndex: evidenceIndex}
	projection, err := runtimeports.SealReviewEvidenceApplicabilityProjectionV1(runtimeports.ReviewEvidenceApplicabilityProjectionV1{Ref: runtimeports.ReviewEvidenceApplicabilityRefV1{Revision: 1}, Subject: runtimeports.ReviewEvidenceApplicabilitySubjectV1{TenantID: target.TenantID, Target: runtimeports.ReviewEvidenceTargetRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}, RunID: target.RunID, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, ReviewEvidence: evidence}, EvidenceSubject: key, EvidenceSubjectProjection: evidenceProjection.Ref, EvidenceSubjectSnapshot: subjectSnapshot, Record: record, TrustClass: runtimeports.EvidenceTrustObservation, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	index, _ := runtimeports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(runtimeports.ReviewEvidenceApplicabilityCurrentIndexRefV1{Revision: 1, SubjectDigest: projection.SubjectDigest, CurrentProjection: projection.Ref, HighestRevision: 1})
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
}

func routeProofFixtureV2[T any](t *testing.T, declaration runtimeports.ReviewGroundingRouteDeclarationV2) (runtimeports.ReviewGroundingRouteRefV2, runtimeports.ReviewGroundingReaderBindingRefV2) {
	t.Helper()
	route, err := runtimeports.DeriveReviewGroundingRouteRefV2(declaration)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := runtimeports.SealReviewGroundingReaderBindingRefV2(runtimeports.ReviewGroundingReaderBindingRefV2{ID: "reader-" + string(declaration.Kind), Revision: 1, Route: route, AdapterArtifactDigest: testkit.Digest("adapter-" + string(declaration.Kind))})
	if err != nil {
		t.Fatal(err)
	}
	return route, binding
}

func routeArtifactFixtureV2(t *testing.T, source runtimeports.ReviewArtifactExactSourceRefV2, reader runtimeports.ReviewArtifactCurrentReaderV2) runtimeports.ReviewArtifactResolvedRouteV2 {
	d := runtimeports.ReviewGroundingRouteDeclarationV2{Family: runtimeports.ReviewGroundingArtifactRouteV2, Kind: source.Kind, Owner: source.Owner, Required: true}
	route, binding := routeProofFixtureV2[struct{}](t, d)
	return runtimeports.ReviewArtifactResolvedRouteV2{Proof: runtimeports.ReviewArtifactResolvedRouteProofV2{Declaration: d, Route: route, ReaderBinding: binding}, Reader: reader}
}
func routeEnvironmentFixtureV2(t *testing.T, source runtimeports.ReviewEnvironmentExactRefV2, reader runtimeports.ReviewEnvironmentCurrentReaderV2) runtimeports.ReviewEnvironmentResolvedRouteV2 {
	d := runtimeports.ReviewGroundingRouteDeclarationV2{Family: runtimeports.ReviewGroundingEnvironmentRouteV2, Kind: source.Kind, Owner: source.Owner, Required: true}
	route, binding := routeProofFixtureV2[struct{}](t, d)
	return runtimeports.ReviewEnvironmentResolvedRouteV2{Proof: runtimeports.ReviewEnvironmentResolvedRouteProofV2{Declaration: d, Route: route, ReaderBinding: binding}, Reader: reader}
}
func routeScopeFixtureV2(t *testing.T, source runtimeports.ReviewValidationScopeExactRefV2, reader runtimeports.ReviewValidationScopeCurrentReaderV2) runtimeports.ReviewValidationScopeResolvedRouteV2 {
	d := runtimeports.ReviewGroundingRouteDeclarationV2{Family: runtimeports.ReviewGroundingValidationScopeRouteV2, Kind: source.Source.Kind, Owner: source.Owner, Required: true}
	route, binding := routeProofFixtureV2[struct{}](t, d)
	return runtimeports.ReviewValidationScopeResolvedRouteV2{Proof: runtimeports.ReviewValidationScopeResolvedRouteProofV2{Declaration: d, Route: route, ReaderBinding: binding}, Reader: reader}
}

type storedReaderFakeV2 struct {
	value        ResultBundleGroundingStoredFactsV2
	calls        int
	mutateSecond func(*ResultBundleGroundingStoredFactsV2)
}

func (f *storedReaderFakeV2) InspectResultBundleGroundingStoredFactsV2(context.Context, ResultBundleCurrentGroundingRequestV2) (ResultBundleGroundingStoredFactsV2, error) {
	f.calls++
	out := f.value.Clone()
	if f.calls > 1 && f.mutateSecond != nil {
		f.mutateSecond(&out)
	}
	return out, nil
}

type contextReaderFakeV2 struct {
	value        contract.ReviewerContextEnvelopeV1
	inspectCalls int
}

func (f *contextReaderFakeV2) ResolveCurrentReviewerContextV1(context.Context, reviewport.ReviewerContextCurrentResolveRequestV1) (contract.ReviewerContextEnvelopeRefV1, error) {
	return f.value.Ref, nil
}
func (f *contextReaderFakeV2) InspectCurrentReviewerContextV1(context.Context, contract.ReviewerContextSubjectV1, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	f.inspectCalls++
	return f.value.Clone(), nil
}
func (f *contextReaderFakeV2) InspectHistoricalReviewerContextV1(context.Context, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	return f.value.Clone(), nil
}

type bindingReaderFakeV2 struct {
	values          map[runtimeports.ComponentIDV2]runtimeports.ReviewBindingCurrentProjectionV1
	inspectCalls    map[runtimeports.ComponentIDV2]int
	mutateSecondFor runtimeports.ComponentIDV2
}

func (f *bindingReaderFakeV2) ResolveCurrentReviewBindingV1(_ context.Context, r runtimeports.ResolveReviewBindingCurrentRequestV1) (runtimeports.ReviewBindingProjectionRefV1, error) {
	return f.values[r.Source.ComponentID].Ref, nil
}
func (f *bindingReaderFakeV2) InspectReviewBindingProjectionV1(_ context.Context, r runtimeports.InspectReviewBindingProjectionRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return f.values[r.ExpectedSource.ComponentID].CloneV1(), nil
}
func (f *bindingReaderFakeV2) InspectCurrentReviewBindingV1(_ context.Context, r runtimeports.InspectCurrentReviewBindingRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	if f.inspectCalls == nil {
		f.inspectCalls = map[runtimeports.ComponentIDV2]int{}
	}
	id := r.ExpectedSource.ComponentID
	f.inspectCalls[id]++
	out := f.values[id].CloneV1()
	if f.inspectCalls[id] > 1 && id == f.mutateSecondFor {
		out.Ref.Digest = testkit.Digest("binding-drift")
	}
	return out, nil
}

type artifactReaderFakeV2 struct {
	value        runtimeports.ReviewArtifactCurrentProjectionV2
	inspectCalls int
}

func (f *artifactReaderFakeV2) ResolveCurrentReviewArtifactV2(context.Context, runtimeports.ReviewArtifactCurrentSubjectV2) (runtimeports.ReviewArtifactCurrentProjectionRefV2, error) {
	return f.value.Ref, nil
}
func (f *artifactReaderFakeV2) InspectCurrentReviewArtifactV2(context.Context, runtimeports.ReviewArtifactCurrentSubjectV2, runtimeports.ReviewArtifactCurrentProjectionRefV2) (runtimeports.ReviewArtifactCurrentProjectionV2, error) {
	f.inspectCalls++
	return f.value.Clone(), nil
}
func (f *artifactReaderFakeV2) InspectHistoricalReviewArtifactV2(context.Context, runtimeports.ReviewArtifactCurrentProjectionRefV2) (runtimeports.ReviewArtifactCurrentProjectionV2, error) {
	return f.value.Clone(), nil
}

type environmentReaderFakeV2 struct {
	value          runtimeports.ReviewEnvironmentCurrentProjectionV2
	inspectCalls   int
	mutateSecond   func(*runtimeports.ReviewEnvironmentCurrentProjectionV2)
	failSecondOnce bool
}

func (f *environmentReaderFakeV2) ResolveCurrentReviewEnvironmentV2(context.Context, runtimeports.ReviewEnvironmentCurrentSubjectV2) (runtimeports.ReviewEnvironmentCurrentProjectionRefV2, error) {
	return f.value.Ref, nil
}
func (f *environmentReaderFakeV2) InspectCurrentReviewEnvironmentV2(ctx context.Context, _ runtimeports.ReviewEnvironmentCurrentSubjectV2, _ runtimeports.ReviewEnvironmentCurrentProjectionRefV2) (runtimeports.ReviewEnvironmentCurrentProjectionV2, error) {
	f.inspectCalls++
	if f.inspectCalls == 2 && f.failSecondOnce {
		return runtimeports.ReviewEnvironmentCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost reply")
	}
	out := f.value.Clone()
	if f.inspectCalls > 1 && f.mutateSecond != nil {
		f.mutateSecond(&out)
	}
	return out, nil
}
func (f *environmentReaderFakeV2) InspectHistoricalReviewEnvironmentV2(context.Context, runtimeports.ReviewEnvironmentCurrentProjectionRefV2) (runtimeports.ReviewEnvironmentCurrentProjectionV2, error) {
	return f.value.Clone(), nil
}

type associationReaderFakeV2 struct {
	value        runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2
	inspectCalls int
}

func (f *associationReaderFakeV2) ResolveCurrentReviewValidationScopeOwnerAssociationV2(context.Context, runtimeports.ReviewValidationScopeOwnerAssociationSubjectV2) (runtimeports.ReviewValidationScopeOwnerAssociationRefV2, error) {
	return f.value.Ref, nil
}
func (f *associationReaderFakeV2) InspectCurrentReviewValidationScopeOwnerAssociationV2(context.Context, runtimeports.ReviewValidationScopeOwnerAssociationSubjectV2, runtimeports.ReviewValidationScopeOwnerAssociationRefV2) (runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error) {
	f.inspectCalls++
	return f.value, nil
}
func (f *associationReaderFakeV2) InspectHistoricalReviewValidationScopeOwnerAssociationV2(context.Context, runtimeports.ReviewValidationScopeOwnerAssociationRefV2) (runtimeports.ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error) {
	return f.value, nil
}

type scopeReaderFakeV2 struct {
	value        runtimeports.ReviewValidationScopeCurrentProjectionV2
	inspectCalls int
	mutateSecond func(*runtimeports.ReviewValidationScopeCurrentProjectionV2)
}

func (f *scopeReaderFakeV2) ResolveCurrentReviewValidationScopeV2(context.Context, runtimeports.ReviewValidationScopeCurrentSubjectV2) (runtimeports.ReviewValidationScopeCurrentProjectionRefV2, error) {
	return f.value.Ref, nil
}
func (f *scopeReaderFakeV2) InspectCurrentReviewValidationScopeV2(context.Context, runtimeports.ReviewValidationScopeCurrentSubjectV2, runtimeports.ReviewValidationScopeCurrentProjectionRefV2) (runtimeports.ReviewValidationScopeCurrentProjectionV2, error) {
	f.inspectCalls++
	out := f.value.Clone()
	if f.inspectCalls > 1 && f.mutateSecond != nil {
		f.mutateSecond(&out)
	}
	return out, nil
}
func (f *scopeReaderFakeV2) InspectHistoricalReviewValidationScopeV2(context.Context, runtimeports.ReviewValidationScopeCurrentProjectionRefV2) (runtimeports.ReviewValidationScopeCurrentProjectionV2, error) {
	return f.value.Clone(), nil
}

type evidenceReaderFakeV2 struct {
	value        runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	inspectCalls int
}

func (f *evidenceReaderFakeV2) ResolveReviewEvidenceApplicabilityCurrentV1(context.Context, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(f.value), nil
}
func (f *evidenceReaderFakeV2) InspectCurrentReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	f.inspectCalls++
	return runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(f.value), nil
}
func (f *evidenceReaderFakeV2) InspectHistoricalReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return runtimeports.CloneReviewEvidenceApplicabilityProjectionV1(f.value.Projection), nil
}

type routeResolverFakeV2 struct {
	artifact    runtimeports.ReviewArtifactResolvedRouteV2
	environment runtimeports.ReviewEnvironmentResolvedRouteV2
	scope       runtimeports.ReviewValidationScopeResolvedRouteV2
}

func (f routeResolverFakeV2) ResolveReviewArtifactReaderV2(context.Context, runtimeports.ReviewGroundingRouteRequestV2) (runtimeports.ReviewArtifactResolvedRouteV2, error) {
	return f.artifact, nil
}
func (f routeResolverFakeV2) ResolveReviewEnvironmentReaderV2(context.Context, runtimeports.ReviewGroundingRouteRequestV2) (runtimeports.ReviewEnvironmentResolvedRouteV2, error) {
	return f.environment, nil
}
func (f routeResolverFakeV2) ResolveReviewValidationScopeReaderV2(context.Context, runtimeports.ReviewGroundingRouteRequestV2) (runtimeports.ReviewValidationScopeResolvedRouteV2, error) {
	return f.scope, nil
}
