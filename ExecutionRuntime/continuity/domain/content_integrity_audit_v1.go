package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type ContentIntegrityAuditControllerV1 struct {
	repository contentIntegrityAuditRepositoryV1
	metadata   ports.MetadataStore
	content    ports.ContentStore
	owner      contract.OwnerBinding
	clock      Clock
}

// contentIntegrityAuditRepositoryV1 remains owner-local. It exposes no cleanup,
// purge, retention mutation, or provider operation.
type contentIntegrityAuditRepositoryV1 interface {
	ports.ContentIntegrityAuditReaderV1
	CreateContentIntegrityAuditFactV1(context.Context, contract.ContentIntegrityAuditFactV1) (contract.ContentIntegrityAuditFactV1, bool, error)
}

func NewContentIntegrityAuditControllerV1(repository contentIntegrityAuditRepositoryV1, metadata ports.MetadataStore, content ports.ContentStore, owner contract.OwnerBinding, clock Clock) (*ContentIntegrityAuditControllerV1, error) {
	if nilInterfaceV1(repository) || nilInterfaceV1(metadata) || nilInterfaceV1(content) || nilInterfaceV1(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "content_integrity_audit_controller", "repository, metadata, content, and clock are required")
	}
	if err := owner.Validate(); err != nil || owner.ComponentID != contract.ContinuityComponentID || owner.Capability != contract.ContentIntegrityAuditCapabilityV1 || owner.FactKind != "content_integrity_audit_fact_v1" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "owner_binding", "invalid Continuity Content Integrity Audit owner")
	}
	return &ContentIntegrityAuditControllerV1{repository: repository, metadata: metadata, content: content, owner: owner, clock: clock}, nil
}

func (c *ContentIntegrityAuditControllerV1) CreateContentIntegrityAuditV1(ctx context.Context, request ports.CreateContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	if c == nil || nilInterfaceV1(c.repository) || nilInterfaceV1(c.metadata) || nilInterfaceV1(c.content) || nilInterfaceV1(c.clock) {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrUnsupported, "content_integrity_audit_controller", "controller is not configured")
	}
	if err := request.Validate(); err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	byID := ports.InspectContentIntegrityAuditByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		AuditID: request.AuditID, Owner: c.owner,
	}
	if existing, inspectErr := c.repository.InspectContentIntegrityAuditByIDV1(ctx, byID); inspectErr == nil {
		if existing.RequestDigest == requestDigest && existing.IdempotencyKey == request.IdempotencyKey {
			return existing.Clone(), true, nil
		}
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "audit_id", "create-once audit changed request")
	} else if !contract.HasCode(inspectErr, contract.ErrNotFound) {
		return contract.ContentIntegrityAuditFactV1{}, false, normalizeContentIntegrityBoundaryErrorV1(inspectErr, "content_integrity_audit_repository")
	}

	subjects, err := contract.NormalizeContentIntegritySubjectsV1(request.Subjects)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	s1 := c.inspectSubjectsV1(ctx, request.Scope, subjects)
	s2 := c.inspectSubjectsV1(ctx, request.Scope, subjects)
	if !sameCanonicalV1(s1, s2) {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "content_integrity_s1_s2", "metadata, journal, or content changed during inspection")
	}
	fact, err := contract.NewContentIntegrityAuditFactV1(request.AuditID, request.IdempotencyKey, requestDigest, request.Scope, c.owner, subjects, s2, c.clock.Now())
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	stored, replay, err := c.repository.CreateContentIntegrityAuditFactV1(ctx, fact)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, normalizeContentIntegrityBoundaryErrorV1(err, "content_integrity_audit_repository")
	}
	return stored.Clone(), replay, nil
}

func (c *ContentIntegrityAuditControllerV1) InspectContentIntegrityAuditV1(ctx context.Context, request ports.InspectContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrUnsupported, "content_integrity_audit_reader", "reader is not configured")
	}
	return c.repository.InspectContentIntegrityAuditV1(ctx, request)
}

func (c *ContentIntegrityAuditControllerV1) InspectContentIntegrityAuditByIDV1(ctx context.Context, request ports.InspectContentIntegrityAuditByIDRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ContentIntegrityAuditFactV1{}, contract.NewError(contract.ErrUnsupported, "content_integrity_audit_reader", "reader is not configured")
	}
	return c.repository.InspectContentIntegrityAuditByIDV1(ctx, request)
}

func (c *ContentIntegrityAuditControllerV1) inspectSubjectsV1(ctx context.Context, scope contract.Scope, subjects []contract.ContentIntegritySubjectV1) []contract.ContentIntegrityFindingV1 {
	findings := make([]contract.ContentIntegrityFindingV1, len(subjects))
	for i := range subjects {
		findings[i] = c.inspectSubjectV1(ctx, scope, subjects[i])
	}
	return findings
}

func (c *ContentIntegrityAuditControllerV1) inspectSubjectV1(ctx context.Context, scope contract.Scope, subject contract.ContentIntegritySubjectV1) contract.ContentIntegrityFindingV1 {
	finding := contract.ContentIntegrityFindingV1{Subject: subject, Classification: contract.ContentIntegrityHealthy, DetailCode: "exact_integrity_verified"}
	manifest, visible, manifestErr := c.metadata.InspectObject(ctx, subject.ObjectID)
	journal, journalErr := c.metadata.InspectJournal(ctx, subject.JournalID)
	if manifestErr == nil {
		finding.ManifestDigest = manifest.Digest
		finding.Visible = visible
	}
	if journalErr == nil {
		finding.JournalRevision = journal.Revision
		finding.JournalState = journal.State
	}

	if manifestErr != nil {
		if contract.HasCode(manifestErr, contract.ErrNotFound) {
			finding.Classification = contract.ContentIntegrityMetadataAbsent
			finding.DetailCode = "object_metadata_absent"
		} else {
			finding.Classification = contract.ContentIntegrityIndeterminate
			finding.DetailCode = "object_metadata_unavailable"
		}
	}
	if journalErr != nil {
		if contract.HasCode(journalErr, contract.ErrNotFound) {
			if finding.Classification == contract.ContentIntegrityHealthy {
				finding.Classification = contract.ContentIntegrityJournalAbsent
				finding.DetailCode = "journal_absent"
			}
		} else {
			finding.Classification = contract.ContentIntegrityIndeterminate
			finding.DetailCode = "journal_unavailable"
		}
	}
	if manifestErr != nil || journalErr != nil {
		return finding
	}

	if err := manifest.Validate(); err != nil {
		finding.Classification = contract.ContentIntegrityCorruptContent
		finding.DetailCode = "manifest_invalid"
		return finding
	}
	if err := journal.Validate(); err != nil {
		finding.Classification = contract.ContentIntegrityIndeterminate
		finding.DetailCode = "journal_invalid"
		return finding
	}
	if manifest.ScopeDigest != scope.ExecutionScopeDigest {
		finding.Classification = contract.ContentIntegrityIndeterminate
		finding.DetailCode = "manifest_scope_mismatch"
		return finding
	}
	if subject.ExpectedManifestDigest != "" && manifest.Digest != subject.ExpectedManifestDigest {
		finding.Classification = contract.ContentIntegrityIndeterminate
		finding.DetailCode = "expected_manifest_drift"
		return finding
	}
	if journal.ObjectID != subject.ObjectID || journal.ManifestDigest != manifest.Digest || journal.ObjectDigest != manifest.ContentDigest {
		finding.Classification = contract.ContentIntegrityIndeterminate
		finding.DetailCode = "journal_manifest_binding_mismatch"
		return finding
	}

	assembled := make([]byte, 0, manifest.TotalLength)
	for _, ref := range manifest.Chunks {
		chunkFinding := contract.ContentIntegrityChunkFindingV1{Chunk: ref, Status: contract.ContentIntegrityChunkHealthy, DetailCode: "chunk_verified"}
		present, err := c.content.HasChunk(ctx, ref)
		if err != nil {
			chunkFinding.Status = contract.ContentIntegrityChunkIndeterminate
			chunkFinding.DetailCode = "chunk_presence_unavailable"
			finding.Classification = contract.ContentIntegrityIndeterminate
			finding.DetailCode = "chunk_inspection_indeterminate"
			finding.Chunks = append(finding.Chunks, chunkFinding)
			continue
		}
		if !present {
			chunkFinding.Status = contract.ContentIntegrityChunkMissing
			chunkFinding.DetailCode = "chunk_missing"
			if finding.Classification != contract.ContentIntegrityIndeterminate {
				finding.Classification = contract.ContentIntegrityDanglingReference
				finding.DetailCode = "referenced_chunk_missing"
			}
			finding.Chunks = append(finding.Chunks, chunkFinding)
			continue
		}
		data, err := c.content.GetChunk(ctx, ref)
		if err != nil {
			chunkFinding.Status = contract.ContentIntegrityChunkIndeterminate
			chunkFinding.DetailCode = "chunk_read_unavailable"
			finding.Classification = contract.ContentIntegrityIndeterminate
			finding.DetailCode = "chunk_inspection_indeterminate"
			finding.Chunks = append(finding.Chunks, chunkFinding)
			continue
		}
		if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
			chunkFinding.Status = contract.ContentIntegrityChunkCorrupt
			chunkFinding.DetailCode = "chunk_digest_mismatch"
			if finding.Classification != contract.ContentIntegrityIndeterminate {
				finding.Classification = contract.ContentIntegrityCorruptContent
				finding.DetailCode = "content_corrupt"
			}
		} else {
			assembled = append(assembled, data...)
		}
		finding.Chunks = append(finding.Chunks, chunkFinding)
	}
	if finding.Classification == contract.ContentIntegrityHealthy && (int64(len(assembled)) != manifest.TotalLength || contract.DigestBytes(assembled) != manifest.ContentDigest) {
		finding.Classification = contract.ContentIntegrityCorruptContent
		finding.DetailCode = "object_digest_mismatch"
	}
	if finding.Classification == contract.ContentIntegrityHealthy {
		switch {
		case visible && journal.State == contract.JournalClosed:
			// exact_integrity_verified remains.
		case !visible && journal.State != contract.JournalClosed:
			finding.Classification = contract.ContentIntegrityWriteIncomplete
			finding.DetailCode = "write_not_visible"
		default:
			finding.Classification = contract.ContentIntegrityIndeterminate
			finding.DetailCode = "visibility_journal_mismatch"
		}
	}
	return finding
}

func normalizeContentIntegrityBoundaryErrorV1(err error, field string) error {
	for _, code := range []contract.ErrorCode{
		contract.ErrInvalidArgument, contract.ErrContentDigestMismatch,
		contract.ErrRevisionConflict, contract.ErrNotFound, contract.ErrUnsupported,
		contract.ErrPreconditionFailed, contract.ErrUnavailable, contract.ErrIndeterminate,
	} {
		if contract.HasCode(err, code) {
			return err
		}
	}
	return contract.NewError(contract.ErrIndeterminate, field, "boundary returned an unclassified result; inspect the original audit coordinates")
}

var _ ports.ContentIntegrityAuditGovernancePortV1 = (*ContentIntegrityAuditControllerV1)(nil)
