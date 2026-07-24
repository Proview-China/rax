package applicationadapter

import (
	"context"
	"fmt"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const applicationAttemptKindV1 = runtimeports.NamespacedNameV2("application/context-attempt")

type ContextTurnRefreshAdapterV1 struct {
	service   *kernel.ContextTurnRefreshServiceV1
	owner     contextports.ContextTurnRefreshOwnerBackendV1
	proofs    contextports.ContextTurnTransitionProofStoreV1
	content   kernel.ReferenceStore
	memory    applicationports.ContextOwnerSourceReaderV1
	knowledge applicationports.ContextOwnerSourceReaderV1
	clock     func() time.Time
}

var _ applicationports.ContextTurnRefreshPortV1 = (*ContextTurnRefreshAdapterV1)(nil)

func NewContextTurnRefreshAdapterV1(service *kernel.ContextTurnRefreshServiceV1, owner contextports.ContextTurnRefreshOwnerBackendV1, proofs contextports.ContextTurnTransitionProofStoreV1, content kernel.ReferenceStore, memory, knowledge applicationports.ContextOwnerSourceReaderV1, clock func() time.Time) (*ContextTurnRefreshAdapterV1, error) {
	if service == nil || owner == nil || proofs == nil || content == nil || (memory == nil && knowledge == nil) {
		return nil, fmt.Errorf("%w: Context Application adapter dependencies", contract.ErrInvalid)
	}
	if clock == nil {
		clock = time.Now
	}
	return &ContextTurnRefreshAdapterV1{service: service, owner: owner, proofs: proofs, content: content, memory: memory, knowledge: knowledge, clock: clock}, nil
}

func (a *ContextTurnRefreshAdapterV1) PrepareContextTurnRefreshV1(ctx context.Context, request applicationcontract.ContextTurnRefreshPrepareRequestV1) (applicationcontract.ContextTurnRefreshPreparedV1, error) {
	if a == nil || ctx == nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	now := a.clock()
	if now.IsZero() || request.ValidateCurrent(now) != nil || request.SourceTurn.Ordinal == 0 {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, contract.ErrInvalid
	}
	var contextRequest contract.ContextTurnRefreshRequestV1
	if err := core.DecodeStrictJSON(request.OpaqueContextRequest, &contextRequest); err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, contract.ErrInvalid
	}
	sealed, err := contract.SealContextTurnRefreshRequestV1(contextRequest)
	if err != nil || !reflect.DeepEqual(sealed, contextRequest) || !prepareMatchesContext(request, contextRequest) {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, contract.ErrConflict
	}
	contextRequest.MemorySource, err = a.materialize(ctx, request.MemoryRequest, request.Memory, a.memory, contract.ContextOwnerSourceMemoryV1, now)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	contextRequest.KnowledgeSource, err = a.materialize(ctx, request.KnowledgeRequest, request.Knowledge, a.knowledge, contract.ContextOwnerSourceKnowledgeV1, now)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	contextRequest.Cardinality.Memory = boolCount(contextRequest.MemorySource != nil)
	contextRequest.Cardinality.Knowledge = boolCount(contextRequest.KnowledgeSource != nil)
	contextRequest, err = contract.SealContextTurnRefreshRequestV1(contextRequest)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	stableSet, err := applicationcontract.StableContextSourceSetDigestV1(request.Memory, request.Knowledge)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	s1Set, err := applicationcontract.ContextSourceAssociationSetDigestV1(request.Memory, request.Knowledge)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	applicationAttempt := contract.FactRef{ID: request.ID, Revision: 1, Digest: contract.Digest(request.Digest)}
	refreshAttempt := contract.FactRef{ID: contextRequest.RefreshAttemptID, Revision: 1, Digest: contextRequest.Digest}
	transition, err := contract.SealContextTurnTransitionRequestV1(contract.ContextTurnTransitionRequestV1{
		ApplicationAttemptRef: applicationAttempt, RefreshAttemptRef: refreshAttempt,
		SourceSessionRef:  typedApplicationRef(request.SessionApplicability.Kind, request.SourceSession.ID, request.SourceSession.Revision, request.SourceSession.Digest),
		SourceTurnRef:     typedApplicationRef(request.TurnApplicability.Kind, request.SourceTurn.ID, request.SourceTurn.Revision, request.SourceTurn.Digest),
		SourceTurnOrdinal: request.SourceTurn.Ordinal, ExpectedTargetOrdinal: request.ExpectedTargetTurn, ExpectedCurrent: contextRequest.ExpectedCurrent,
		StableSourceSetDigest: contract.Digest(stableSet), S1AssociationSetDigest: contract.Digest(s1Set),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: minInt64(request.RequestedNotAfterNano, contextRequest.NotAfterUnixNano),
	}, now.UnixNano())
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	if err = a.proofs.ReserveContextTurnTransitionRequestV1(ctx, transition); err != nil {
		return a.recoverPrepared(context.WithoutCancel(ctx), applicationAttempt, now)
	}
	if _, err = a.service.RefreshContextTurnV1(ctx, contextRequest); err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	return a.ensurePrepared(ctx, transition, now)
}

func (a *ContextTurnRefreshAdapterV1) ApplyContextTurnRefreshV1(ctx context.Context, request applicationcontract.ContextTurnRefreshApplyRequestV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	if a == nil || ctx == nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	now := a.clock()
	if now.IsZero() || request.ValidateCurrent(now) != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, contract.ErrInvalid
	}
	applicationAttempt := contextFactRef(request.Prepared.AttemptRef)
	transition, err := a.proofs.InspectContextTurnTransitionRequestByApplicationAttemptV1(ctx, applicationAttempt)
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	proofCurrent, err := a.proofs.InspectContextTurnTransitionProofV1(ctx, contextFactRef(request.Prepared.TransitionProofRef))
	if err != nil || proofCurrent.ValidateAt(now.UnixNano()) != nil || proofCurrent.Proof.ApplicationAttemptRef != applicationAttempt || proofCurrent.Proof.StableSourceSetDigest != contract.Digest(request.Prepared.StableSourceSetDigest) || proofCurrent.S1AssociationSetDigest != contract.Digest(request.Prepared.S1AssociationSetDigest) {
		return applicationcontract.ContextTurnRefreshResultV1{}, contract.ErrConflict
	}
	record, err := a.owner.LoadContextTurnRefreshPendingRecordV1(ctx, transition.RefreshAttemptRef)
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	if err = a.verifyS2(ctx, request.MemoryRequest, request.Memory, a.memory, record.Request.MemorySource, now); err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	if err = a.verifyS2(ctx, request.KnowledgeRequest, request.Knowledge, a.knowledge, record.Request.KnowledgeSource, now); err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	proofRef, err := proofCurrent.Proof.Ref()
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	apply, err := contract.SealApplyContextTurnRefreshRequestV1(contract.ApplyContextTurnRefreshRequestV1{
		AttemptRef: transition.RefreshAttemptRef, PendingDomainResultRef: proofCurrent.Proof.PendingDomainResultRef,
		ExpectedCurrent: transition.ExpectedCurrent, TransitionProofRef: &proofRef,
		StableSourceSetDigest: proofCurrent.Proof.StableSourceSetDigest, S2AssociationSetDigest: contract.Digest(request.S2AssociationSetDigest),
		CheckedUnixNano:  now.UnixNano(),
		NotAfterUnixNano: minInt64(request.RequestedNotAfterNano, proofCurrent.ExpiresUnixNano, record.Pending.ExpiresUnixNano),
	}, now.UnixNano())
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	if _, err = a.service.ApplyContextTurnRefreshV1(ctx, apply); err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	return a.inspectResult(ctx, transition, proofCurrent)
}

func (a *ContextTurnRefreshAdapterV1) InspectContextTurnRefreshV1(ctx context.Context, request applicationcontract.ContextTurnRefreshInspectRequestV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	if a == nil || ctx == nil || request.Validate() != nil || request.AttemptRef.Kind != applicationAttemptKindV1 {
		return applicationcontract.ContextTurnRefreshResultV1{}, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	applicationAttempt := contextFactRef(request.AttemptRef)
	transition, err := a.proofs.InspectContextTurnTransitionRequestByApplicationAttemptV1(ctx, applicationAttempt)
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	now := a.clock()
	prepared, err := a.ensurePrepared(ctx, transition, now)
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	proofCurrent, err := a.proofs.InspectContextTurnTransitionProofV1(ctx, contextFactRef(prepared.TransitionProofRef))
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	return a.inspectResult(ctx, transition, proofCurrent)
}

func (a *ContextTurnRefreshAdapterV1) materialize(ctx context.Context, sourceRequest *applicationcontract.ContextOwnerSourceRequestV1, envelope *applicationcontract.ContextOwnerSourceEnvelopeV1, reader applicationports.ContextOwnerSourceReaderV1, owner contract.ContextOwnerSourceKindV1, now time.Time) (*contract.ContextOwnerSourceContributionV1, error) {
	if sourceRequest == nil || envelope == nil {
		if sourceRequest != nil || envelope != nil {
			return nil, contract.ErrConflict
		}
		return nil, nil
	}
	if reader == nil || envelope.ValidateCurrent(now) != nil || sourceRequest.ValidateCurrent(now) != nil {
		return nil, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var contentBytes uint64
	for _, sourceItem := range envelope.Items {
		length := uint64(sourceItem.ContentRef.Length)
		if length > contract.MaxContextTurnRefreshSourceBytesV1-contentBytes {
			return nil, contract.ErrLimitExceeded
		}
		contentBytes += length
	}
	items := make([]contract.ContextOwnerSourceItemV1, len(envelope.Items))
	for index, sourceItem := range envelope.Items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contentRequest, err := applicationcontract.SealContextOwnerContentRequestV1(applicationcontract.ContextOwnerContentRequestV1{SourceRequest: *sourceRequest, Envelope: *envelope, Rank: uint32(index), MaxBodyBytes: sourceItem.ContentRef.Length, RequestedNotAfterNano: sourceRequest.RequestedNotAfterNano})
		if err != nil {
			return nil, err
		}
		observation, body, err := reader.ReadContextOwnerContentExactV1(ctx, contentRequest)
		if err != nil {
			return nil, err
		}
		if observation.ValidateCurrent(a.clock()) != nil || core.DigestBytes(body) != observation.ObservedDigest || observation.ProjectionItemDigest != sourceItem.ItemDigest {
			return nil, contract.ErrConflict
		}
		content, err := a.content.Put(body)
		if err != nil {
			return nil, err
		}
		if content.Digest != contract.Digest(sourceItem.ContentRef.Digest) || content.Length != uint64(sourceItem.ContentRef.Length) {
			return nil, contract.ErrConflict
		}
		chain := make([]contract.ContextTypedFactRefV1, len(sourceItem.StableOwnerChain))
		for refIndex, ref := range sourceItem.StableOwnerChain {
			chain[refIndex] = typedExactRef(ref)
		}
		item, err := contract.SealContextOwnerSourceItemV1(contract.ContextOwnerSourceItemV1{
			Rank: sourceItem.Rank, OwnerItemDigest: contract.Digest(sourceItem.ItemDigest), RecordRef: typedExactRef(sourceItem.RecordRef), StableOwnerChain: chain,
			Content: content, TokenEstimate: sourceItem.TokenEstimate, Sensitivity: contract.Sensitivity(sourceItem.Sensitivity), CitationDigest: contract.Digest(sourceItem.CitationDigest), License: sourceItem.License,
			ExpiresUnixNano: minInt64(sourceItem.ExpiresUnixNano, observation.ExpiresUnixNano, envelope.ExpiresUnixNano),
		})
		if err != nil {
			return nil, err
		}
		items[index] = item
	}
	contribution, err := contract.SealContextOwnerSourceContributionV1(contract.ContextOwnerSourceContributionV1{
		Owner: owner, EnvelopeRef: typedEnvelopeRef(*envelope), OwnerProjectionRef: typedExactRef(envelope.CurrentProjectionRef),
		StableClosureDigest: contract.Digest(envelope.StableClosureDigest), StableAssociationDigest: contract.Digest(envelope.StableAssociationDigest),
		SourceSessionRef: typedApplicationRef(envelope.SessionApplicability.Kind, envelope.SourceSession.ID, envelope.SourceSession.Revision, envelope.SourceSession.Digest),
		SourceTurnRef:    typedApplicationRef(envelope.TurnApplicability.Kind, envelope.SourceTurn.ID, envelope.SourceTurn.Revision, envelope.SourceTurn.Digest), SourceTurnOrdinal: envelope.SourceTurn.Ordinal,
		Items: items, CheckedUnixNano: envelope.CheckedUnixNano, ExpiresUnixNano: envelope.ExpiresUnixNano,
	})
	if err != nil {
		return nil, err
	}
	return &contribution, nil
}

func (a *ContextTurnRefreshAdapterV1) verifyS2(ctx context.Context, sourceRequest *applicationcontract.ContextOwnerSourceRequestV1, envelope *applicationcontract.ContextOwnerSourceEnvelopeV1, reader applicationports.ContextOwnerSourceReaderV1, frozen *contract.ContextOwnerSourceContributionV1, now time.Time) error {
	if sourceRequest == nil || envelope == nil || frozen == nil {
		if sourceRequest == nil && envelope == nil && frozen == nil {
			return nil
		}
		return contract.ErrConflict
	}
	if envelope.Phase != applicationcontract.ContextSourceCheckS2V1 || sourceRequest.Phase != applicationcontract.ContextSourceCheckS2V1 || contract.Digest(envelope.StableAssociationDigest) != frozen.StableAssociationDigest || len(envelope.Items) != len(frozen.Items) {
		return contract.ErrConflict
	}
	for index, sourceItem := range envelope.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		contentRequest, err := applicationcontract.SealContextOwnerContentRequestV1(applicationcontract.ContextOwnerContentRequestV1{SourceRequest: *sourceRequest, Envelope: *envelope, Rank: uint32(index), MaxBodyBytes: sourceItem.ContentRef.Length, RequestedNotAfterNano: sourceRequest.RequestedNotAfterNano})
		if err != nil {
			return err
		}
		observation, body, err := reader.ReadContextOwnerContentExactV1(ctx, contentRequest)
		if err != nil {
			return err
		}
		if observation.ValidateCurrent(a.clock()) != nil || core.DigestBytes(body) != observation.ObservedDigest || contract.Digest(observation.ObservedDigest) != frozen.Items[index].Content.Digest || contract.Digest(sourceItem.ItemDigest) != frozen.Items[index].OwnerItemDigest {
			return contract.ErrConflict
		}
	}
	return nil
}

func (a *ContextTurnRefreshAdapterV1) ensurePrepared(ctx context.Context, transition contract.ContextTurnTransitionRequestV1, now time.Time) (applicationcontract.ContextTurnRefreshPreparedV1, error) {
	record, err := a.owner.LoadContextTurnRefreshPendingRecordV1(ctx, transition.RefreshAttemptRef)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	pendingDigest, err := record.Pending.DigestValue()
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	requestRef, err := transition.Ref()
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	proof, err := contract.SealContextTurnTransitionProofV1(contract.ContextTurnTransitionProofV1{
		TransitionRequestRef: requestRef, ApplicationAttemptRef: transition.ApplicationAttemptRef, RefreshAttemptRef: transition.RefreshAttemptRef,
		SourceSessionRef: transition.SourceSessionRef, SourceTurnRef: transition.SourceTurnRef, SourceTurnOrdinal: transition.SourceTurnOrdinal, TargetTurnOrdinal: transition.ExpectedTargetOrdinal,
		ExpectedCurrent: transition.ExpectedCurrent, ChildExecution: record.Frame.Execution,
		PendingDomainResultRef: contract.FactRef{ID: record.Pending.ID, Revision: record.Pending.Revision, Digest: pendingDigest},
		ManifestRef:            record.Pending.ManifestRef, FrameRef: record.Pending.FrameRef, GenerationRef: record.Pending.GenerationRef,
		StableSourceSetDigest: transition.StableSourceSetDigest,
	})
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	current, err := contract.SealContextTurnTransitionProofCurrentV1(contract.ContextTurnTransitionProofCurrentV1{Proof: proof, S1AssociationSetDigest: transition.S1AssociationSetDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: minInt64(transition.ExpiresUnixNano, record.Pending.ExpiresUnixNano)}, now.UnixNano())
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	current, err = a.proofs.EnsureContextTurnTransitionProofV1(ctx, current)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	proofRef, _ := current.Proof.Ref()
	prepared, err := applicationcontract.SealContextTurnRefreshPreparedV1(applicationcontract.ContextTurnRefreshPreparedV1{
		AttemptRef: appExactRef(applicationAttemptKindV1, transition.ApplicationAttemptRef), PendingDomainResultRef: appExactRef("context/pending-result", current.Proof.PendingDomainResultRef),
		TransitionProofRef: appExactRef("context/transition-proof", proofRef), ManifestRef: appExactRef("context/manifest", current.Proof.ManifestRef), FrameRef: appExactRef("context/frame", current.Proof.FrameRef), GenerationRef: appExactRef("context/generation", current.Proof.GenerationRef),
		StableSourceSetDigest: core.Digest(current.Proof.StableSourceSetDigest), S1AssociationSetDigest: core.Digest(current.S1AssociationSetDigest), CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano,
	})
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	return prepared, nil
}

func (a *ContextTurnRefreshAdapterV1) recoverPrepared(ctx context.Context, applicationAttempt contract.FactRef, now time.Time) (applicationcontract.ContextTurnRefreshPreparedV1, error) {
	transition, err := a.proofs.InspectContextTurnTransitionRequestByApplicationAttemptV1(ctx, applicationAttempt)
	if err != nil {
		return applicationcontract.ContextTurnRefreshPreparedV1{}, err
	}
	return a.ensurePrepared(ctx, transition, now)
}

func (a *ContextTurnRefreshAdapterV1) inspectResult(ctx context.Context, transition contract.ContextTurnTransitionRequestV1, proofCurrent contract.ContextTurnTransitionProofCurrentV1) (applicationcontract.ContextTurnRefreshResultV1, error) {
	ownerResult, err := a.service.InspectContextTurnRefreshV1(ctx, contract.InspectContextTurnRefreshRequestV1{AttemptRef: transition.RefreshAttemptRef})
	if err != nil {
		return applicationcontract.ContextTurnRefreshResultV1{}, err
	}
	proofRef, _ := proofCurrent.Proof.Ref()
	result := applicationcontract.ContextTurnRefreshResultV1{
		AttemptRef: appExactRef(applicationAttemptKindV1, transition.ApplicationAttemptRef), PendingDomainResultRef: appExactRef("context/pending-result", proofCurrent.Proof.PendingDomainResultRef), TransitionProofRef: appExactRef("context/transition-proof", proofRef),
		ManifestRef: appExactRef("context/manifest", proofCurrent.Proof.ManifestRef), FrameRef: appExactRef("context/frame", proofCurrent.Proof.FrameRef), GenerationRef: appExactRef("context/generation", proofCurrent.Proof.GenerationRef),
		StableSourceSetDigest: core.Digest(proofCurrent.Proof.StableSourceSetDigest), S1AssociationSetDigest: core.Digest(proofCurrent.S1AssociationSetDigest), CheckedUnixNano: proofCurrent.CheckedUnixNano, ExpiresUnixNano: proofCurrent.ExpiresUnixNano,
		State: applicationcontract.ContextTurnRefreshPreparedStateV1,
	}
	if ownerResult.Status == contract.ContextTurnRefreshAppliedV1 {
		result.State = applicationcontract.ContextTurnRefreshAppliedStateV1
		if ownerResult.ApplySettlementRef == nil || ownerResult.Current == nil || ownerResult.TransitionProofRef == nil || *ownerResult.TransitionProofRef != proofRef || ownerResult.StableSourceSetDigest != proofCurrent.Proof.StableSourceSetDigest || ownerResult.S2AssociationSetDigest.Validate() != nil {
			return applicationcontract.ContextTurnRefreshResultV1{}, contract.ErrConflict
		}
		settlement := appExactRef("context/apply-settlement", *ownerResult.ApplySettlementRef)
		pointer := appExactRef("context/current-pointer", contract.FactRef{ID: ownerResult.Current.ID, Revision: ownerResult.Current.Revision, Digest: ownerResult.Current.Digest})
		result.ApplySettlementRef, result.CurrentPointerRef = &settlement, &pointer
		result.S2AssociationSetDigest = core.Digest(ownerResult.S2AssociationSetDigest)
	}
	return applicationcontract.SealContextTurnRefreshResultV1(result)
}

func prepareMatchesContext(app applicationcontract.ContextTurnRefreshPrepareRequestV1, ctx contract.ContextTurnRefreshRequestV1) bool {
	return ctx.ExpectedCurrent.ExecutionScopeDigest == contract.Digest(app.ExecutionScopeDigest) && ctx.ExpectedCurrent.RunID == string(app.RunID) && ctx.ExpectedCurrent.SessionRef.ID == app.SourceSession.ID && ctx.ExpectedCurrent.SessionRef.Revision == uint64(app.SourceSession.Revision) && ctx.ExpectedCurrent.SessionRef.Digest == contract.Digest(app.SourceSession.Digest) && ctx.ExpectedCurrent.Turn == app.SourceTurn.Ordinal && ctx.ToolSource.Execution.Turn == app.SourceTurn.Ordinal && ctx.NotAfterUnixNano <= app.RequestedNotAfterNano
}

func typedExactRef(ref applicationcontract.ContextRefreshExactRefV1) contract.ContextTypedFactRefV1 {
	return contract.ContextTypedFactRefV1{Kind: string(ref.Kind), Ref: contextFactRef(ref)}
}

func typedEnvelopeRef(envelope applicationcontract.ContextOwnerSourceEnvelopeV1) contract.ContextTypedFactRefV1 {
	kind := runtimeports.NamespacedNameV2("memory/context-envelope")
	if envelope.Owner == applicationcontract.ContextOwnerKnowledgeV1 {
		kind = "knowledge/context-envelope"
	}
	return typedApplicationRef(kind, envelope.ID, envelope.Revision, envelope.Digest)
}

func typedApplicationRef(kind runtimeports.NamespacedNameV2, id string, revision core.Revision, digest core.Digest) contract.ContextTypedFactRefV1 {
	return contract.ContextTypedFactRefV1{Kind: string(kind), Ref: contract.FactRef{ID: id, Revision: uint64(revision), Digest: contract.Digest(digest)}}
}

func contextFactRef(ref applicationcontract.ContextRefreshExactRefV1) contract.FactRef {
	return contract.FactRef{ID: ref.ID, Revision: uint64(ref.Revision), Digest: contract.Digest(ref.Digest)}
}

func appExactRef(kind runtimeports.NamespacedNameV2, ref contract.FactRef) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{Kind: kind, ID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(ref.Digest)}
}

func boolCount(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}

func minInt64(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
