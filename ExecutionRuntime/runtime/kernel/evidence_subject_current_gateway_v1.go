package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// EvidenceSubjectCurrentGatewayV1 is the only public currentness reader. The
// Fact Owner remains historical/atomic storage and is deliberately not a
// structural implementation of ports.EvidenceSubjectCurrentReaderV1.
type EvidenceSubjectCurrentGatewayV1 struct {
	Facts                ports.EvidenceSubjectCurrentFactPortV1
	Records              ports.EvidenceSubjectRecordRegistrationCurrentReaderV1
	Policies             ports.EvidenceSourcePolicyReaderV2
	CurrentScopes        ports.ExecutionScopeFactReaderV2
	Bindings             ports.ProviderBindingCurrentnessPortV2
	Authority            ports.AuthorityFactReaderV2
	Presence             ports.EvidenceSubjectPresenceReadabilityCurrentReaderV1
	ConsumerAssociations ports.EvidenceSubjectConsumerAssociationCurrentReaderV1
	ConsumerAssociation  ports.EvidenceSubjectConsumerAssociationRefV1
	Clock                func() time.Time
}

func (g EvidenceSubjectCurrentGatewayV1) InspectEvidenceSubjectProjectionV1(ctx context.Context, ref ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectCurrentProjectionV1, error) {
	if err := g.validateDependencies(); err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	projection, err := g.Facts.InspectEvidenceSubjectProjectionFactV1(ctx, ref)
	if err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	if err := projection.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	if projection.Ref != ref {
		return ports.EvidenceSubjectCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject historical backend returned another Projection")
	}
	return ports.CloneEvidenceSubjectCurrentProjectionV1(projection), nil
}

func (g EvidenceSubjectCurrentGatewayV1) InspectEvidenceSubjectCurrentV1(ctx context.Context, request ports.EvidenceSubjectCurrentLookupRequestV1) (ports.EvidenceSubjectCurrentSnapshotV1, error) {
	if err := g.validateDependencies(); err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	return g.inspectCurrent(ctx, request, nil)
}

func (g EvidenceSubjectCurrentGatewayV1) ValidateEvidenceSubjectCurrentV1(ctx context.Context, request ports.EvidenceSubjectCurrentValidationRequestV1) (ports.EvidenceSubjectCurrentSnapshotV1, error) {
	if err := g.validateDependencies(); err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	lookup := ports.EvidenceSubjectCurrentLookupRequestV1{ContractVersion: request.ContractVersion, Subject: request.Subject, ExpectedConsumer: request.ExpectedConsumer, ExpectedExecutionScopeDigest: request.ExpectedExecutionScopeDigest, ExpectedSourcePolicy: request.ExpectedSourcePolicy}
	return g.inspectCurrent(ctx, lookup, &request)
}

func (g EvidenceSubjectCurrentGatewayV1) inspectCurrent(ctx context.Context, request ports.EvidenceSubjectCurrentLookupRequestV1, expected *ports.EvidenceSubjectCurrentValidationRequestV1) (ports.EvidenceSubjectCurrentSnapshotV1, error) {
	now1 := g.Clock()
	if now1.IsZero() {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Evidence subject current validation requires injected time")
	}
	s1, err := g.readCurrentClosure(ctx, request, now1)
	if err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	if expected != nil {
		if s1.snapshot.Projection.Ref != expected.ExpectedProjection || s1.snapshot.CurrentIndex != expected.ExpectedCurrentIndex || s1.registration != expected.ExpectedRegistration || s1.readerBinding != expected.ExpectedReaderBinding || s1.readerBinding.Capability != expected.ExpectedReaderCapability {
			return ports.EvidenceSubjectCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "Evidence subject expected current coordinates are stale")
		}
	}
	now2 := g.Clock()
	if now2.IsZero() || now2.Before(now1) {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Evidence subject S2 clock regressed")
	}
	s2, err := g.readCurrentClosure(ctx, request, now2)
	if err != nil {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.EvidenceSubjectCurrentSnapshotV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceConflict, "Evidence subject S1 and S2 current closures differ")
	}
	return cloneEvidenceSubjectSnapshotV1(s1.snapshot), nil
}

type evidenceSubjectCurrentClosureV1 struct {
	snapshot      ports.EvidenceSubjectCurrentSnapshotV1
	association   ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1
	record        ports.EvidenceSubjectRecordRegistrationCurrentResultV1
	registration  ports.EvidenceSourceRegistrationRefV1
	policy        ports.EvidenceSourcePolicyFactV2
	scope         ports.ExecutionScopeCurrentFactV2
	producer      ports.ProviderBindingCurrentProjectionV2
	authority     ports.DispatchAuthorityFactV2
	policyAuth    ports.DispatchAuthorityFactV2
	readerBinding ports.EvidenceSubjectReaderBindingRefV1
	presence      ports.EvidenceSubjectPresenceReadabilityCurrentResultV1
}

func (g EvidenceSubjectCurrentGatewayV1) readCurrentClosure(ctx context.Context, request ports.EvidenceSubjectCurrentLookupRequestV1, now time.Time) (evidenceSubjectCurrentClosureV1, error) {
	association, err := g.ConsumerAssociations.InspectEvidenceSubjectConsumerAssociationCurrentV1(ctx, g.ConsumerAssociation)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := association.Validate(now); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if association.Ref != g.ConsumerAssociation || association.Consumer != request.ExpectedConsumer || association.ExecutionScopeDigest != request.ExpectedExecutionScopeDigest {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Evidence subject bound Consumer association does not authorize this request")
	}
	recordRequest := ports.EvidenceSubjectRecordRegistrationCurrentRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: request.Subject}
	record, err := g.Records.InspectEvidenceSubjectRecordRegistrationCurrentV1(ctx, recordRequest)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := record.Validate(now); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := control.ValidateEvidenceLedgerRecordV2(record.Record); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(request.Subject)
	index, err := g.Facts.InspectEvidenceSubjectCurrentIndexV1(ctx, subjectDigest)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := index.Validate(); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	projection, err := g.Facts.InspectEvidenceSubjectProjectionFactV1(ctx, index.CurrentProjection)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	snapshot := ports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
	if err := snapshot.Validate(); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if projection.Subject != request.Subject || projection.Consumer != request.ExpectedConsumer || projection.ExecutionScopeDigest != request.ExpectedExecutionScopeDigest || projection.SourcePolicy != request.ExpectedSourcePolicy {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject current Projection belongs to another request")
	}
	registration, err := validateEvidenceSubjectRecordProjectionV1(record, projection)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	policy, err := g.Policies.InspectEvidenceSourcePolicy(ctx, projection.SourcePolicy.Ref)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := validateEvidenceSubjectPolicyCurrentV1(policy, projection, now); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	scope, err := g.CurrentScopes.InspectCurrentExecutionScope(ctx, projection.CurrentScope.Ref)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := validateEvidenceSubjectScopeCurrentV1(scope, projection, now); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	producer, err := g.Bindings.InspectProviderBindingCurrentV2(ctx, ports.ProviderBindingRefV2(projection.Producer))
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := producer.ValidateCurrent(ports.ProviderBindingRefV2(projection.Producer), now); err != nil || !reflect.DeepEqual(producer, projection.ProducerBindingCurrent) {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Evidence subject Producer Binding drifted")
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, projection.Authority.Ref)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := authority.ValidateCurrent(projection.Authority, projection.ExecutionScope, projection.ActionScopeDigest, now); err != nil || !reflect.DeepEqual(authority, projection.AuthorityCurrent) {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "Evidence subject source Authority drifted")
	}
	policyAuth, err := g.Authority.InspectDispatchAuthority(ctx, projection.SourcePolicyAuthority.Ref)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := policyAuth.ValidateCurrent(projection.SourcePolicyAuthority, projection.ExecutionScope, projection.ActionScopeDigest, now); err != nil || !reflect.DeepEqual(policyAuth, projection.SourcePolicyAuthorityCurrent) {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "Evidence subject Policy Authority drifted")
	}
	readerBinding, err := ports.EvidenceSubjectReaderBindingFromCurrentV1(association.BindingCurrent)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if readerBinding != projection.ReaderBinding || readerBinding.Capability != projection.ReaderCapability {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Evidence subject Reader Binding drifted")
	}
	presenceRequest := ports.EvidenceSubjectPresenceReadabilityCurrentRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: request.Subject, ExpectedConsumer: request.ExpectedConsumer, ExpectedExecutionScopeDigest: request.ExpectedExecutionScopeDigest, ExpectedOwnerWatermark: projection.Ref.OwnerWatermark}
	presence, err := g.Presence.InspectEvidenceSubjectPresenceReadabilityCurrentV1(ctx, presenceRequest)
	if err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if err := presence.Validate(presenceRequest, now); err != nil {
		return evidenceSubjectCurrentClosureV1{}, err
	}
	if presence.Presence != projection.Presence || presence.Readability != projection.Readability || !reflect.DeepEqual(presence.Tombstone, projection.Tombstone) || !reflect.DeepEqual(presence.TombstoneAbsence, projection.TombstoneAbsence) || presence.ReadabilityPolicy != projection.ReadabilityPolicy {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Presence or Readability drifted")
	}
	expires := minimumUnixNanoV1(record.ExpiresUnixNano, policy.ExpiresUnixNano, scope.ExpiresUnixNano, producer.ExpiresUnixNano, authority.ExpiresUnixNano, policyAuth.ExpiresUnixNano, association.ExpiresUnixNano, presence.ExpiresUnixNano, projection.ReadabilityPolicy.ExpiresUnixNano)
	if projection.ExpiresUnixNano != expires || now.Before(time.Unix(0, projection.CheckedUnixNano)) || !now.Before(time.Unix(0, expires)) {
		return evidenceSubjectCurrentClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Evidence subject natural TTL drifted or expired")
	}
	return evidenceSubjectCurrentClosureV1{snapshot: snapshot, association: association, record: record, registration: registration, policy: policy, scope: scope, producer: producer, authority: authority, policyAuth: policyAuth, readerBinding: readerBinding, presence: presence}, nil
}

func (g EvidenceSubjectCurrentGatewayV1) validateDependencies() error {
	for _, dependency := range []any{g.Facts, g.Records, g.Policies, g.CurrentScopes, g.Bindings, g.Authority, g.Presence, g.ConsumerAssociations} {
		if nilOrTypedNilEvidenceSubjectV1(dependency) {
			return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Evidence subject current Gateway dependency is unavailable")
		}
	}
	if g.Clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Evidence subject current Gateway clock is unavailable")
	}
	return g.ConsumerAssociation.Validate()
}

func validateEvidenceSubjectPolicyCurrentV1(f ports.EvidenceSourcePolicyFactV2, p ports.EvidenceSubjectCurrentProjectionV1, now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	digest, err := f.DigestV2()
	if err != nil {
		return err
	}
	if f.Ref != p.SourcePolicy.Ref || f.Revision != p.SourcePolicy.Revision || digest != p.SourcePolicy.Digest || f.State != ports.EvidenceSourcePolicyActive || p.SourcePolicyState != f.State || f.Producer != p.Producer || f.PolicyOwner != p.SourcePolicyOwner || f.PolicyAuthority != p.SourcePolicyAuthority || f.ActionScopeDigest != p.ActionScopeDigest || !ports.SameExecutionScopeV2(f.PolicyScope, p.ExecutionScope) || p.SourcePolicyExpiresUnixNano != f.ExpiresUnixNano || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "Evidence subject Source Policy drifted or expired")
	}
	return nil
}

// validateEvidenceSubjectRecordProjectionV1 proves that every Candidate and
// Registration field carried by the sealed current Projection came from the
// exact immutable Ledger record. CandidateDigest alone is not sufficient:
// without these comparisons a projection could retain the real digest while
// type-punning trust, class, payload, Owner fact or scope semantics.
func validateEvidenceSubjectRecordProjectionV1(r ports.EvidenceSubjectRecordRegistrationCurrentResultV1, p ports.EvidenceSubjectCurrentProjectionV1) (ports.EvidenceSourceRegistrationRefV1, error) {
	record := r.Record
	candidate := record.Candidate
	registration := r.Registration
	registrationRef, err := control.NewEvidenceSourceRegistrationRefV1(registration)
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	ledgerScopeDigest, err := candidate.LedgerScope.DigestV2()
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}
	executionScopeDigest, err := ports.ExecutionScopeDigestV2(candidate.ExecutionScope)
	if err != nil {
		return ports.EvidenceSourceRegistrationRefV1{}, err
	}

	expectedSource := ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}
	if record.Ref != p.Record || record.CandidateDigest != candidateDigest || record.CandidateDigest != p.CandidateDigest || record.PreviousRecordDigest != p.PreviousRecordDigest || record.IngestedUnixNano != p.IngestedUnixNano ||
		p.Source != expectedSource || p.Subject.Source != expectedSource ||
		p.LedgerScope != candidate.LedgerScope || p.LedgerScopeDigest != ledgerScopeDigest || p.LedgerScopeDigest != record.Ref.LedgerScopeDigest ||
		!ports.SameExecutionScopeV2(p.ExecutionScope, candidate.ExecutionScope) || p.ExecutionScopeDigest != executionScopeDigest ||
		p.TrustClass != candidate.TrustClass || p.ClaimKind != candidate.ClaimKind || p.EventKind != candidate.EventKind || p.CustomClass != candidate.CustomClass || p.Payload != candidate.Payload ||
		!sameEvidenceSubjectCausationV1(p.Causation, candidate.Causation) || p.CorrelationID != candidate.CorrelationID || !reflect.DeepEqual(p.OwnerFact, candidate.OwnerFact) || !reflect.DeepEqual(p.HistoricalSource, candidate.HistoricalSource) || p.ObservedUnixNano != candidate.ObservedUnixNano {
		return ports.EvidenceSourceRegistrationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Projection drifted from exact Ledger Candidate")
	}

	// RegistrationRevision, SourceConfigurationDigest, Producer, Authority and
	// SourcePolicy in the Candidate name the historical append-time source.
	// The current Registration may legally advance those watermarks, so only
	// immutable source coordinates are compared across that boundary. The
	// Projection's current-governance fields, however, must equal the current
	// Registration exactly.
	if candidate.RegistrationID != registration.ID || candidate.SourceID != registration.SourceID || candidate.SourceEpoch != registration.SourceEpoch ||
		candidate.LedgerScope != registration.LedgerScope || !ports.SameExecutionScopeV2(candidate.ExecutionScope, registration.ExecutionScope) ||
		registrationRef != p.Registration || p.RegistrationState != registration.State || p.RegistrationExpiresUnixNano != registration.ExpiresUnixNano ||
		p.LedgerScope != registration.LedgerScope || !ports.SameExecutionScopeV2(p.ExecutionScope, registration.ExecutionScope) || p.CurrentScope != registration.CurrentScope || p.CurrentScopeWatermark != registration.CurrentScopeWatermark || p.Producer != registration.Producer || p.Authority != registration.Authority || p.ActionScopeDigest != registration.ActionScopeDigest || p.SourcePolicy != registration.Policy {
		return ports.EvidenceSourceRegistrationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Projection or Candidate drifted from exact Source Registration")
	}
	return registrationRef, nil
}

func sameEvidenceSubjectCausationV1(left, right []ports.EvidenceCausationRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateEvidenceSubjectScopeCurrentV1(f ports.ExecutionScopeCurrentFactV2, p ports.EvidenceSubjectCurrentProjectionV1, now time.Time) error {
	if f.Ref != p.CurrentScope.Ref || f.Revision != p.CurrentScope.Revision || f.Digest != p.CurrentScope.Digest || f.State != ports.ExecutionScopeFactActive || f.ProjectionWatermark != p.CurrentScopeWatermark || !ports.SameExecutionScopeV2(f.Scope, p.ExecutionScope) || !reflect.DeepEqual(f, p.ExecutionScopeCurrent) || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "Evidence subject Execution Scope drifted or expired")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Execution Scope digest drifted")
	}
	return nil
}

func minimumUnixNanoV1(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value <= 0 {
			return 0
		}
		if minimum == 0 || value < minimum {
			minimum = value
		}
	}
	return minimum
}

func nilOrTypedNilEvidenceSubjectV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func cloneEvidenceSubjectSnapshotV1(value ports.EvidenceSubjectCurrentSnapshotV1) ports.EvidenceSubjectCurrentSnapshotV1 {
	value.Projection = ports.CloneEvidenceSubjectCurrentProjectionV1(value.Projection)
	if value.CurrentIndex.PreviousProjection != nil {
		copy := *value.CurrentIndex.PreviousProjection
		value.CurrentIndex.PreviousProjection = &copy
	}
	return value
}

var _ ports.EvidenceSubjectCurrentReaderV1 = EvidenceSubjectCurrentGatewayV1{}
