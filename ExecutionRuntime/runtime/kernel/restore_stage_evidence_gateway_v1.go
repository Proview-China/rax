package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageEvidenceGatewayV1 is the additive Restore Evidence path. It
// reads the Sandbox authoritative fact and Runtime source twice, then appends
// exactly once. It never invokes the Sandbox Provider or settles the effect.
type RestoreStageEvidenceGatewayV1 struct {
	Domains  ports.RestoreStageDomainEvidenceCurrentReaderV1
	Sources  ports.EvidenceSourceRegistrationReaderV1
	Evidence ports.EvidenceGovernancePortV2
	Clock    func() time.Time
}

func NewRestoreStageEvidenceGatewayV1(domains ports.RestoreStageDomainEvidenceCurrentReaderV1, sources ports.EvidenceSourceRegistrationReaderV1, evidence ports.EvidenceGovernancePortV2, clock func() time.Time) (*RestoreStageEvidenceGatewayV1, error) {
	for _, dependency := range []struct {
		value any
		name  string
	}{{domains, "Sandbox Restore Stage DomainResult Evidence current Reader"}, {sources, "Runtime Evidence source Reader"}, {evidence, "Runtime Evidence governance"}, {clock, "Restore Stage Evidence clock"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return nil, err
		}
	}
	return &RestoreStageEvidenceGatewayV1{Domains: domains, Sources: sources, Evidence: evidence, Clock: clock}, nil
}

func (g *RestoreStageEvidenceGatewayV1) PublishRestoreStageEvidenceV1(ctx context.Context, request ports.PublishRestoreStageEvidenceRequestV1) (ports.EvidenceRecordRefV2, error) {
	if err := restoreStageEvidenceContextV1(ctx); err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	now, err := g.nowV1(time.Time{})
	if err != nil || request.Validate(now) != nil {
		if err != nil {
			return ports.EvidenceRecordRefV2{}, err
		}
		return ports.EvidenceRecordRefV2{}, request.Validate(now)
	}
	key := restoreStageEvidenceSourceKeyV1(request.SourceRegistration)
	if existing, inspectErr := g.Evidence.InspectGovernedBySource(ctx, key); inspectErr == nil {
		if err := validateRestoreStagePublishedEvidenceV1(existing, request); err != nil {
			return ports.EvidenceRecordRefV2{}, err
		}
		return existing.Ref, nil
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.EvidenceRecordRefV2{}, inspectErr
	}

	first, err := g.readCurrentV1(ctx, request, now)
	if err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	fresh, err := g.nowV1(now)
	if err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	second, err := g.readCurrentV1(ctx, request, fresh)
	if err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	if first.domain.ProjectionDigest != second.domain.ProjectionDigest || first.sourceFactDigest != second.sourceFactDigest || first.sourceConfigurationDigest != second.sourceConfigurationDigest {
		return ports.EvidenceRecordRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence inputs changed between S1 and S2")
	}
	candidate, err := buildRestoreStageEvidenceCandidateV1(second.source, request, second.domain)
	if err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	record, appendErr := g.Evidence.AppendGoverned(ctx, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: request.SourceRegistration.Revision})
	if appendErr != nil {
		if !restoreStageEvidenceRecoverableV1(appendErr) {
			return ports.EvidenceRecordRefV2{}, appendErr
		}
		record, err = g.Evidence.InspectGovernedBySource(context.WithoutCancel(ctx), key)
		if err != nil {
			return ports.EvidenceRecordRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "Restore Stage Evidence append outcome cannot be inspected")
		}
	}
	if err := validateRestoreStageEvidenceRecordCandidateV1(record, candidate, request.DomainResult); err != nil {
		return ports.EvidenceRecordRefV2{}, err
	}
	return record.Ref, nil
}

func (g *RestoreStageEvidenceGatewayV1) InspectRestoreStageEvidenceV1(ctx context.Context, request ports.PublishRestoreStageEvidenceRequestV1) (ports.EvidenceLedgerRecordV2, error) {
	if err := restoreStageEvidenceContextV1(ctx); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if request.SourceRegistration.Validate() != nil || request.DomainResult.Validate() != nil {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage Evidence Inspect coordinates are invalid")
	}
	record, err := g.Evidence.InspectGovernedBySource(ctx, restoreStageEvidenceSourceKeyV1(request.SourceRegistration))
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	return record, validateRestoreStagePublishedEvidenceV1(record, request)
}

type restoreStageEvidenceInputsV1 struct {
	domain                    ports.RestoreStageDomainEvidenceCurrentProjectionV1
	source                    ports.EvidenceSourceRegistrationFactV2
	sourceFactDigest          core.Digest
	sourceConfigurationDigest core.Digest
}

func (g *RestoreStageEvidenceGatewayV1) readCurrentV1(ctx context.Context, request ports.PublishRestoreStageEvidenceRequestV1, now time.Time) (restoreStageEvidenceInputsV1, error) {
	domain, err := g.Domains.InspectRestoreStageDomainEvidenceCurrentV1(ctx, request.DomainResult)
	if err != nil || domain.ValidateCurrent(now) != nil || !ports.SameRestoreStageDomainResultFactRefV1(domain.Domain.Fact, request.DomainResult) {
		if err != nil {
			return restoreStageEvidenceInputsV1{}, err
		}
		return restoreStageEvidenceInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Sandbox Restore Stage Evidence projection is not exact current")
	}
	source, err := g.Sources.InspectSource(ctx, request.SourceRegistration.RegistrationID)
	if err != nil {
		return restoreStageEvidenceInputsV1{}, err
	}
	factDigest, configurationDigest, err := validateRestoreStageEvidenceSourceV1(source, request, now)
	if err != nil {
		return restoreStageEvidenceInputsV1{}, err
	}
	return restoreStageEvidenceInputsV1{domain: domain, source: source, sourceFactDigest: factDigest, sourceConfigurationDigest: configurationDigest}, nil
}

func validateRestoreStageEvidenceSourceV1(source ports.EvidenceSourceRegistrationFactV2, request ports.PublishRestoreStageEvidenceRequestV1, now time.Time) (core.Digest, core.Digest, error) {
	if err := source.Validate(); err != nil {
		return "", "", err
	}
	ref, err := control.NewEvidenceSourceRegistrationRefV1(source)
	if err != nil {
		return "", "", err
	}
	if ref != request.SourceRegistration || source.State != ports.EvidenceSourceActive || source.NextSourceSequence != 1 || now.IsZero() || !now.Before(time.Unix(0, source.ExpiresUnixNano)) || source.SourceID != ports.RestoreStageEvidenceSourceIDV1 || source.LedgerScope.Partition != ports.EvidencePartitionInstance || !source.LedgerScope.MatchesExecutionScope(request.DomainResult.Operation.ExecutionScope) || !ports.SameExecutionScopeV2(source.ExecutionScope, request.DomainResult.Operation.ExecutionScope) || source.Producer != ports.EvidenceProducerBindingRefV2(request.DomainResult.Owner) {
		return "", "", core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "Restore Stage Evidence source is not the exact dedicated current source")
	}
	allowedKind, allowedClass := false, false
	for _, kind := range source.AllowedKinds {
		allowedKind = allowedKind || kind == ports.RestoreStageEvidenceEventKindV1
	}
	for _, mapping := range source.ClassMappings {
		allowedClass = allowedClass || mapping.Class == ports.RestoreStageEvidenceClassV1 && mapping.Trust == ports.EvidenceTrustAuthoritativeFact
	}
	if !allowedKind || !allowedClass {
		return "", "", core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "Restore Stage Evidence source does not grant authoritative DomainResult evidence")
	}
	factDigest, err := source.DigestV2()
	if err != nil {
		return "", "", err
	}
	configurationDigest, err := source.ConfigurationDigestV2()
	return factDigest, configurationDigest, err
}

func buildRestoreStageEvidenceCandidateV1(source ports.EvidenceSourceRegistrationFactV2, request ports.PublishRestoreStageEvidenceRequestV1, domain ports.RestoreStageDomainEvidenceCurrentProjectionV1) (ports.EvidenceEventCandidateV2, error) {
	configuration, err := source.ConfigurationDigestV2()
	if err != nil {
		return ports.EvidenceEventCandidateV2{}, err
	}
	ownerFact := request.DomainResult.EvidenceOwnerFactV2()
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: source.LedgerScope, EventID: request.DomainResult.ID,
		RegistrationID: source.ID, RegistrationRevision: source.Revision, SourceConfigurationDigest: configuration, SourcePolicy: source.Policy,
		SourceID: source.SourceID, SourceEpoch: source.SourceEpoch, SourceSequence: 1, TrustClass: ports.EvidenceTrustAuthoritativeFact,
		EventKind: ports.RestoreStageEvidenceEventKindV1, CustomClass: ports.RestoreStageEvidenceClassV1, ExecutionScope: source.ExecutionScope,
		Payload: domain.Payload, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: request.Governance.RestoreAttempt.ID,
		Producer: source.Producer, Authority: source.Authority, OwnerFact: &ownerFact, ObservedUnixNano: request.DomainResult.AuthoritativeTime,
	}
	return candidate, candidate.Validate()
}

func validateRestoreStagePublishedEvidenceV1(record ports.EvidenceLedgerRecordV2, request ports.PublishRestoreStageEvidenceRequestV1) error {
	if err := ports.ValidateRestoreStageEvidenceRecordV1(record, request.DomainResult); err != nil {
		return err
	}
	candidate := record.Candidate
	if candidate.EventID != request.DomainResult.ID || candidate.RegistrationID != request.SourceRegistration.RegistrationID || candidate.RegistrationRevision != request.SourceRegistration.Revision || candidate.SourceConfigurationDigest != request.SourceRegistration.ConfigurationDigest || candidate.SourceID != request.SourceRegistration.SourceID || candidate.SourceEpoch != request.SourceRegistration.SourceEpoch || candidate.SourceSequence != 1 || candidate.EventKind != ports.RestoreStageEvidenceEventKindV1 || candidate.CustomClass != ports.RestoreStageEvidenceClassV1 || candidate.CorrelationID != request.Governance.RestoreAttempt.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence record belongs to another source or request")
	}
	return nil
}

func validateRestoreStageEvidenceRecordCandidateV1(record ports.EvidenceLedgerRecordV2, candidate ports.EvidenceEventCandidateV2, domain ports.RestoreStageDomainResultFactRefV1) error {
	if err := control.ValidateEvidenceLedgerRecordV2(record); err != nil {
		return err
	}
	digest, err := candidate.DigestV2()
	if err != nil || record.CandidateDigest != digest || !reflect.DeepEqual(record.Candidate, candidate) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence append returned different canonical content")
	}
	return ports.ValidateRestoreStageEvidenceRecordV1(record, domain)
}

func restoreStageEvidenceSourceKeyV1(ref ports.EvidenceSourceRegistrationRefV1) ports.EvidenceSourceKeyV2 {
	return ports.EvidenceSourceKeyV2{RegistrationID: ref.RegistrationID, SourceEpoch: ref.SourceEpoch, SourceSequence: 1}
}

func (g *RestoreStageEvidenceGatewayV1) nowV1(previous time.Time) (time.Time, error) {
	if g == nil || g.Clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Restore Stage Evidence clock is unavailable")
	}
	now := g.Clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore Stage Evidence clock regressed")
	}
	return now, nil
}

func restoreStageEvidenceContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage Evidence context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func restoreStageEvidenceRecoverableV1(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

var _ ports.RestoreStageEvidenceGovernancePortV1 = (*RestoreStageEvidenceGatewayV1)(nil)
