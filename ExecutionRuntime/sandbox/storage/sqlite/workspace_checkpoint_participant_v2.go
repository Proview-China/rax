package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CommitWorkspaceCheckpointPreparedV2(ctx context.Context, bundle contract.WorkspaceCheckpointPreparedBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	body, err := encode(bundle.Clone())
	if err != nil {
		return false, err
	}
	participant := bundle.Participant.ExactRef()
	coverage := bundle.Coverage.ExactRef()
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_checkpoint_prepared(tenant_id,scope_digest,checkpoint_attempt_id,participant_id,participant_fact_id,participant_revision,participant_digest,coverage_fact_id,coverage_revision,coverage_digest,body) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		bundle.Participant.TenantID, bundle.Participant.ScopeDigest, bundle.Participant.CheckpointAttemptRef.ID, bundle.Participant.ParticipantID,
		participant.ID, participant.Revision, participant.Digest, coverage.ID, coverage.Revision, coverage.Digest, body)
	if err != nil {
		return false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}
	existing, err := s.InspectWorkspaceCheckpointPreparedV2(ctx, contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: bundle.Participant.TenantID, ScopeDigest: bundle.Participant.ScopeDigest, CheckpointAttemptID: bundle.Participant.CheckpointAttemptRef.ID, ParticipantID: bundle.Participant.ParticipantID})
	if err != nil {
		return false, err
	}
	if !contract.SameSnapshotArtifactExactRef(existing.Participant.ExactRef(), participant) || !contract.SameSnapshotArtifactExactRef(existing.Coverage.ExactRef(), coverage) {
		return false, ports.ErrConflict
	}
	return false, nil
}

func (s *Store) InspectWorkspaceCheckpointPreparedV2(ctx context.Context, request contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	if err := request.Validate(); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_checkpoint_prepared WHERE tenant_id=? AND scope_digest=? AND checkpoint_attempt_id=? AND participant_id=?`, request.TenantID, request.ScopeDigest, request.CheckpointAttemptID, request.ParticipantID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	var bundle contract.WorkspaceCheckpointPreparedBundleV2
	if err := decode(body, &bundle); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if err := bundle.ValidateShape(); err != nil || bundle.Participant.TenantID != request.TenantID || bundle.Participant.ScopeDigest != request.ScopeDigest || bundle.Participant.CheckpointAttemptRef.ID != request.CheckpointAttemptID || bundle.Participant.ParticipantID != request.ParticipantID {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrConflict
	}
	return bundle.Clone(), nil
}

func (s *Store) InspectWorkspaceCheckpointParticipantV2(ctx context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointParticipantFactV2, error) {
	if err := request.Validate(contract.WorkspaceCheckpointParticipantTypeURL, contract.WorkspaceCheckpointParticipantDigestDomain, "workspace checkpoint participant"); err != nil {
		return contract.WorkspaceCheckpointParticipantFactV2{}, err
	}
	bundle, err := s.inspectWorkspaceCheckpointBundleByFactV2(ctx, request, true)
	if err != nil {
		return contract.WorkspaceCheckpointParticipantFactV2{}, err
	}
	return bundle.Participant, nil
}

func (s *Store) InspectWorkspaceCheckpointCoverageV2(ctx context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointCoverageFactV2, error) {
	if err := request.Validate(contract.WorkspaceCheckpointCoverageTypeURL, contract.WorkspaceCheckpointCoverageDigestDomain, "workspace checkpoint coverage"); err != nil {
		return contract.WorkspaceCheckpointCoverageFactV2{}, err
	}
	bundle, err := s.inspectWorkspaceCheckpointBundleByFactV2(ctx, request, false)
	if err != nil {
		return contract.WorkspaceCheckpointCoverageFactV2{}, err
	}
	return bundle.Coverage.Clone(), nil
}

func (s *Store) inspectWorkspaceCheckpointBundleByFactV2(ctx context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2, participant bool) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	column := "coverage_fact_id"
	if participant {
		column = "participant_fact_id"
	}
	var body []byte
	query := `SELECT body FROM workspace_checkpoint_prepared WHERE tenant_id=? AND scope_digest=? AND ` + column + `=?`
	if err := s.db.QueryRowContext(ctx, query, request.TenantID, request.ScopeDigest, request.ExpectedRef.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	var bundle contract.WorkspaceCheckpointPreparedBundleV2
	if err := decode(body, &bundle); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	if err := bundle.ValidateShape(); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrConflict
	}
	actual := bundle.Coverage.ExactRef()
	if participant {
		actual = bundle.Participant.ExactRef()
	}
	if !contract.SameSnapshotArtifactExactRef(actual, request.ExpectedRef) {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrConflict
	}
	return bundle.Clone(), nil
}

var _ ports.WorkspaceCheckpointParticipantStoreV2 = (*Store)(nil)
