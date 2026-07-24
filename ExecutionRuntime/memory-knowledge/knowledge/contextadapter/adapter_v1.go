package contextadapter

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	ownercontract "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/knowledge/contextsource"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const knowledgeEnvelopeKindV1 = runtimeports.NamespacedNameV2("knowledge/context-envelope")

type AdapterV1 struct {
	owner contextsource.KnowledgeContextSourceCurrentReaderV2
	clock func() time.Time
}

var _ applicationports.ContextOwnerSourceReaderV1 = (*AdapterV1)(nil)

func NewAdapterV1(owner contextsource.KnowledgeContextSourceCurrentReaderV2, clock func() time.Time) (*AdapterV1, error) {
	if owner == nil {
		return nil, ownercontract.ErrInvalidArgument
	}
	if clock == nil {
		clock = time.Now
	}
	return &AdapterV1{owner: owner, clock: clock}, nil
}

func (a *AdapterV1) InspectContextOwnerSourceCurrentV1(ctx context.Context, request applicationcontract.ContextOwnerSourceRequestV1) (applicationcontract.ContextOwnerSourceEnvelopeV1, error) {
	envelope, _, _, err := a.inspect(ctx, request)
	return envelope, err
}

func (a *AdapterV1) inspect(ctx context.Context, request applicationcontract.ContextOwnerSourceRequestV1) (applicationcontract.ContextOwnerSourceEnvelopeV1, contextsource.CurrentRequestV2, contextsource.CurrentProjectionV2, error) {
	if a == nil || a.owner == nil || ctx == nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrInvalidArgument
	}
	now := a.clock()
	if now.IsZero() || request.Owner != applicationcontract.ContextOwnerKnowledgeV1 || request.ValidateCurrent(now) != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrInvalidArgument
	}
	var currentRequest contextsource.CurrentRequestV2
	if err := core.DecodeStrictJSON(request.OwnerRequest, &currentRequest); err != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrInvalidArgument
	}
	sealed, err := knowledgeRequestForPhase(currentRequest, request)
	if err != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, fmt.Errorf("knowledge Context adapter owner request: %w", err)
	}
	if !knowledgeRequestMatchesApplication(sealed, request) {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, fmt.Errorf("%w: knowledge Context adapter Session/Turn mapping", ownercontract.ErrEvidenceConflict)
	}
	currentRequest = sealed
	inspection, err := a.owner.InspectAttempt(ctx, currentRequest.Coordinate)
	if err != nil || inspection.Validate() != nil || inspection.Status != contextsource.AttemptPersistedAndSettled {
		if err != nil {
			return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, err
		}
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrInspectionIncomplete
	}
	projection, err := a.owner.InspectForTurn(ctx, currentRequest)
	if err != nil || projection.Validate() != nil || !reflect.DeepEqual(projection.Coordinate, currentRequest.Coordinate) {
		if err != nil {
			return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, err
		}
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrEvidenceConflict
	}
	if request.Phase == applicationcontract.ContextSourceCheckS2V1 && core.Digest(projection.StableClosureDigest) != request.ExpectedOwnerClosure {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrEvidenceConflict
	}
	items := make([]applicationcontract.ContextOwnerSourceItemV1, len(projection.Items))
	expires := minTime(inspection.ExpiresAt, projection.ExpiresAt, time.Unix(0, request.RequestedNotAfterNano))
	for index, item := range projection.Items {
		converted, convertErr := knowledgeItem(item, projection.SensitivityMax)
		if convertErr != nil {
			return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, convertErr
		}
		items[index] = converted
		expires = minTime(expires, item.ExpiresAt)
	}
	checked := a.clock()
	if checked.IsZero() || checked.Before(now) || !checked.Before(expires) {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrNotCurrent
	}
	envelope, err := applicationcontract.SealContextOwnerSourceEnvelopeV1(applicationcontract.ContextOwnerSourceEnvelopeV1{
		ID: "knowledge-envelope:" + projection.Ref.Digest, Owner: applicationcontract.ContextOwnerKnowledgeV1,
		SourceSession: request.SourceSession, SessionApplicability: request.SessionApplicability, SourceTurn: request.SourceTurn, TurnApplicability: request.TurnApplicability,
		AttemptInspectionRef: ownerRef("knowledge/inspection", inspection.Ref), CurrentProjectionRef: ownerRef("knowledge/projection", projection.Ref),
		StableClosureDigest: core.Digest(projection.StableClosureDigest), Items: items, Phase: request.Phase,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil || envelope.ValidateCurrent(checked) != nil {
		return applicationcontract.ContextOwnerSourceEnvelopeV1{}, contextsource.CurrentRequestV2{}, contextsource.CurrentProjectionV2{}, ownercontract.ErrEvidenceConflict
	}
	return envelope, currentRequest, projection, nil
}

func (a *AdapterV1) ReadContextOwnerContentExactV1(ctx context.Context, request applicationcontract.ContextOwnerContentRequestV1) (applicationcontract.ContextOwnerContentObservationV1, []byte, error) {
	if a == nil || ctx == nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrInvalidArgument
	}
	now := a.clock()
	if now.IsZero() || request.ValidateCurrent(now) != nil || request.SourceRequest.Owner != applicationcontract.ContextOwnerKnowledgeV1 {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrInvalidArgument
	}
	fresh, ownerRequest, projection, err := a.inspect(ctx, request.SourceRequest)
	if err != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, err
	}
	if fresh.StableAssociationDigest != request.Envelope.StableAssociationDigest || int(request.Rank) >= len(projection.Items) || fresh.Items[request.Rank].Digest != request.Envelope.Items[request.Rank].Digest {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrEvidenceConflict
	}
	notAfter := minTime(time.Unix(0, request.RequestedNotAfterNano), time.Unix(0, request.SourceRequest.RequestedNotAfterNano), projection.ExpiresAt)
	exact, err := contextsource.SealExactContentRequestV2(contextsource.ExactContentRequestV2{
		Coordinate: ownerRequest.Coordinate, Projection: projection, Rank: int(request.Rank), CheckPhase: projection.CheckPhase,
		ExpectedStableClosureDigest: projection.StableClosureDigest, MaxBodyBytes: request.MaxBodyBytes,
		CheckedUpperBound: projection.OwnerCheckedAt, NotAfter: notAfter,
	})
	if err != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, err
	}
	ownerObservation, body, err := a.owner.ReadContentExact(ctx, exact)
	if err != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, err
	}
	if ownerObservation.Validate() != nil || int64(len(body)) != ownerObservation.ObservedLength || core.DigestBytes(body) != core.Digest(ownerObservation.ObservedDigest) || ownerObservation.ContentRef.Digest != projection.Items[request.Rank].ContentRef.Digest || ownerObservation.License != projection.Items[request.Rank].License || ownerObservation.LicenseDigest != projection.Items[request.Rank].LicenseDigest {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrEvidenceConflict
	}
	completed := a.clock()
	expires := minTime(ownerObservation.ExpiresAt, notAfter, time.Unix(0, fresh.ExpiresUnixNano))
	if completed.IsZero() || completed.Before(now) || !completed.Before(expires) {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrNotCurrent
	}
	observation, err := applicationcontract.SealContextOwnerContentObservationV1(applicationcontract.ContextOwnerContentObservationV1{
		ID: "knowledge-content-observation:" + ownerObservation.Ref.Digest, Owner: applicationcontract.ContextOwnerKnowledgeV1,
		EnvelopeRef:          applicationcontract.ContextRefreshExactRefV1{Kind: knowledgeEnvelopeKindV1, ID: fresh.ID, Revision: fresh.Revision, Digest: fresh.Digest},
		ProjectionItemDigest: core.Digest(ownerObservation.ProjectionItemDigest), ContentRef: ownerContentRef(ownerObservation.ContentRef),
		ObservedLength: ownerObservation.ObservedLength, ObservedDigest: core.Digest(ownerObservation.ObservedDigest), CheckedUnixNano: completed.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil || observation.ValidateCurrent(completed) != nil {
		return applicationcontract.ContextOwnerContentObservationV1{}, nil, ownercontract.ErrEvidenceConflict
	}
	return observation, bytes.Clone(body), nil
}

func knowledgeRequestMatchesApplication(owner contextsource.CurrentRequestV2, app applicationcontract.ContextOwnerSourceRequestV1) bool {
	coordinate := owner.Coordinate
	phase := applicationcontract.ContextSourceCheckS1V1
	if owner.CheckPhase == contextsource.CheckPhaseS2V2 {
		phase = applicationcontract.ContextSourceCheckS2V1
	}
	return phase == app.Phase && coordinate.SessionRef.ID == app.SourceSession.ID && coordinate.SessionRef.Revision == uint64(app.SourceSession.Revision) && coordinate.SessionRef.Digest == string(app.SourceSession.Digest) && coordinate.SessionEvidenceRef.ID == app.SessionApplicability.ID && coordinate.SessionEvidenceRef.Revision == uint64(app.SessionApplicability.Revision) && coordinate.SessionEvidenceRef.Digest == string(app.SessionApplicability.Digest) && coordinate.SessionCheckedAt.UnixNano() == app.SourceSession.CheckedUnixNano && coordinate.SessionExpiresAt.UnixNano() == app.SourceSession.ExpiresUnixNano && coordinate.SourceTurnRef.ID == app.SourceTurn.ID && coordinate.SourceTurnRef.Revision == uint64(app.SourceTurn.Revision) && coordinate.SourceTurnRef.Digest == string(app.SourceTurn.Digest) && coordinate.TurnEvidenceRef.ID == app.TurnApplicability.ID && coordinate.TurnEvidenceRef.Revision == uint64(app.TurnApplicability.Revision) && coordinate.TurnEvidenceRef.Digest == string(app.TurnApplicability.Digest) && coordinate.SourceTurnRef == coordinate.TurnEvidenceRef && coordinate.SourceTurnOrdinal == app.SourceTurn.Ordinal && coordinate.LegacyTurnID == app.SourceTurn.ID && owner.NotAfter.UnixNano() <= app.RequestedNotAfterNano && (phase != applicationcontract.ContextSourceCheckS2V1 || core.Digest(owner.ExpectedS1ClosureDigest) == app.ExpectedOwnerClosure)
}

func knowledgeRequestForPhase(owner contextsource.CurrentRequestV2, app applicationcontract.ContextOwnerSourceRequestV1) (contextsource.CurrentRequestV2, error) {
	if app.Phase == applicationcontract.ContextSourceCheckS1V1 {
		sealed, err := contextsource.SealCurrentRequestV2(owner)
		if err != nil || !reflect.DeepEqual(sealed, owner) {
			return contextsource.CurrentRequestV2{}, ownercontract.ErrEvidenceConflict
		}
		return owner, nil
	}
	if owner.CheckPhase == contextsource.CheckPhaseS1V2 {
		owner.CheckPhase = contextsource.CheckPhaseS2V2
		owner.ExpectedS1ClosureDigest = string(app.ExpectedOwnerClosure)
		owner.ProjectionID += "/s2"
		owner.ProjectionRevision++
		owner.Digest = ""
		return contextsource.SealCurrentRequestV2(owner)
	}
	sealed, err := contextsource.SealCurrentRequestV2(owner)
	if err != nil || !reflect.DeepEqual(sealed, owner) {
		return contextsource.CurrentRequestV2{}, ownercontract.ErrEvidenceConflict
	}
	return owner, nil
}

func knowledgeItem(item contextsource.ProjectionItemV2, sensitivity string) (applicationcontract.ContextOwnerSourceItemV1, error) {
	chain := make([]applicationcontract.ContextRefreshExactRefV1, 0, len(item.SourceRefs)+len(item.EvidenceRefs)+len(item.ProjectionRefs)+4)
	chain = append(chain, ownerRef("knowledge/package", item.PackageRef), ownerRef("knowledge/snapshot", item.SnapshotRef))
	for _, ref := range item.SourceRefs {
		chain = append(chain, ownerRef("knowledge/source", ref))
	}
	for _, ref := range item.EvidenceRefs {
		chain = append(chain, ownerRef("knowledge/evidence", ref))
	}
	for _, ref := range item.ProjectionRefs {
		chain = append(chain, ownerRef("knowledge/projection", ref))
	}
	chain = append(chain, ownerRef("knowledge/domain-result", item.DomainResultRef), ownerRef("knowledge/application", item.ApplicationRef))
	converted, err := applicationcontract.SealContextOwnerSourceItemV1(applicationcontract.ContextOwnerSourceItemV1{
		Rank: uint32(item.Rank), ItemDigest: core.Digest(item.Digest), RecordRef: ownerRef("knowledge/record", item.RecordRef), StableOwnerChain: chain,
		ContentRef: ownerContentRef(item.ContentRef), TokenEstimate: uint64(item.TokenEstimate), Sensitivity: sensitivity,
		CitationDigest: core.Digest(item.CitationDigest), License: item.License, ExpiresUnixNano: item.ExpiresAt.UnixNano(),
	})
	if err != nil || converted.ValidateCurrent(item.ExpiresAt.Add(-time.Nanosecond)) != nil {
		return applicationcontract.ContextOwnerSourceItemV1{}, ownercontract.ErrEvidenceConflict
	}
	return converted, nil
}

func ownerRef(kind string, ref ownercontract.Ref) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: ref.ID, Revision: core.Revision(ref.Revision), Digest: core.Digest(ref.Digest)}
}

func ownerContentRef(ref ownercontract.ContentRef) applicationcontract.ContextOwnerContentRefV1 {
	return applicationcontract.ContextOwnerContentRefV1{ID: ref.ID, Digest: core.Digest(ref.Digest), Length: ref.Length, MediaType: ref.MediaType}
}

func minTime(values ...time.Time) time.Time {
	result := values[0]
	for _, value := range values[1:] {
		if value.Before(result) {
			result = value
		}
	}
	return result
}
