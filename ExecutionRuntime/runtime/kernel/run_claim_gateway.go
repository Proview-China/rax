package kernel

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunClaimGateway persists component completion observations before associating
// them with Runtime Run facts. It does not inspect effects or complete the Run.
type RunClaimGateway struct {
	evidence ports.EvidencePort
	runs     *RunJournal
}

type RunClaimIngestRequest struct {
	Scope          core.ExecutionScope         `json:"scope"`
	RunID          core.AgentRunID             `json:"run_id"`
	SourceSequence uint64                      `json:"source_sequence"`
	ClaimKind      core.RunCompletionClaimKind `json:"claim_kind"`
	CausationID    string                      `json:"causation_id"`
	Observation    ports.ExecutionObservation  `json:"observation"`
}

type RunClaimIngestResult struct {
	Run      core.AgentRunRecord     `json:"run"`
	Evidence ports.EvidenceRecordRef `json:"evidence"`
}

func NewRunClaimGateway(evidence ports.EvidencePort, runs *RunJournal) (*RunClaimGateway, error) {
	if evidence == nil || runs == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "evidence port and run journal are required")
	}
	return &RunClaimGateway{evidence: evidence, runs: runs}, nil
}

func (g *RunClaimGateway) Ingest(ctx context.Context, request RunClaimIngestRequest) (RunClaimIngestResult, error) {
	if err := request.Scope.Validate(); err != nil {
		return RunClaimIngestResult{}, err
	}
	if strings.TrimSpace(string(request.RunID)) == "" || request.SourceSequence == 0 || request.CausationID != string(request.RunID) {
		return RunClaimIngestResult{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim requires run id, source sequence and run-bound causation")
	}
	observation := request.Observation
	if strings.TrimSpace(observation.SourceComponentID) == "" || observation.SourceEpoch != request.Scope.Instance.Epoch || observation.ObservedAt.IsZero() {
		return RunClaimIngestResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "claim source and epoch do not match the execution scope")
	}
	if err := ports.ValidateOpaquePayload(observation.Payload); err != nil {
		return RunClaimIngestResult{}, err
	}
	current, err := g.runs.Inspect(ctx, request.Scope, request.RunID)
	if err != nil {
		return RunClaimIngestResult{}, err
	}
	if current.Status == core.RunPending || current.Status == core.RunTerminal || observation.ObservedAt.Before(current.StartedAt) {
		return RunClaimIngestResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "claim does not belong to an active run interval")
	}
	if !validRunClaimKind(request.ClaimKind) {
		return RunClaimIngestResult{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim kind is invalid")
	}

	evidenceRef, err := g.evidence.AppendObservation(ctx, ports.EvidenceObservationRecord{
		SourceID: observation.SourceComponentID, SourceEpoch: observation.SourceEpoch,
		SourceSequence: request.SourceSequence, PayloadDigest: observation.Payload.Digest,
		CausationID: request.CausationID,
	})
	if err != nil {
		return RunClaimIngestResult{}, err
	}
	evidence, err := g.evidence.Read(ctx, evidenceRef)
	if err != nil {
		return RunClaimIngestResult{}, err
	}
	if evidence.Classification != "observation" || evidence.PayloadDigest != observation.Payload.Digest {
		return RunClaimIngestResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "persisted observation evidence does not match the completion claim")
	}
	claim := core.RunCompletionClaim{
		SourceID: observation.SourceComponentID, SourceEpoch: observation.SourceEpoch,
		SourceSequence: request.SourceSequence, Kind: request.ClaimKind,
		PayloadDigest: observation.Payload.Digest, EvidenceScope: evidenceRef.Scope,
		EvidenceSequence: evidenceRef.Sequence, ObservedAt: observation.ObservedAt,
	}
	persisted, err := g.runs.RecordCompletionClaim(ctx, request.Scope, request.RunID, claim)
	if err != nil {
		return RunClaimIngestResult{}, err
	}
	return RunClaimIngestResult{Run: persisted, Evidence: evidenceRef}, nil
}

func validRunClaimKind(kind core.RunCompletionClaimKind) bool {
	switch kind {
	case core.RunClaimCompleted, core.RunClaimCancelled, core.RunClaimFailed, core.RunClaimIndeterminate:
		return true
	default:
		return false
	}
}
