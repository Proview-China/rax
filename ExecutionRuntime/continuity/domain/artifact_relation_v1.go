package domain

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type ArtifactRelationControllerV1 struct {
	repository   artifactRelationRepositoryV1
	timeline     ports.ArtifactTimelineReaderV1
	sourceReader ports.ArtifactRelationSourceReaderV1
	owner        contract.OwnerBinding
	clock        Clock
}

// artifactRelationRepositoryV1 is deliberately owner-local. Public callers
// receive the Governance controller and cannot treat a raw store as an Attach
// API.
type artifactRelationRepositoryV1 interface {
	ports.ArtifactRelationReaderV1
	CreateArtifactRelationFactV1(context.Context, contract.ArtifactRelationFactV1) (contract.ArtifactRelationFactV1, bool, error)
}

func NewArtifactRelationControllerV1(repository artifactRelationRepositoryV1, timeline ports.ArtifactTimelineReaderV1, sourceReader ports.ArtifactRelationSourceReaderV1, owner contract.OwnerBinding, clock Clock) (*ArtifactRelationControllerV1, error) {
	if nilInterfaceV1(repository) || nilInterfaceV1(timeline) || nilInterfaceV1(sourceReader) || nilInterfaceV1(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "artifact_relation_controller", "repository, timeline, typed source reader, and clock are required")
	}
	if err := owner.Validate(); err != nil || owner.ComponentID != contract.ContinuityComponentID || owner.Capability != contract.ArtifactRelationCapabilityV1 || owner.FactKind != "artifact_relation_fact_v1" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "owner_binding", "invalid Continuity Artifact Relation owner")
	}
	return &ArtifactRelationControllerV1{repository: repository, timeline: timeline, sourceReader: sourceReader, owner: owner, clock: clock}, nil
}

func (c *ArtifactRelationControllerV1) CreateArtifactRelationV1(ctx context.Context, request ports.CreateArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, bool, error) {
	if c == nil || nilInterfaceV1(c.repository) || nilInterfaceV1(c.timeline) || nilInterfaceV1(c.sourceReader) || nilInterfaceV1(c.clock) {
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrUnsupported, "artifact_relation_controller", "controller is not configured")
	}
	if err := request.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}

	byID := ports.InspectArtifactRelationByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		RelationID: request.RelationID, Owner: c.owner,
	}
	if existing, err := c.repository.InspectArtifactRelationByIDV1(ctx, byID); err == nil {
		if artifactRelationMatchesRequestV1(existing, request) {
			return existing.Clone(), true, nil
		}
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "relation_id", "create-once relation changed content")
	} else if !contract.HasCode(err, contract.ErrNotFound) {
		return contract.ArtifactRelationFactV1{}, false, normalizeArtifactBoundaryErrorV1(err, "artifact_relation_repository")
	}

	sourceRequest := ports.ArtifactRelationSourceRequestV1{
		ArtifactFactRef: request.ArtifactFactRef, RelatedFactRef: request.RelatedFactRef,
		Kind: request.Kind, EvidenceRecordRef: request.EvidenceRecordRef,
		ExecutionScopeDigest:        request.Scope.ExecutionScopeDigest,
		ExpectedSourceProjectionRef: cloneExactRefV1(request.ExpectedSourceProjectionRef),
	}
	sourceS1, eventS1, err := c.inspectSourcesV1(ctx, request, sourceRequest)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	sourceS2, eventS2, err := c.inspectSourcesV1(ctx, request, sourceRequest)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	if !sameCanonicalV1(sourceS1, sourceS2) || !sameCanonicalV1(eventS1, eventS2) {
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "artifact_relation_s1_s2", "source projection or Timeline Event drifted during inspection")
	}
	fact, err := contract.NewArtifactRelationFactV1(request.RelationID, request.IdempotencyKey, request.Scope, c.owner, sourceS2, c.clock.Now())
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	stored, replay, err := c.repository.CreateArtifactRelationFactV1(ctx, fact)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, normalizeArtifactBoundaryErrorV1(err, "artifact_relation_repository")
	}
	return stored.Clone(), replay, nil
}

func (c *ArtifactRelationControllerV1) InspectArtifactRelationV1(ctx context.Context, request ports.InspectArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrUnsupported, "artifact_relation_reader", "reader is not configured")
	}
	return c.repository.InspectArtifactRelationV1(ctx, request)
}

func (c *ArtifactRelationControllerV1) InspectArtifactRelationByIDV1(ctx context.Context, request ports.InspectArtifactRelationByIDRequestV1) (contract.ArtifactRelationFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrUnsupported, "artifact_relation_reader", "reader is not configured")
	}
	return c.repository.InspectArtifactRelationByIDV1(ctx, request)
}

func (c *ArtifactRelationControllerV1) ListArtifactRelationsV1(ctx context.Context, request ports.ListArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return nil, contract.NewError(contract.ErrUnsupported, "artifact_relation_reader", "reader is not configured")
	}
	return c.repository.ListArtifactRelationsV1(ctx, request)
}

func (c *ArtifactRelationControllerV1) ListRelatedArtifactRelationsV1(ctx context.Context, request ports.ListRelatedArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return nil, contract.NewError(contract.ErrUnsupported, "artifact_relation_reader", "reader is not configured")
	}
	return c.repository.ListRelatedArtifactRelationsV1(ctx, request)
}

func (c *ArtifactRelationControllerV1) inspectSourcesV1(ctx context.Context, request ports.CreateArtifactRelationRequestV1, sourceRequest ports.ArtifactRelationSourceRequestV1) (contract.ArtifactRelationSourceProjectionV1, contract.TimelineEventRecord, error) {
	source, err := c.sourceReader.InspectArtifactRelationSourceV1(ctx, sourceRequest)
	if err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, normalizeArtifactBoundaryErrorV1(err, "artifact_source_reader")
	}
	if err := source.Validate(); err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrEvidenceNotInspectable, "artifact_source_projection", "typed owner reader returned an invalid projection")
	}
	if !source.Artifact.ArtifactFactRef.Equal(request.ArtifactFactRef) || !source.RelatedFactRef.Equal(request.RelatedFactRef) ||
		source.Kind != request.Kind || source.EvidenceRecordRef != request.EvidenceRecordRef ||
		source.ExecutionScopeDigest != request.Scope.ExecutionScopeDigest {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrRevisionConflict, "artifact_source_projection", "typed owner projection does not match request coordinates")
	}
	if request.ExpectedSourceProjectionRef != nil && !source.SourceProjectionRef.Equal(*request.ExpectedSourceProjectionRef) {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrRevisionConflict, "source_projection_ref", "typed owner projection changed expected exact ref")
	}
	event, err := c.timeline.InspectByEvidence(ctx, request.EvidenceRecordRef)
	if err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, normalizeArtifactBoundaryErrorV1(err, "artifact_timeline_reader")
	}
	if err := event.Validate(); err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_event", "Timeline reader returned an invalid Event")
	}
	if event.EvidenceRecordRef != request.EvidenceRecordRef || event.EvidenceRecordDigest != source.EvidenceRecordDigest || event.Candidate.Scope != request.Scope {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrRevisionConflict, "artifact_evidence", "Timeline Event does not exact-bind the owner projection and request scope")
	}
	if !eventReferencesV1(event, source.Artifact.ArtifactFactRef.ID) && !eventReferencesV1(event, source.Artifact.StorageRef) {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrEvidenceNotInspectable, "artifact_ref", "Timeline Event does not reference the artifact")
	}
	if !eventReferencesV1(event, source.RelatedFactRef.ID) {
		return contract.ArtifactRelationSourceProjectionV1{}, contract.TimelineEventRecord{}, contract.NewError(contract.ErrEvidenceNotInspectable, "related_fact_ref", "Timeline Event does not reference the related fact")
	}
	return source.Clone(), event.Clone(), nil
}

func artifactRelationMatchesRequestV1(fact contract.ArtifactRelationFactV1, request ports.CreateArtifactRelationRequestV1) bool {
	return fact.RelationID == request.RelationID && fact.IdempotencyKey == request.IdempotencyKey && fact.Scope == request.Scope &&
		fact.SourceProjection.Artifact.ArtifactFactRef.Equal(request.ArtifactFactRef) &&
		fact.SourceProjection.RelatedFactRef.Equal(request.RelatedFactRef) && fact.SourceProjection.Kind == request.Kind &&
		fact.SourceProjection.EvidenceRecordRef == request.EvidenceRecordRef &&
		(request.ExpectedSourceProjectionRef == nil || fact.SourceProjection.SourceProjectionRef.Equal(*request.ExpectedSourceProjectionRef))
}

func eventReferencesV1(event contract.TimelineEventRecord, target string) bool {
	if contract.Contains(event.Candidate.ObjectRefs, target) || contract.Contains(event.Candidate.ParentRefs, target) || contract.Contains(event.Candidate.CausationRefs, target) {
		return true
	}
	return event.Candidate.CorrelationID == target
}

func sameCanonicalV1(left, right any) bool {
	leftDigest, leftErr := contract.CanonicalDigest(left)
	rightDigest, rightErr := contract.CanonicalDigest(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func cloneExactRefV1(value *contract.ExactFactRefV2) *contract.ExactFactRefV2 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nilInterfaceV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func normalizeArtifactBoundaryErrorV1(err error, field string) error {
	for _, code := range []contract.ErrorCode{
		contract.ErrInvalidArgument, contract.ErrEvidenceNotInspectable,
		contract.ErrContentDigestMismatch, contract.ErrRevisionConflict,
		contract.ErrNotFound, contract.ErrUnsupported, contract.ErrPreconditionFailed,
		contract.ErrUnavailable, contract.ErrIndeterminate,
	} {
		if contract.HasCode(err, code) {
			return err
		}
	}
	return contract.NewError(contract.ErrIndeterminate, field, "boundary returned an unclassified result; inspect the original coordinates")
}

var _ ports.ArtifactRelationGovernancePortV1 = (*ArtifactRelationControllerV1)(nil)
