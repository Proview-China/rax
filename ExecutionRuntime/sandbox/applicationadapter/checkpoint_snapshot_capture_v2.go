package applicationadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const CheckpointSnapshotCaptureBindingContractVersionV2 = "praxis.sandbox/checkpoint-snapshot-capture-binding/v2"

type CheckpointSnapshotCapturePlanV2 struct {
	DataDomain               string                              `json:"data_domain"`
	SchemaRef                contract.Ref                        `json:"schema_ref"`
	RetentionPolicyRef       contract.Ref                        `json:"retention_policy_ref"`
	EncryptionPolicyRef      contract.Ref                        `json:"encryption_policy_ref"`
	ResidencyPolicyRef       contract.Ref                        `json:"residency_policy_ref"`
	StorageNamespaceExactRef contract.SnapshotArtifactExactRefV2 `json:"storage_namespace_exact_ref"`
	EncryptionFactRef        contract.Ref                        `json:"encryption_fact_ref"`
	ResidencyFactRef         contract.Ref                        `json:"residency_fact_ref"`
	WorkspaceStableID        string                              `json:"workspace_stable_id"`
	CoveragePolicyRef        contract.Ref                        `json:"coverage_policy_ref"`
	Included                 []string                            `json:"included"`
	DeclaredExcluded         []string                            `json:"declared_excluded"`
	ResidualRefs             []contract.Ref                      `json:"residual_refs"`
	RequestedNotAfter        int64                               `json:"requested_not_after"`
}

func (p CheckpointSnapshotCapturePlanV2) Clone() CheckpointSnapshotCapturePlanV2 {
	p.Included = append([]string(nil), p.Included...)
	p.DeclaredExcluded = append([]string(nil), p.DeclaredExcluded...)
	p.ResidualRefs = append([]contract.Ref(nil), p.ResidualRefs...)
	return p
}

func (p CheckpointSnapshotCapturePlanV2) ValidateCurrent(now time.Time) error {
	if p.DataDomain != contract.WorkspaceSnapshotDataDomain || strings.TrimSpace(p.WorkspaceStableID) == "" || p.SchemaRef.ValidateShape("checkpoint snapshot schema") != nil || p.RetentionPolicyRef.ValidateShape("checkpoint snapshot retention policy") != nil || p.EncryptionPolicyRef.ValidateShape("checkpoint snapshot encryption policy") != nil || p.ResidencyPolicyRef.ValidateShape("checkpoint snapshot residency policy") != nil || p.StorageNamespaceExactRef.ValidateCurrent("checkpoint snapshot storage namespace", now) != nil || p.EncryptionFactRef.ValidateShape("checkpoint snapshot encryption fact") != nil || p.ResidencyFactRef.ValidateShape("checkpoint snapshot residency fact") != nil || p.CoveragePolicyRef.ValidateShape("checkpoint snapshot coverage policy") != nil || p.RequestedNotAfter <= 0 || now.IsZero() || now.UnixNano() >= p.RequestedNotAfter {
		return errors.New("checkpoint snapshot capture plan is incomplete or stale")
	}
	if len(p.Included) == 0 || contract.ValidateSortedUnique(p.Included, "checkpoint snapshot included coverage") != nil || contract.ValidateSortedUnique(p.DeclaredExcluded, "checkpoint snapshot declared exclusions") != nil {
		return errors.New("checkpoint snapshot coverage is empty or non-canonical")
	}
	if !slices.IsSortedFunc(p.ResidualRefs, compareCheckpointSnapshotRefV2) {
		return errors.New("checkpoint snapshot residual refs are non-canonical")
	}
	for index, ref := range p.ResidualRefs {
		if ref.ValidateShape("checkpoint snapshot residual") != nil || index > 0 && contract.SameRef(p.ResidualRefs[index-1], ref) {
			return errors.New("checkpoint snapshot residual refs are invalid or non-canonical")
		}
	}
	return nil
}

type CheckpointSnapshotCaptureBindingV2 struct {
	ContractVersion              string                                            `json:"contract_version"`
	SnapshotReservation          contract.SnapshotArtifactExactRefV2               `json:"snapshot_reservation"`
	CheckpointReservation        contract.Ref                                      `json:"checkpoint_reservation"`
	StorageArtifactRef           contract.SnapshotStorageArtifactRefV2             `json:"storage_artifact_ref"`
	ProviderArtifact             contract.CheckpointWorkspaceArtifactObservationV2 `json:"provider_artifact"`
	MaterializationInspectionRef contract.Ref                                      `json:"materialization_inspection_ref"`
	WorkspaceBundleDigest        string                                            `json:"workspace_bundle_digest"`
	ProviderObservationRef       contract.Ref                                      `json:"provider_observation_ref"`
	ProviderReceiptRef           contract.Ref                                      `json:"provider_receipt_ref"`
	EvidenceConsumptionRef       contract.Ref                                      `json:"evidence_consumption_ref"`
	OwnerInspectionRef           contract.Ref                                      `json:"owner_inspection_ref"`
	SourceAttemptRef             contract.Ref                                      `json:"source_attempt_ref"`
	TenantID                     string                                            `json:"tenant_id"`
	ScopeDigest                  string                                            `json:"scope_digest"`
	RunID                        string                                            `json:"run_id"`
	CheckpointAttemptRef         contract.Ref                                      `json:"checkpoint_attempt_ref"`
	BarrierRef                   contract.Ref                                      `json:"barrier_ref"`
	EffectCutRef                 contract.Ref                                      `json:"effect_cut_ref"`
	ParticipantID                string                                            `json:"participant_id"`
	ParticipantDigest            string                                            `json:"participant_digest"`
	WorkspaceStableID            string                                            `json:"workspace_stable_id"`
	CoveragePolicyRef            contract.Ref                                      `json:"coverage_policy_ref"`
	Included                     []string                                          `json:"included"`
	DeclaredExcluded             []string                                          `json:"declared_excluded"`
	ResidualRefs                 []contract.Ref                                    `json:"residual_refs"`
	RequestedNotAfter            int64                                             `json:"requested_not_after"`
	ExpiresUnixNano              int64                                             `json:"expires_unix_nano"`
	Digest                       string                                            `json:"digest"`
}

func (v CheckpointSnapshotCaptureBindingV2) Clone() CheckpointSnapshotCaptureBindingV2 {
	v.Included = append([]string(nil), v.Included...)
	v.DeclaredExcluded = append([]string(nil), v.DeclaredExcluded...)
	v.ResidualRefs = append([]contract.Ref(nil), v.ResidualRefs...)
	return v
}

func SealCheckpointSnapshotCaptureBindingV2(value CheckpointSnapshotCaptureBindingV2, now time.Time) (CheckpointSnapshotCaptureBindingV2, error) {
	value = value.Clone()
	slices.Sort(value.Included)
	slices.Sort(value.DeclaredExcluded)
	slices.SortFunc(value.ResidualRefs, compareCheckpointSnapshotRefV2)
	value.ContractVersion = CheckpointSnapshotCaptureBindingContractVersionV2
	value.Digest = ""
	digest, err := contract.Digest("praxis.sandbox/checkpoint-snapshot-capture-binding/body/v2", value)
	if err != nil {
		return CheckpointSnapshotCaptureBindingV2{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(now)
}

func (v CheckpointSnapshotCaptureBindingV2) ValidateCurrent(now time.Time) error {
	if v.ContractVersion != CheckpointSnapshotCaptureBindingContractVersionV2 || v.SnapshotReservation.ValidateCurrent("checkpoint snapshot Reservation", now) != nil || v.CheckpointReservation.ValidateShape("checkpoint phase Reservation") != nil || v.StorageArtifactRef.ValidateCurrent(now) != nil || v.ProviderArtifact.ValidateCurrent(now) != nil || v.MaterializationInspectionRef.ValidateShape("checkpoint workspace materialization inspection") != nil || !contract.ValidDigest(v.WorkspaceBundleDigest) || v.ProviderObservationRef.ValidateShape("checkpoint Provider observation") != nil || v.ProviderReceiptRef.ValidateShape("checkpoint Provider receipt") != nil || v.EvidenceConsumptionRef.ValidateShape("checkpoint Evidence consumption") != nil || v.OwnerInspectionRef.ValidateShape("checkpoint Owner inspection") != nil || v.SourceAttemptRef.ValidateShape("checkpoint source attempt") != nil || strings.TrimSpace(v.TenantID) == "" || !contract.ValidDigest(v.ScopeDigest) || strings.TrimSpace(v.RunID) == "" || v.CheckpointAttemptRef.ValidateShape("checkpoint snapshot Runtime attempt") != nil || v.BarrierRef.ValidateShape("checkpoint snapshot Barrier") != nil || v.EffectCutRef.ValidateShape("checkpoint snapshot Effect Cut") != nil || strings.TrimSpace(v.ParticipantID) == "" || !contract.ValidDigest(v.ParticipantDigest) || strings.TrimSpace(v.WorkspaceStableID) == "" || v.CoveragePolicyRef.ValidateShape("checkpoint snapshot coverage policy") != nil || v.RequestedNotAfter <= 0 || v.ExpiresUnixNano <= 0 || now.IsZero() || now.UnixNano() >= v.ExpiresUnixNano || !contract.ValidDigest(v.Digest) {
		return errors.New("checkpoint snapshot capture binding is incomplete or stale")
	}
	if len(v.Included) == 0 || contract.ValidateSortedUnique(v.Included, "checkpoint snapshot included coverage") != nil || contract.ValidateSortedUnique(v.DeclaredExcluded, "checkpoint snapshot declared exclusions") != nil || !slices.IsSortedFunc(v.ResidualRefs, compareCheckpointSnapshotRefV2) {
		return errors.New("checkpoint snapshot capture binding coverage is non-canonical")
	}
	for index, ref := range v.ResidualRefs {
		if ref.ValidateShape("checkpoint snapshot residual") != nil || index > 0 && contract.SameRef(v.ResidualRefs[index-1], ref) {
			return errors.New("checkpoint snapshot capture binding residual refs are invalid")
		}
	}
	if v.ExpiresUnixNano > v.RequestedNotAfter || v.ExpiresUnixNano > v.SnapshotReservation.ExpiresUnixNano || v.ExpiresUnixNano > v.StorageArtifactRef.ExpiresUnixNano || v.ExpiresUnixNano > v.ProviderArtifact.ExpiresUnixNano {
		return errors.New("checkpoint snapshot capture binding extends an upstream TTL")
	}
	copy := v.Clone()
	copy.Digest = ""
	digest, err := contract.Digest("praxis.sandbox/checkpoint-snapshot-capture-binding/body/v2", copy)
	if err != nil || digest != v.Digest {
		return errors.New("checkpoint snapshot capture binding digest drifted")
	}
	return nil
}

type CheckpointSnapshotCaptureBindingStoreV2 interface {
	CreateCheckpointSnapshotCaptureBindingV2(context.Context, CheckpointSnapshotCaptureBindingV2) (CheckpointSnapshotCaptureBindingV2, error)
	InspectCheckpointSnapshotCaptureBindingV2(context.Context, contract.SnapshotArtifactExactRefV2) (CheckpointSnapshotCaptureBindingV2, error)
}

// CheckpointSnapshotArtifactCurrentReaderV2 independently re-reads the exact
// Provider attempt, Evidence consumption, applied Sandbox phase, and Snapshot
// Reservation. It never calls Provider Dispatch and never writes an Artifact.
type CheckpointSnapshotArtifactCurrentReaderV2 struct {
	composition       *CheckpointProductionCompositionV2
	bindings          CheckpointSnapshotCaptureBindingStoreV2
	artifacts         ports.SnapshotArtifactStoreV2
	checkpointContent ports.CheckpointWorkspaceArtifactReaderV2
	snapshotContent   ports.SnapshotContentStoreV2
	clock             func() time.Time
}

func NewCheckpointSnapshotArtifactCurrentReaderV2(composition *CheckpointProductionCompositionV2, bindings CheckpointSnapshotCaptureBindingStoreV2, artifacts ports.SnapshotArtifactStoreV2, checkpointContent ports.CheckpointWorkspaceArtifactReaderV2, snapshotContent ports.SnapshotContentStoreV2, clock func() time.Time) (*CheckpointSnapshotArtifactCurrentReaderV2, error) {
	if composition == nil || composition.ActualPoint == nil || composition.ProviderBoundary == nil || composition.Store == nil || nilLike(bindings) || nilLike(artifacts) || nilLike(checkpointContent) || nilLike(snapshotContent) || clock == nil {
		return nil, errors.New("checkpoint Snapshot Artifact current Reader dependencies are required")
	}
	return &CheckpointSnapshotArtifactCurrentReaderV2{composition: composition, bindings: bindings, artifacts: artifacts, checkpointContent: checkpointContent, snapshotContent: snapshotContent, clock: clock}, nil
}

func (r *CheckpointSnapshotArtifactCurrentReaderV2) InspectSnapshotArtifactCommitCurrentV2(ctx context.Context, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
	now := r.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, err
	}
	binding, err := r.bindings.InspectCheckpointSnapshotCaptureBindingV2(ctx, request.ReservationRef)
	if err != nil || binding.ValidateCurrent(now) != nil {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint snapshot binding is unavailable", ports.ErrStale)
	}
	if !checkpointSnapshotBindingMatchesRequestV2(binding, request) {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint snapshot commit request crossed its binding", ports.ErrConflict)
	}
	materialized, err := r.checkpointContent.InspectCheckpointWorkspaceArtifactV2(ctx, checkpointWorkspaceInspectionRequestV2(binding))
	if err != nil || materialized.ValidateCurrent(now) != nil || materialized.Digest != binding.MaterializationInspectionRef.Digest || materialized.Bundle.BundleDigest != binding.WorkspaceBundleDigest {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint workspace materialization is not exact current", ports.ErrConflict)
	}
	content, err := r.snapshotContent.InspectSnapshotContentV2(ctx, &contract.InspectSnapshotContentRequestV2{ExpectedRef: binding.StorageArtifactRef})
	if err != nil || content.StorageRef != binding.StorageArtifactRef {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: materialized Snapshot content is unavailable", ports.ErrStale)
	}
	bundle, err := contract.DecodeWorkspaceSnapshotBundleV1(content.Content)
	if err != nil || bundle.BundleDigest != binding.WorkspaceBundleDigest || bundle.BundleDigest != materialized.Bundle.BundleDigest {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: materialized Snapshot content drifted", ports.ErrConflict)
	}
	reservation, err := r.artifacts.InspectSnapshotArtifactReservation(ctx, binding.SnapshotReservation)
	if err != nil || reservation.ValidateCurrent(now) != nil || !contract.SameRef(reservation.SourceAttemptRef, binding.SourceAttemptRef) {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: Snapshot Artifact Reservation is not exact current", ports.ErrStale)
	}
	phase, err := r.composition.Store.InspectCheckpointPhaseFactByReservation(ctx, binding.CheckpointReservation)
	if err != nil || phase.ValidateCurrent(now) != nil || phase.State != contract.CheckpointPhasePrepared || !contract.SameRef(phase.CheckpointAttemptRef, binding.SourceAttemptRef) {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint prepare phase is not applied current", ports.ErrStale)
	}
	providerCurrent, err := r.composition.ActualPoint.InspectCheckpointPhaseResultCurrentV2(ctx, binding.CheckpointReservation)
	if err != nil || providerCurrent.ValidateCurrent(now) != nil || providerCurrent.State != contract.CheckpointPhasePrepared || !contract.SameRef(providerCurrent.ProviderObservation, binding.ProviderObservationRef) || !contract.SameRef(providerCurrent.ProviderReceipt, binding.ProviderReceiptRef) || !contract.SameRef(providerCurrent.EvidenceConsumption, binding.EvidenceConsumptionRef) || providerCurrent.ProjectionDigest != binding.OwnerInspectionRef.Digest {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Provider/Evidence current drifted", ports.ErrConflict)
	}
	fresh := r.clock()
	if !fresh.Before(time.Unix(0, binding.ExpiresUnixNano)) {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint snapshot current expired", ports.ErrStale)
	}
	projection, err := contract.SealSnapshotArtifactCommitCurrentProjectionV2(contract.SnapshotArtifactCommitCurrentProjectionV2{
		TenantID: reservation.TenantID, DataDomain: reservation.DataDomain, ReservationRef: binding.SnapshotReservation,
		ExpectedAggregateRef: request.ExpectedAggregateRef, StorageArtifactRef: binding.StorageArtifactRef,
		ProviderObservationRef: binding.ProviderObservationRef, ProviderReceiptRef: binding.ProviderReceiptRef,
		FormalEvidenceRefs: []contract.Ref{binding.EvidenceConsumptionRef}, OwnerInspectionRef: binding.OwnerInspectionRef,
		SourceAttemptRef: binding.SourceAttemptRef, CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: binding.ExpiresUnixNano,
	}, fresh)
	if err != nil || !projection.MatchesRequest(request) {
		return contract.SnapshotArtifactCommitCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint snapshot projection does not match request", ports.ErrConflict)
	}
	return projection, nil
}

type CheckpointSnapshotCaptureV2 struct {
	composition       *CheckpointProductionCompositionV2
	bindings          CheckpointSnapshotCaptureBindingStoreV2
	artifacts         ports.SnapshotArtifactOwnerPortV2
	checkpointContent ports.CheckpointWorkspaceArtifactReaderV2
	snapshotContent   ports.SnapshotContentStoreV2
	clock             func() time.Time
}

func NewCheckpointSnapshotCaptureV2(composition *CheckpointProductionCompositionV2, bindings CheckpointSnapshotCaptureBindingStoreV2, artifacts ports.SnapshotArtifactOwnerPortV2, checkpointContent ports.CheckpointWorkspaceArtifactReaderV2, snapshotContent ports.SnapshotContentStoreV2, clock func() time.Time) (*CheckpointSnapshotCaptureV2, error) {
	if composition == nil || composition.ActualPoint == nil || composition.ProviderBoundary == nil || composition.Store == nil || nilLike(bindings) || nilLike(artifacts) || nilLike(checkpointContent) || nilLike(snapshotContent) || clock == nil {
		return nil, errors.New("checkpoint Snapshot capture dependencies are required")
	}
	return &CheckpointSnapshotCaptureV2{composition: composition, bindings: bindings, artifacts: artifacts, checkpointContent: checkpointContent, snapshotContent: snapshotContent, clock: clock}, nil
}

func (c *CheckpointSnapshotCaptureV2) CapturePreparedCheckpointSnapshotV2(ctx context.Context, checkpointReservation contract.Ref, work appcontract.CheckpointParticipantWorkRequestV1, plan CheckpointSnapshotCapturePlanV2) (contract.CommitSnapshotArtifactResultV2, error) {
	now := c.clock()
	if checkpointReservation.ValidateShape("checkpoint snapshot phase Reservation") != nil || work.Validate(now) != nil || plan.ValidateCurrent(now) != nil {
		return contract.CommitSnapshotArtifactResultV2{}, errors.New("checkpoint Snapshot capture request is invalid")
	}
	phase, err := c.composition.Store.InspectCheckpointPhaseFactByReservation(ctx, checkpointReservation)
	if err != nil || phase.ValidateCurrent(now) != nil || phase.Phase != contract.CheckpointPhasePrepare || phase.State != contract.CheckpointPhasePrepared {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: checkpoint prepare is not applied", ports.ErrStale)
	}
	providerCurrent, response, err := c.inspectCheckpointArtifactV2(ctx, checkpointReservation)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	artifact := response.ProviderObservation.CheckpointArtifact
	providerArtifact := contract.CheckpointWorkspaceArtifactObservationV2{Provider: response.ProviderObservation.Provider, ArtifactID: artifact.ArtifactID, SubjectDigest: artifact.SubjectDigest, ContentDigest: artifact.ContentDigest, ContentLength: artifact.ContentLength, State: artifact.State, CheckpointPhase: artifact.CheckpointPhase, RecordedUnixNano: artifact.RecordedUnixNano, ExpiresUnixNano: artifact.ExpiresUnixNano}
	materialized, err := c.checkpointContent.InspectCheckpointWorkspaceArtifactV2(ctx, &contract.InspectCheckpointWorkspaceArtifactRequestV2{Observation: providerArtifact, SnapshotID: artifact.ArtifactID, TenantID: phase.TenantID, SourceScopeDigest: trimCheckpointRuntimeDigestV2(work.Gate.ScopeDigest)})
	if err != nil || materialized.ValidateCurrent(now) != nil {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: checkpoint workspace content cannot be independently materialized", ports.ErrConflict)
	}
	encoded, err := contract.EncodeWorkspaceSnapshotBundleV1(materialized.Bundle)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	expires := minimumInt64(plan.RequestedNotAfter, artifact.ExpiresUnixNano, providerCurrent.ExpiresUnixNano, plan.StorageNamespaceExactRef.ExpiresUnixNano, materialized.ExpiresUnixNano)
	put, err := c.snapshotContent.PutSnapshotContentV2(ctx, &contract.PutSnapshotContentRequestV2{TenantID: phase.TenantID, DataDomain: plan.DataDomain, Content: encoded, SchemaRef: plan.SchemaRef, EncryptionFactRef: plan.EncryptionFactRef, ResidencyFactRef: plan.ResidencyFactRef, RequestedNotAfter: expires})
	if err != nil || put.StorageRef.ValidateCurrent(now) != nil || !contract.SameSnapshotArtifactExactRef(put.StorageRef.StorageNamespaceExactRef, plan.StorageNamespaceExactRef) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: checkpoint workspace content was not durably stored", ports.ErrConflict)
	}
	expires = minimumInt64(expires, put.StorageRef.ExpiresUnixNano)
	reserve := contract.ReserveArtifactRequestV2{
		TenantID: phase.TenantID, DataDomain: plan.DataDomain, SourceOperationID: phase.OperationID, SourceEffectID: phase.EffectID,
		SourceAttemptRef: phase.CheckpointAttemptRef, SchemaRef: plan.SchemaRef, ExpectedContentDigest: put.StorageRef.ContentDigest,
		RetentionPolicyRef: plan.RetentionPolicyRef, EncryptionPolicyRef: plan.EncryptionPolicyRef, ResidencyPolicyRef: plan.ResidencyPolicyRef,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactAbsent}, RequestedNotAfter: expires,
	}
	reserved, err := c.artifacts.ReserveArtifact(ctx, &reserve)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	storage := put.StorageRef
	ownerInspection := contract.Ref{ID: checkpointReservation.ID + "-checkpoint-current", Revision: checkpointReservation.Revision, Digest: providerCurrent.ProjectionDigest}
	materializationInspection := contract.Ref{ID: artifact.ArtifactID + "-workspace-materialization", Revision: 1, Digest: materialized.Digest}
	binding, err := SealCheckpointSnapshotCaptureBindingV2(CheckpointSnapshotCaptureBindingV2{
		SnapshotReservation: reserved.Reservation.ExactRef(), CheckpointReservation: checkpointReservation, StorageArtifactRef: storage, ProviderArtifact: providerArtifact, MaterializationInspectionRef: materializationInspection, WorkspaceBundleDigest: materialized.Bundle.BundleDigest,
		ProviderObservationRef: providerCurrent.ProviderObservation, ProviderReceiptRef: providerCurrent.ProviderReceipt,
		EvidenceConsumptionRef: providerCurrent.EvidenceConsumption, OwnerInspectionRef: ownerInspection,
		SourceAttemptRef: phase.CheckpointAttemptRef, TenantID: string(work.Attempt.TenantID), ScopeDigest: trimCheckpointRuntimeDigestV2(work.Gate.ScopeDigest), RunID: string(work.Gate.RunID),
		CheckpointAttemptRef: checkpointLocalRefV2(work.Attempt.ID, uint64(work.Attempt.Revision), work.Attempt.Digest), BarrierRef: checkpointLocalRefV2(work.Barrier.ID, uint64(work.Barrier.Revision), work.Barrier.Digest), EffectCutRef: checkpointLocalRefV2(work.EffectCut.ID, uint64(work.EffectCut.Revision), work.EffectCut.Digest),
		ParticipantID: work.Participant.ID, ParticipantDigest: trimCheckpointRuntimeDigestV2(work.Participant.Digest), WorkspaceStableID: plan.WorkspaceStableID, CoveragePolicyRef: plan.CoveragePolicyRef,
		Included: append([]string(nil), plan.Included...), DeclaredExcluded: append([]string(nil), plan.DeclaredExcluded...), ResidualRefs: append([]contract.Ref(nil), plan.ResidualRefs...),
		RequestedNotAfter: expires, ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	stored, err := c.bindings.CreateCheckpointSnapshotCaptureBindingV2(ctx, binding)
	if err != nil {
		stored, err = c.bindings.InspectCheckpointSnapshotCaptureBindingV2(context.WithoutCancel(ctx), reserved.Reservation.ExactRef())
		if err != nil {
			return contract.CommitSnapshotArtifactResultV2{}, err
		}
	}
	if !reflect.DeepEqual(stored, binding) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: checkpoint Snapshot binding winner differs", ports.ErrConflict)
	}
	if reserved.CurrentIndex.AggregateState == contract.SnapshotArtifactAggregateAvailable && reserved.CurrentIndex.ArtifactFactRef.Ref != nil {
		fact, inspectErr := c.artifacts.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: *reserved.CurrentIndex.ArtifactFactRef.Ref})
		if inspectErr != nil {
			return contract.CommitSnapshotArtifactResultV2{}, inspectErr
		}
		return contract.CommitSnapshotArtifactResultV2{Fact: fact, CurrentIndex: reserved.CurrentIndex, Created: false}, nil
	}
	if reserved.CurrentIndex.AggregateState != contract.SnapshotArtifactAggregateReserved {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: reserved Snapshot aggregate current is unavailable", ports.ErrStale)
	}
	commit := contract.CommitSnapshotArtifactRequestV2{
		ReservationRef: binding.SnapshotReservation, ExpectedAggregateRef: reserved.CurrentIndex.HeadAggregateEnvelopeRef, StorageArtifactRef: binding.StorageArtifactRef,
		ProviderObservationRef: binding.ProviderObservationRef, ProviderReceiptRef: binding.ProviderReceiptRef,
		FormalEvidenceRefs: []contract.Ref{binding.EvidenceConsumptionRef}, OwnerInspectionRef: binding.OwnerInspectionRef,
		SourceAttemptRef: binding.SourceAttemptRef, RequestedNotAfter: binding.RequestedNotAfter,
	}
	return c.artifacts.CommitArtifact(ctx, &commit)
}

func checkpointWorkspaceInspectionRequestV2(binding CheckpointSnapshotCaptureBindingV2) *contract.InspectCheckpointWorkspaceArtifactRequestV2 {
	return &contract.InspectCheckpointWorkspaceArtifactRequestV2{Observation: binding.ProviderArtifact, SnapshotID: binding.ProviderArtifact.ArtifactID, TenantID: binding.TenantID, SourceScopeDigest: binding.ScopeDigest}
}

func (c *CheckpointSnapshotCaptureV2) inspectCheckpointArtifactV2(ctx context.Context, reservation contract.Ref) (contract.CheckpointPhaseResultCurrentProjectionV2, dataplaneadapter.DispatchResponseV1, error) {
	current, err := c.composition.ActualPoint.InspectCheckpointPhaseResultCurrentV2(ctx, reservation)
	if err != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, dataplaneadapter.DispatchResponseV1{}, err
	}
	binding, err := c.composition.Store.InspectCheckpointProviderResultBindingV2(ctx, reservation)
	if err != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, dataplaneadapter.DispatchResponseV1{}, err
	}
	response, err := c.composition.ProviderBoundary.dataplane.Inspect(context.WithoutCancel(ctx), binding.Execute)
	if err != nil || validateCheckpointProviderInspectionV1(binding.Execute, response, c.clock()) != nil || response.ProviderObservation.CheckpointArtifact == nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, dataplaneadapter.DispatchResponseV1{}, fmt.Errorf("%w: checkpoint artifact exact Inspect is unavailable", ports.ErrUnknownOutcome)
	}
	return current, response, nil
}

func checkpointSnapshotBindingMatchesRequestV2(binding CheckpointSnapshotCaptureBindingV2, request contract.CommitSnapshotArtifactRequestV2) bool {
	return contract.SameSnapshotArtifactExactRef(binding.SnapshotReservation, request.ReservationRef) && binding.StorageArtifactRef == request.StorageArtifactRef && contract.SameRef(binding.ProviderObservationRef, request.ProviderObservationRef) && contract.SameRef(binding.ProviderReceiptRef, request.ProviderReceiptRef) && len(request.FormalEvidenceRefs) == 1 && contract.SameRef(binding.EvidenceConsumptionRef, request.FormalEvidenceRefs[0]) && contract.SameRef(binding.OwnerInspectionRef, request.OwnerInspectionRef) && contract.SameRef(binding.SourceAttemptRef, request.SourceAttemptRef) && binding.RequestedNotAfter == request.RequestedNotAfter
}

func compareCheckpointSnapshotRefV2(left, right contract.Ref) int {
	if result := strings.Compare(left.ID, right.ID); result != 0 {
		return result
	}
	if left.Revision < right.Revision {
		return -1
	}
	if left.Revision > right.Revision {
		return 1
	}
	return strings.Compare(left.Digest, right.Digest)
}

var _ ports.SnapshotArtifactCommitCurrentReaderV2 = (*CheckpointSnapshotArtifactCurrentReaderV2)(nil)
