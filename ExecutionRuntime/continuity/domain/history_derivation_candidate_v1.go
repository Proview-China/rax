package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type HistoryDerivationCandidateControllerV1 struct {
	repository contentDerivationRepositoryV1
	timeline   ports.HistoryDerivationTimelineReaderV1
	metadata   ports.MetadataStore
	content    ports.ContentStore
	owner      contract.OwnerBinding
	clock      Clock
}

type contentDerivationRepositoryV1 interface {
	ports.HistoryDerivationCandidateReaderV1
	CreateHistoryDerivationCandidateFactV1(context.Context, contract.HistoryDerivationCandidateFactV1) (contract.HistoryDerivationCandidateFactV1, bool, error)
}

func NewHistoryDerivationCandidateControllerV1(repository contentDerivationRepositoryV1, timeline ports.HistoryDerivationTimelineReaderV1, metadata ports.MetadataStore, content ports.ContentStore, owner contract.OwnerBinding, clock Clock) (*HistoryDerivationCandidateControllerV1, error) {
	if nilInterfaceV1(repository) || nilInterfaceV1(timeline) || nilInterfaceV1(metadata) || nilInterfaceV1(content) || nilInterfaceV1(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "history_derivation_controller", "repository, timeline, metadata, content, and clock are required")
	}
	if err := owner.Validate(); err != nil || owner.ComponentID != contract.ContinuityComponentID || owner.Capability != contract.HistoryDerivationCapabilityV1 || owner.FactKind != "history_derivation_candidate_fact_v1" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "owner_binding", "invalid Continuity History Derivation owner")
	}
	return &HistoryDerivationCandidateControllerV1{repository: repository, timeline: timeline, metadata: metadata, content: content, owner: owner, clock: clock}, nil
}

func (c *HistoryDerivationCandidateControllerV1) CreateHistoryDerivationCandidateV1(ctx context.Context, request ports.CreateHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	if c == nil || nilInterfaceV1(c.repository) || nilInterfaceV1(c.timeline) || nilInterfaceV1(c.metadata) || nilInterfaceV1(c.content) || nilInterfaceV1(c.clock) {
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrUnsupported, "history_derivation_controller", "controller is not configured")
	}
	if err := request.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	byID := ports.InspectHistoryDerivationCandidateByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		CandidateID: request.CandidateID, Owner: c.owner,
	}
	if existing, inspectErr := c.repository.InspectHistoryDerivationCandidateByIDV1(ctx, byID); inspectErr == nil {
		if existing.RequestDigest == requestDigest && existing.IdempotencyKey == request.IdempotencyKey {
			return existing.Clone(), true, nil
		}
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "candidate_id", "create-once History Derivation Candidate changed request")
	} else if !contract.HasCode(inspectErr, contract.ErrNotFound) {
		return contract.HistoryDerivationCandidateFactV1{}, false, normalizeHistoryDerivationBoundaryErrorV1(inspectErr, "history_derivation_repository")
	}

	s1, err := c.inspectSourcesV1(ctx, request)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	s2, err := c.inspectSourcesV1(ctx, request)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	if !sameCanonicalV1(s1, s2) {
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "history_derivation_s1_s2", "source Event or output object changed during inspection")
	}
	fact, err := contract.NewHistoryDerivationCandidateFactV1(request.CandidateID, request.IdempotencyKey, requestDigest, request.Scope, c.owner, request.Kind, s2.Events, s2.Output, c.clock.Now())
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	stored, replay, err := c.repository.CreateHistoryDerivationCandidateFactV1(ctx, fact)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, normalizeHistoryDerivationBoundaryErrorV1(err, "history_derivation_repository")
	}
	return stored.Clone(), replay, nil
}

type historyDerivationInspectionV1 struct {
	Events []contract.HistoryDerivationEventRefV1 `json:"events"`
	Output contract.ContentObjectRefV1            `json:"output"`
}

func (c *HistoryDerivationCandidateControllerV1) inspectSourcesV1(ctx context.Context, request ports.CreateHistoryDerivationCandidateRequestV1) (historyDerivationInspectionV1, error) {
	events := make([]contract.HistoryDerivationEventRefV1, 0, len(request.Sources))
	for _, source := range request.Sources {
		record, err := c.timeline.InspectByEvidence(ctx, source.EvidenceRecordRef)
		if err != nil {
			return historyDerivationInspectionV1{}, normalizeHistoryDerivationBoundaryErrorV1(err, "history_derivation_event")
		}
		if err := record.Validate(); err != nil {
			return historyDerivationInspectionV1{}, contract.NewError(contract.ErrContentDigestMismatch, "history_derivation_event", "stored Timeline Event failed validation")
		}
		if record.Candidate.Scope != request.Scope {
			return historyDerivationInspectionV1{}, contract.NewError(contract.ErrRevisionConflict, "history_derivation_scope", "Timeline Event belongs to another exact execution scope")
		}
		if record.EvidenceRecordDigest != source.ExpectedEvidenceRecordDigest || record.Candidate.Digest != source.ExpectedProjectionDigest {
			return historyDerivationInspectionV1{}, contract.NewError(contract.ErrRevisionConflict, "history_derivation_event", "Timeline Event changed expected exact digest")
		}
		events = append(events, contract.HistoryDerivationEventRefFromRecordV1(record))
	}
	output, _, err := inspectContentObjectExactV1(ctx, c.metadata, c.content, request.OutputObjectID, request.ExpectedOutputManifestDigest, request.Scope.ExecutionScopeDigest)
	if err != nil {
		return historyDerivationInspectionV1{}, err
	}
	return historyDerivationInspectionV1{Events: events, Output: output}, nil
}

func (c *HistoryDerivationCandidateControllerV1) InspectHistoryDerivationCandidateV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrUnsupported, "history_derivation_reader", "reader is not configured")
	}
	return c.repository.InspectHistoryDerivationCandidateV1(ctx, request)
}

func (c *HistoryDerivationCandidateControllerV1) InspectHistoryDerivationCandidateByIDV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateByIDRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrUnsupported, "history_derivation_reader", "reader is not configured")
	}
	return c.repository.InspectHistoryDerivationCandidateByIDV1(ctx, request)
}

func normalizeHistoryDerivationBoundaryErrorV1(err error, field string) error {
	for _, code := range []contract.ErrorCode{
		contract.ErrInvalidArgument, contract.ErrContentDigestMismatch, contract.ErrCrossStoreIndeterminate,
		contract.ErrRevisionConflict, contract.ErrNotFound, contract.ErrUnsupported,
		contract.ErrPreconditionFailed, contract.ErrUnavailable, contract.ErrIndeterminate,
	} {
		if contract.HasCode(err, code) {
			return err
		}
	}
	return contract.NewError(contract.ErrIndeterminate, field, "boundary returned an unclassified result; inspect the original derivation coordinates")
}

var _ ports.HistoryDerivationCandidateGovernancePortV1 = (*HistoryDerivationCandidateControllerV1)(nil)
