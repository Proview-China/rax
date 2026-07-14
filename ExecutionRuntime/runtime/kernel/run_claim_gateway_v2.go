package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunClaimGatewayV2 persists an exact Evidence V2 record and then a create-once
// association sidecar. It never turns a Claim into Runtime ExecutionOutcome.
type RunClaimGatewayV2 struct {
	Evidence     ports.EvidenceGovernancePortV2
	Associations ports.RunClaimAssociationPortV2
	Runs         control.RunFactPort
	Clock        func() time.Time
}
type RunClaimIngestRequestV2 = ports.RunClaimIngestRequestV2
type RunClaimIngestResultV2 = ports.RunClaimIngestResultV2

func NewRunClaimGatewayV2(evidence ports.EvidenceGovernancePortV2, associations ports.RunClaimAssociationPortV2, runs control.RunFactPort, clock func() time.Time) (*RunClaimGatewayV2, error) {
	if evidence == nil || associations == nil || runs == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V2 run claim gateway requires governed evidence, association, run and clock ports")
	}
	return &RunClaimGatewayV2{Evidence: evidence, Associations: associations, Runs: runs, Clock: clock}, nil
}

func (g *RunClaimGatewayV2) Ingest(ctx context.Context, request RunClaimIngestRequestV2) (RunClaimIngestResultV2, error) {
	return g.IngestRunClaimV2(ctx, request)
}

func (g *RunClaimGatewayV2) IngestRunClaimV2(ctx context.Context, request ports.RunClaimIngestRequestV2) (ports.RunClaimIngestResultV2, error) {
	candidate := request.Candidate
	if err := request.Validate(); err != nil {
		return RunClaimIngestResultV2{}, err
	}
	run, err := g.Runs.InspectRun(ctx, candidate.ExecutionScope, candidate.LedgerScope.RunID)
	if err != nil {
		return RunClaimIngestResultV2{}, err
	}
	if run.ID != candidate.LedgerScope.RunID || !ports.SameExecutionScopeV2(run.Scope, candidate.ExecutionScope) || run.Revision != request.ExpectedRunRevision || (run.Status != core.RunRunning && run.Status != core.RunStopping) || candidate.ObservedUnixNano < run.StartedAt.UnixNano() {
		return RunClaimIngestResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "claim does not bind the exact active run revision and interval")
	}
	record, err := g.Evidence.AppendGoverned(ctx, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: candidate.RegistrationRevision})
	if err != nil {
		original := err
		if !claimRecoveryErrorV2(err) {
			return RunClaimIngestResultV2{}, err
		}
		key := ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}
		record, err = g.Evidence.InspectGovernedBySource(ctx, key)
		if err != nil {
			if core.HasCategory(err, core.ErrorNotFound) {
				return RunClaimIngestResultV2{}, original
			}
			return RunClaimIngestResultV2{}, err
		}
		digest, _ := candidate.DigestV2()
		if record.CandidateDigest != digest {
			return RunClaimIngestResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "inspected claim evidence differs after append uncertainty")
		}
	}
	if err := validateClaimEvidenceRecordV2(candidate, record); err != nil {
		return RunClaimIngestResultV2{}, err
	}
	associatedRun, err := g.Runs.InspectRun(ctx, candidate.ExecutionScope, candidate.LedgerScope.RunID)
	if err != nil {
		return RunClaimIngestResultV2{}, err
	}
	initialIdentity, _ := ports.RunIdentityDigestV2(run)
	associatedIdentity, err := ports.RunIdentityDigestV2(associatedRun)
	if err != nil {
		return RunClaimIngestResultV2{}, err
	}
	if initialIdentity != associatedIdentity || associatedRun.Revision < run.Revision || (associatedRun.Status != core.RunRunning && associatedRun.Status != core.RunStopping) {
		return RunClaimIngestResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "run became terminal or changed identity before claim association")
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(candidate.ExecutionScope)
	associationID, err := ports.RunClaimAssociationIDV2(associatedRun.ID, record.Ref)
	if err != nil {
		return RunClaimIngestResultV2{}, err
	}
	association := ports.RunClaimAssociationFactV2{ContractVersion: ports.RunClaimAssociationContractVersionV2, ID: "claim-association:" + string(record.Ref.RecordDigest), Revision: 1, State: ports.RunClaimAssociatedV2, RunID: associatedRun.ID, RunRevisionAtAssociation: associatedRun.Revision, RunIdentityDigest: associatedIdentity, ExecutionScope: associatedRun.Scope, ExecutionScopeDigest: scopeDigest, LineagePlanDigest: associatedRun.Scope.Lineage.PlanDigest, ClaimKind: candidate.ClaimKind, RegistrationID: candidate.RegistrationID, SourceID: candidate.SourceID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence, EventID: candidate.EventID, Evidence: record.Ref, CandidateDigest: record.CandidateDigest, PayloadDigest: candidate.Payload.ContentDigest, ObservedUnixNano: candidate.ObservedUnixNano, EvidenceIngestedUnixNano: record.IngestedUnixNano, CreatedUnixNano: record.IngestedUnixNano}
	association.ID = associationID
	if err := validateClaimAssociationRecordV2(association, record); err != nil {
		return RunClaimIngestResultV2{}, err
	}
	persisted, err := g.Associations.CreateRunClaimAssociation(ctx, association)
	if err != nil {
		original := err
		if !claimRecoveryErrorV2(err) {
			return RunClaimIngestResultV2{}, err
		}
		persisted, err = g.Associations.InspectRunClaimAssociation(ctx, scopeDigest, associatedRun.ID)
		if err != nil {
			if core.HasCategory(err, core.ErrorNotFound) {
				return RunClaimIngestResultV2{}, original
			}
			return RunClaimIngestResultV2{}, err
		}
		persistedRecord, inspectErr := g.Evidence.InspectGovernedRecord(ctx, persisted.Evidence)
		if inspectErr != nil {
			return RunClaimIngestResultV2{}, inspectErr
		}
		if err := validateClaimAssociationRecordV2(persisted, persistedRecord); err != nil {
			return RunClaimIngestResultV2{}, err
		}
		left, digestErr := persisted.DigestV2()
		if digestErr != nil {
			return RunClaimIngestResultV2{}, digestErr
		}
		right, digestErr := association.DigestV2()
		if digestErr != nil {
			return RunClaimIngestResultV2{}, digestErr
		}
		if left != right {
			return RunClaimIngestResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "run has a conflicting V2 claim association")
		}
	} else if err := validateClaimAssociationRecordV2(persisted, record); err != nil {
		return RunClaimIngestResultV2{}, err
	}
	result := ports.RunClaimIngestResultV2{Run: associatedRun, Evidence: record, Association: persisted}
	return result, result.Validate()
}

func (g *RunClaimGatewayV2) InspectRunClaimV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunClaimIngestResultV2, error) {
	if err := scope.Validate(); err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	association, err := g.Associations.InspectRunClaimAssociation(ctx, scopeDigest, runID)
	if err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	record, err := g.Evidence.InspectGovernedRecord(ctx, association.Evidence)
	if err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	if err := validateClaimAssociationRecordV2(association, record); err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	run, err := g.Runs.InspectRun(ctx, scope, runID)
	if err != nil {
		return ports.RunClaimIngestResultV2{}, err
	}
	result := ports.RunClaimIngestResultV2{Run: run, Evidence: record, Association: association}
	return result, result.Validate()
}

func claimRecoveryErrorV2(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}
func validateClaimEvidenceRecordV2(candidate ports.EvidenceEventCandidateV2, record ports.EvidenceLedgerRecordV2) error {
	if err := control.ValidateEvidenceLedgerRecordV2(record); err != nil {
		return err
	}
	expected, err := candidate.DigestV2()
	if err != nil {
		return err
	}
	actual, err := record.Candidate.DigestV2()
	if err != nil {
		return err
	}
	if record.CandidateDigest != expected || actual != expected {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ledger record does not contain the exact submitted claim candidate")
	}
	return nil
}
func validateClaimAssociationRecordV2(f ports.RunClaimAssociationFactV2, r ports.EvidenceLedgerRecordV2) error {
	if err := control.ValidateEvidenceLedgerRecordV2(r); err != nil {
		return err
	}
	c := r.Candidate
	if c.LedgerScope.Partition != ports.EvidencePartitionRun || c.LedgerScope.RunID != f.RunID || !ports.SameExecutionScopeV2(c.ExecutionScope, f.ExecutionScope) || f.Evidence != r.Ref || f.CandidateDigest != r.CandidateDigest || f.PayloadDigest != c.Payload.ContentDigest || f.RegistrationID != c.RegistrationID || f.SourceID != c.SourceID || f.SourceEpoch != c.SourceEpoch || f.SourceSequence != c.SourceSequence || f.EventID != c.EventID || f.ClaimKind != c.ClaimKind || f.ObservedUnixNano != c.ObservedUnixNano || f.EvidenceIngestedUnixNano != r.IngestedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "claim association does not exactly match ledger record")
	}
	return f.Validate()
}
