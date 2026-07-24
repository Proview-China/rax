package kernel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type ContextTurnRefreshServiceV1 struct {
	parentCurrent *ParentFrameCurrentReaderV1
	toolCurrent   settledActionContextSourceCurrentReaderV1
	owner         contextports.ContextTurnRefreshOwnerBackendV1
	content       ReferenceStore
	clock         func() time.Time
}

// settledActionContextSourceCurrentReaderV1 is an Owner-local consumer seam.
// It is deliberately not the Application-owned public Port; a future
// Application adapter must translate its public DTO into this domain input.
type settledActionContextSourceCurrentReaderV1 interface {
	InspectSettledActionContextSourceCurrentV1(context.Context, contract.SettledActionContextSourceRequestV1) (contract.SettledActionContextSourceCurrentV1, error)
}

func NewContextTurnRefreshServiceV1(
	owner contextports.ContextTurnRefreshOwnerBackendV1,
	toolCurrent settledActionContextSourceCurrentReaderV1,
	content ReferenceStore,
	clock func() time.Time,
	parentCurrentTTL time.Duration,
) (*ContextTurnRefreshServiceV1, error) {
	if owner == nil || toolCurrent == nil || content == nil || clock == nil {
		return nil, fmt.Errorf("%w: context turn refresh dependencies", contract.ErrInvalid)
	}
	parentCurrent, err := NewParentFrameCurrentReaderV1(owner, owner, owner, owner, owner, content, clock, parentCurrentTTL)
	if err != nil {
		return nil, err
	}
	return &ContextTurnRefreshServiceV1{parentCurrent: parentCurrent, toolCurrent: toolCurrent, owner: owner, content: content, clock: clock}, nil
}

type refreshSnapshotV1 struct {
	Parent     contract.ContextParentFrameCurrentProjectionV1
	Tool       contract.SettledActionContextSourceCurrentV1
	Binding    contract.ContextParentFrameSourceBindingV1
	Frame      contract.ContextFrame
	Manifest   contract.ContextManifest
	Generation contract.ContextGeneration
	Pointer    contract.ContextGenerationCurrentPointerV1
}

func (s *ContextTurnRefreshServiceV1) RefreshContextTurnV1(ctx context.Context, request contract.ContextTurnRefreshRequestV1) (contract.ContextTurnRefreshPreparedV1, error) {
	if err := checkRefreshContext(ctx); err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	if s == nil || request.Validate() != nil {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: context turn refresh", contract.ErrInvalid)
	}
	attempt := contract.FactRef{ID: request.RefreshAttemptID, Revision: 1, Digest: request.Digest}
	if _, inspectErr := s.owner.InspectContextTurnRefreshV1(ctx, contract.InspectContextTurnRefreshRequestV1{AttemptRef: attempt}); inspectErr == nil {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: %w: refresh attempt already exists; inspect original attempt", contract.ErrInspectOnly, contract.ErrConflict)
	} else if !errors.Is(inspectErr, contract.ErrNotFound) {
		return contract.ContextTurnRefreshPreparedV1{}, inspectErr
	}
	now, err := s.nowAfter(request.CheckedUnixNano)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	if now >= request.NotAfterUnixNano {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: refresh request TTL", contract.ErrExpired)
	}
	snapshot, err := s.readSnapshot(ctx, request, now)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	record, err := s.freeze(request, snapshot, now)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	if err := checkRefreshContext(ctx); err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	return s.owner.ReserveContextTurnRefreshV1(ctx, record)
}

func (s *ContextTurnRefreshServiceV1) ApplyContextTurnRefreshV1(ctx context.Context, request contract.ApplyContextTurnRefreshRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	if err := checkRefreshContext(ctx); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if s == nil {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: context turn refresh service", contract.ErrInvalid)
	}
	now := s.clock()
	if now.IsZero() {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh clock", contract.ErrInvalid)
	}
	nowNano := now.UnixNano()
	if nowNano < request.CheckedUnixNano {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh clock rollback", contract.ErrConflict)
	}
	if err := request.ValidateAt(nowNano); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	existing, inspectErr := s.owner.InspectContextTurnRefreshV1(ctx, contract.InspectContextTurnRefreshRequestV1{AttemptRef: request.AttemptRef})
	if inspectErr != nil {
		return contract.ContextTurnRefreshResultV1{}, inspectErr
	}
	switch existing.Status {
	case contract.ContextTurnRefreshAppliedV1:
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: %w: refresh already applied; inspect original attempt", contract.ErrInspectOnly, contract.ErrConflict)
	case contract.ContextTurnRefreshPendingV1:
		// Only an exact pending result may advance to the fresh-read and CAS path.
	default:
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh inspect state", contract.ErrConflict)
	}
	record, err := s.owner.LoadContextTurnRefreshPendingRecordV1(ctx, request.AttemptRef)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if request.PendingDomainResultRef != mustPendingRef(record.Pending) || request.ExpectedCurrent != record.Request.ExpectedCurrent || nowNano < record.Pending.CreatedUnixNano {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: apply refresh exact binding or clock rollback", contract.ErrConflict)
	}
	if nowNano >= record.Pending.ExpiresUnixNano {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: apply refresh TTL", contract.ErrExpired)
	}
	s2, err := s.readSnapshot(ctx, record.Request, nowNano)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if !sameParentCurrent(record.ParentProjection, s2.Parent) || record.ToolProjection.SourceDigest != s2.Tool.SourceDigest {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh S2 current drift", contract.ErrConflict)
	}
	completed := s.clock()
	if completed.IsZero() || completed.Before(now) || completed.UnixNano() >= record.Pending.ExpiresUnixNano || completed.UnixNano() >= request.NotAfterUnixNano {
		if !completed.IsZero() && completed.Before(now) {
			return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh clock rollback", contract.ErrConflict)
		}
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh S2 TTL crossing", contract.ErrExpired)
	}
	if err := checkRefreshContext(ctx); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	settlement := contract.ContextTurnRefreshApplySettlementV1{
		ContractVersion:        contract.Version,
		ID:                     "ctx-refresh-apply-" + request.AttemptRef.ID,
		Revision:               1,
		AttemptRef:             request.AttemptRef,
		PendingDomainResultRef: request.PendingDomainResultRef,
		TransitionProofRef:     request.TransitionProofRef,
		StableSourceSetDigest:  request.StableSourceSetDigest,
		S2AssociationSetDigest: request.S2AssociationSetDigest,
		PreviousCurrentDigest:  request.ExpectedCurrent.Digest,
		CurrentGenerationRef:   record.Pending.GenerationRef,
		AppliedUnixNano:        completed.UnixNano(),
	}
	return s.owner.ApplyContextTurnRefreshCurrentCASV1(ctx, contract.ContextTurnRefreshCommitV1{Apply: request, Settlement: settlement, AppliedUnixNano: completed.UnixNano()})
}

func (s *ContextTurnRefreshServiceV1) InspectContextTurnRefreshV1(ctx context.Context, request contract.InspectContextTurnRefreshRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	if err := checkRefreshContext(ctx); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if s == nil || request.Validate() != nil {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: inspect context turn refresh", contract.ErrInvalid)
	}
	return s.owner.InspectContextTurnRefreshV1(ctx, request)
}

func (s *ContextTurnRefreshServiceV1) readSnapshot(ctx context.Context, request contract.ContextTurnRefreshRequestV1, now int64) (refreshSnapshotV1, error) {
	tool, err := s.toolCurrent.InspectSettledActionContextSourceCurrentV1(ctx, request.ToolSource)
	if err != nil {
		return refreshSnapshotV1{}, fmt.Errorf("settled tool current: %w", err)
	}
	if tool.Request != request.ToolSource || tool.ValidateAt(now) != nil {
		return refreshSnapshotV1{}, fmt.Errorf("%w: settled tool current", contract.ErrConflict)
	}
	parent, err := s.parentCurrent.InspectContextParentFrameCurrentV1(ctx, request.ParentSource)
	if err != nil {
		return refreshSnapshotV1{}, fmt.Errorf("parent frame current: %w", err)
	}
	if parent.ValidateAt(now) != nil || parent.Source != request.ParentSource {
		return refreshSnapshotV1{}, fmt.Errorf("%w: parent frame current", contract.ErrConflict)
	}
	binding, err := s.owner.ResolveExactSourceBinding(ctx, request.ParentSource)
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	if binding.Validate() != nil || binding.Subject.FrameRef != parent.FrameRef || binding.Subject.ManifestRef != parent.ManifestRef || binding.Subject.GenerationRef != parent.GenerationRef {
		return refreshSnapshotV1{}, fmt.Errorf("%w: refresh parent binding", contract.ErrConflict)
	}
	subject := binding.Subject
	frame, err := s.owner.FrameByExactRef(ctx, subject.FrameRef, subject.ExecutionScopeDigest)
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	manifest, err := s.owner.ManifestByExactRef(ctx, subject.ManifestRef, subject.ExecutionScopeDigest)
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	generation, err := s.owner.GenerationByExactRef(ctx, subject.GenerationRef, subject.ExecutionScopeDigest)
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	pointer, err := s.owner.InspectCurrentGenerationPointer(ctx, contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: subject.ExecutionScopeDigest, RunID: subject.RunID, SessionRef: subject.SessionRef, Turn: subject.Turn})
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	if pointer != request.ExpectedCurrent || frame.Execution != request.ToolSource.Execution || frame.Execution.ScopeDigest != parent.ExecutionScopeDigest || frame.ManifestRef != parent.ManifestRef || frame.GenerationID != parent.GenerationRef.ID || frame.Generation != parent.GenerationOrdinal {
		return refreshSnapshotV1{}, fmt.Errorf("%w: refresh exact current binding", contract.ErrConflict)
	}
	if err := InspectFrame(s.content, manifest, frame); err != nil {
		return refreshSnapshotV1{}, err
	}
	recipeDigest, err := request.Recipe.DigestValue()
	if err != nil {
		return refreshSnapshotV1{}, err
	}
	if (contract.FactRef{ID: request.Recipe.ID, Revision: request.Recipe.Revision, Digest: recipeDigest}) != subject.RecipeRef || request.CacheIdentity.RecipeRef != subject.RecipeRef || request.CacheIdentity.RenderVersion != request.Recipe.RenderVersion || request.CacheIdentity.StablePrefix != frame.StablePrefix || !sameContentRefPtr(request.CacheIdentity.SemiStable, frame.SemiStable) || request.CacheIdentity.AuthorityDigest != frame.Execution.AuthorityDigest {
		return refreshSnapshotV1{}, fmt.Errorf("%w: refresh recipe or stable prefix drift", contract.ErrConflict)
	}
	stableDigest, err := stableSourceSetDigest(manifest)
	if err != nil || stableDigest != request.CacheIdentity.StableSourceSetDigest {
		return refreshSnapshotV1{}, fmt.Errorf("%w: stable source set drift", contract.ErrConflict)
	}
	return refreshSnapshotV1{parent, tool, binding, frame, manifest, generation, pointer}, nil
}

func (s *ContextTurnRefreshServiceV1) freeze(request contract.ContextTurnRefreshRequestV1, snapshot refreshSnapshotV1, now int64) (contract.ContextTurnRefreshPendingRecordV1, error) {
	rule, ok := request.Recipe.Rule(contract.FragmentToolResult)
	if !ok || rule.Region != contract.RegionDynamicTail || snapshot.Tool.TokenEstimate > rule.MaxTokens || snapshot.Manifest.DynamicTokens+snapshot.Tool.TokenEstimate > request.Recipe.Budget.DynamicTailMax || snapshot.Manifest.TotalTokens+snapshot.Tool.TokenEstimate > request.Recipe.Budget.TotalTokens {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: settled tool admission budget", contract.ErrConflict)
	}
	if _, err := readExact(s.content, snapshot.Tool.Content); err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	expires := minInt64(request.NotAfterUnixNano, snapshot.Parent.ExpiresUnixNano, snapshot.Tool.ExpiresUnixNano, request.CacheIdentity.ExpiresUnixNano, request.Recipe.ExpiresUnixNano, snapshot.Frame.ExpiresUnixNano, snapshot.Manifest.ExpiresUnixNano)
	if request.MemorySource != nil {
		expires = minInt64(expires, request.MemorySource.ExpiresUnixNano)
	}
	if request.KnowledgeSource != nil {
		expires = minInt64(expires, request.KnowledgeSource.ExpiresUnixNano)
	}
	if now >= expires {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: refresh freeze TTL", contract.ErrExpired)
	}
	childExecution := snapshot.Frame.Execution
	if childExecution.Turn == ^uint32(0) {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: refresh turn overflow", contract.ErrConflict)
	}
	childExecution.Turn++
	candidate := contract.ContextCandidate{
		ContractVersion: contract.Version, ID: "ctx-refresh-tool-" + request.RefreshAttemptID, Revision: 1, Kind: contract.FragmentToolResult,
		Owner: contract.OwnerRef{ComponentID: "tool/domain-result", BindingDigest: snapshot.Tool.Request.AssociationRef.Digest}, Execution: snapshot.Frame.Execution,
		SourceRef: snapshot.Tool.Request.ToolResultRef.ID, SourceRevision: snapshot.Tool.Request.ToolResultRef.Revision, Content: snapshot.Tool.Content,
		Trust: contract.TrustObservation, Sensitivity: snapshot.Tool.Sensitivity, Mode: contract.MaterializationReference, Required: true,
		TokenEstimate: snapshot.Tool.TokenEstimate, EstimatorDigest: snapshot.Tool.SourceDigest, CacheStability: 0,
		Evidence: contract.EvidenceRef{ID: snapshot.Tool.Request.InspectionRef.ID, Digest: snapshot.Tool.Request.InspectionRef.Digest}, IdempotencyKey: request.IdempotencyKey,
		CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires,
	}
	candidateDigest, err := candidate.DigestValue()
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	candidateRef := contract.FactRef{ID: candidate.ID, Revision: candidate.Revision, Digest: candidateDigest}
	manifest := snapshot.Manifest
	manifest.ID, manifest.Revision, manifest.Execution, manifest.GenerationID = request.ManifestID, 1, childExecution, request.NextGenerationID
	parentFrameRef := snapshot.Parent.FrameRef
	manifest.ParentFrame = &parentFrameRef
	manifest.Decisions = append(append([]contract.AdmissionDecision(nil), snapshot.Manifest.Decisions...), contract.AdmissionDecision{CandidateRef: candidateRef, Disposition: contract.AdmissionAdmitted, Reason: "settled_tool_current", Region: contract.RegionDynamicTail, Tokens: snapshot.Tool.TokenEstimate})
	manifest.Fragments = append(append([]contract.ContextFragment(nil), snapshot.Manifest.Fragments...), contract.ContextFragment{CandidateRef: candidateRef, Kind: contract.FragmentToolResult, Region: contract.RegionDynamicTail, Position: uint32(len(snapshot.Manifest.Fragments) + 1), Content: snapshot.Tool.Content, Tokens: snapshot.Tool.TokenEstimate})
	manifest.DynamicTokens += snapshot.Tool.TokenEstimate
	manifest.TotalTokens += snapshot.Tool.TokenEstimate
	for _, source := range []*contract.ContextOwnerSourceContributionV1{request.MemorySource, request.KnowledgeSource} {
		if source == nil {
			continue
		}
		kind := contract.FragmentMemoryRecall
		ownerID := "memory/domain-result"
		reason := "memory_owner_current"
		if source.Owner == contract.ContextOwnerSourceKnowledgeV1 {
			kind = contract.FragmentKnowledgeReference
			ownerID = "knowledge/domain-result"
			reason = "knowledge_owner_current"
		}
		rule, ok := request.Recipe.Rule(kind)
		if !ok || rule.Region != contract.RegionDynamicTail {
			return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: %s rule", contract.ErrUnsupported, source.Owner)
		}
		for _, item := range source.Items {
			if item.TokenEstimate > rule.MaxTokens || manifest.DynamicTokens+item.TokenEstimate > request.Recipe.Budget.DynamicTailMax || manifest.TotalTokens+item.TokenEstimate > request.Recipe.Budget.TotalTokens {
				return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: %s admission budget", contract.ErrConflict, source.Owner)
			}
			if _, readErr := readExact(s.content, item.Content); readErr != nil {
				return contract.ContextTurnRefreshPendingRecordV1{}, readErr
			}
			ownerCandidate := contract.ContextCandidate{
				ContractVersion: contract.Version, ID: fmt.Sprintf("ctx-refresh-%s-%s-%d", source.Owner, request.RefreshAttemptID, item.Rank), Revision: 1, Kind: kind,
				Owner: contract.OwnerRef{ComponentID: ownerID, BindingDigest: source.StableAssociationDigest}, Execution: snapshot.Frame.Execution,
				SourceRef: item.RecordRef.Ref.ID, SourceRevision: item.RecordRef.Ref.Revision, Content: item.Content,
				Trust: contract.TrustObservation, Sensitivity: item.Sensitivity, Mode: contract.MaterializationReference, Required: false,
				TokenEstimate: item.TokenEstimate, EstimatorDigest: item.OwnerItemDigest, CacheStability: 0,
				Evidence: contract.EvidenceRef{ID: source.EnvelopeRef.Ref.ID, Digest: source.EnvelopeRef.Ref.Digest}, IdempotencyKey: request.IdempotencyKey + ":" + string(source.Owner) + ":" + fmt.Sprint(item.Rank),
				CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: minInt64(expires, item.ExpiresUnixNano),
			}
			ownerDigest, digestErr := ownerCandidate.DigestValue()
			if digestErr != nil {
				return contract.ContextTurnRefreshPendingRecordV1{}, digestErr
			}
			ownerRef := contract.FactRef{ID: ownerCandidate.ID, Revision: ownerCandidate.Revision, Digest: ownerDigest}
			manifest.Decisions = append(manifest.Decisions, contract.AdmissionDecision{CandidateRef: ownerRef, Disposition: contract.AdmissionAdmitted, Reason: reason, Region: contract.RegionDynamicTail, Tokens: item.TokenEstimate})
			manifest.Fragments = append(manifest.Fragments, contract.ContextFragment{CandidateRef: ownerRef, Kind: kind, Region: contract.RegionDynamicTail, Position: uint32(len(manifest.Fragments) + 1), Content: item.Content, Tokens: item.TokenEstimate})
			manifest.DynamicTokens += item.TokenEstimate
			manifest.TotalTokens += item.TokenEstimate
		}
	}
	manifest.CreatedUnixNano, manifest.ExpiresUnixNano = request.CheckedUnixNano, expires
	refs := make([]contract.FactRef, 0, len(manifest.Decisions))
	for _, d := range manifest.Decisions {
		refs = append(refs, d.CandidateRef)
	}
	manifest.SourceSetDigest, err = contract.DigestJSON(refs)
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	regions := map[contract.FrameRegion][]renderedFragment{contract.RegionStablePrefix: {}, contract.RegionSemiStable: {}, contract.RegionDynamicTail: {}}
	for _, fragment := range manifest.Fragments {
		content, readErr := readExact(s.content, fragment.Content)
		if readErr != nil {
			return contract.ContextTurnRefreshPendingRecordV1{}, readErr
		}
		regions[fragment.Region] = append(regions[fragment.Region], renderedFragment{Position: fragment.Position, Kind: fragment.Kind, CandidateDigest: fragment.CandidateRef.Digest, Content: content})
	}
	stableBytes, semiBytes, dynamicBytes, renderedBytes, err := renderRegions(regions)
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	parentStable, err := readExact(s.content, snapshot.Frame.StablePrefix)
	if err != nil || !bytes.Equal(parentStable, stableBytes) {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: stable prefix changed", contract.ErrConflict)
	}
	if snapshot.Frame.SemiStable != nil {
		parentSemi, readErr := readExact(s.content, *snapshot.Frame.SemiStable)
		if readErr != nil || !bytes.Equal(parentSemi, semiBytes) {
			return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: semi-stable changed", contract.ErrConflict)
		}
	}
	dynamicRef, err := s.content.Put(dynamicBytes)
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	renderedRef, err := s.content.Put(renderedBytes)
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	manifestDigest, err := manifest.DigestValue()
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	manifestRef := contract.FactRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifestDigest}
	frame := contract.ContextFrame{ContractVersion: contract.Version, ID: request.FrameID, Revision: 1, Execution: childExecution, ManifestRef: manifestRef, ParentFrame: &parentFrameRef, GenerationID: request.NextGenerationID, Generation: snapshot.Generation.Ordinal + 1, StablePrefix: snapshot.Frame.StablePrefix, SemiStable: cloneContentRefPtr(snapshot.Frame.SemiStable), DynamicTail: dynamicRef, Rendered: renderedRef, SourceSetDigest: manifest.SourceSetDigest, CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires}
	if err := InspectFrame(s.content, manifest, frame); err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	frameDigest, err := frame.DigestValue()
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	frameRef := contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}
	parentGenerationRef := snapshot.Parent.GenerationRef
	generation := contract.ContextGeneration{ContractVersion: contract.Version, ID: request.NextGenerationID, Revision: 1, Ordinal: snapshot.Generation.Ordinal + 1, Parent: &parentGenerationRef, RootFrame: frameRef, RetainedAnchors: append([]contract.FactRef(nil), snapshot.Generation.RetainedAnchors...), OpenEffects: append([]contract.FactRef(nil), snapshot.Generation.OpenEffects...), CreatedUnixNano: request.CheckedUnixNano}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	generationRef := contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest}
	bindingDigest, err := contract.DigestJSON(struct{ ParentFrame, ParentGeneration, Frame, Generation contract.FactRef }{snapshot.Parent.FrameRef, snapshot.Parent.GenerationRef, frameRef, generationRef})
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	subject := contract.ContextParentFrameApplicabilitySubjectV1{ContractVersion: contract.Version, FrameRef: frameRef, ManifestRef: manifestRef, GenerationRef: generationRef, GenerationOrdinal: generation.Ordinal, ExecutionScopeDigest: childExecution.ScopeDigest, RunID: childExecution.RunID, SessionRef: snapshot.Binding.Subject.SessionRef, Turn: childExecution.Turn, ParentFrameRef: &parentFrameRef, ParentGenerationRef: &parentGenerationRef, ParentFrameGenerationBindingDigest: bindingDigest, RecipeRef: manifest.RecipeRef, AuthorityDigest: childExecution.AuthorityDigest}
	source, err := contract.SealContextParentFrameApplicabilitySourceCoordinateV1(subject)
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	binding := contract.ContextParentFrameSourceBindingV1{Source: source, Subject: subject, BindingExpiresUnixNano: expires, RecipeExpiresUnixNano: expires, AuthorityExpiresUnixNano: expires}
	pointer, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{ID: request.ExpectedCurrent.ID, Revision: request.ExpectedCurrent.Revision + 1, ExecutionScopeDigest: childExecution.ScopeDigest, RunID: childExecution.RunID, SessionRef: subject.SessionRef, Turn: childExecution.Turn, GenerationRef: generationRef, GenerationOrdinal: generation.Ordinal, ParentFrameGenerationBindingDigest: bindingDigest, ExpiresUnixNano: expires})
	if err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	pending := contract.ContextTurnRefreshPendingDomainResultV1{ContractVersion: contract.Version, ID: "ctx-refresh-domain-result-" + request.RefreshAttemptID, Revision: 1, RequestDigest: request.Digest, ParentProjectionDigest: snapshot.Parent.Digest, ToolSourceDigest: snapshot.Tool.SourceDigest, ManifestRef: manifestRef, FrameRef: frameRef, GenerationRef: generationRef, ChildSource: source, ExpectedCurrent: request.ExpectedCurrent, NextCurrent: pointer, CacheIdentityDigest: request.CacheIdentity.Digest, CreatedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires, Status: contract.ContextTurnRefreshPendingV1}
	return contract.ContextTurnRefreshPendingRecordV1{Request: request, ParentProjection: snapshot.Parent, ToolProjection: snapshot.Tool, Manifest: manifest, Frame: frame, Generation: generation, Binding: binding, Pointer: pointer, Pending: pending}, nil
}

func stableSourceSetDigest(manifest contract.ContextManifest) (contract.Digest, error) {
	refs := make([]contract.FactRef, 0)
	for _, f := range manifest.Fragments {
		if f.Region == contract.RegionStablePrefix {
			refs = append(refs, f.CandidateRef)
		}
	}
	return contract.DigestJSON(refs)
}
func parentProjectionIdentityDigest(p contract.ContextParentFrameCurrentProjectionV1) (contract.Digest, error) {
	return contract.DigestJSON(struct {
		Source                      contract.ContextParentFrameApplicabilitySourceCoordinateV1
		Frame, Manifest, Generation contract.FactRef
		Ordinal                     uint64
		Scope                       contract.Digest
		Expires                     int64
	}{p.Source, p.FrameRef, p.ManifestRef, p.GenerationRef, p.GenerationOrdinal, p.ExecutionScopeDigest, p.ExpiresUnixNano})
}
func sameParentCurrent(a, b contract.ContextParentFrameCurrentProjectionV1) bool {
	ad, _ := parentProjectionIdentityDigest(a)
	bd, _ := parentProjectionIdentityDigest(b)
	return ad == bd
}
func sameContentRefPtr(a, b *contract.ContentRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func cloneContentRefPtr(v *contract.ContentRef) *contract.ContentRef {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
func minInt64(values ...int64) int64 {
	result := values[0]
	for _, v := range values[1:] {
		if v < result {
			result = v
		}
	}
	return result
}
func mustPendingRef(p contract.ContextTurnRefreshPendingDomainResultV1) contract.FactRef {
	d, _ := p.DigestValue()
	return contract.FactRef{ID: p.ID, Revision: p.Revision, Digest: d}
}
func checkRefreshContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}
func (s *ContextTurnRefreshServiceV1) nowAfter(minimum int64) (int64, error) {
	now := s.clock()
	if now.IsZero() {
		return 0, fmt.Errorf("%w: refresh clock", contract.ErrInvalid)
	}
	if now.UnixNano() < minimum {
		return 0, fmt.Errorf("%w: refresh clock rollback", contract.ErrConflict)
	}
	return now.UnixNano(), nil
}
