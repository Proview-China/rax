package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// PutCheckpointPhaseExecutionPlanV2 is a host-composition write seam. The
// Application-facing lifecycle receives only the exact read capability.
func (s *Store) PutCheckpointPhaseExecutionPlanV2(ctx context.Context, value applicationadapter.CheckpointPhaseExecutionPlanV2) error {
	if err := value.ValidateCurrent(s.clock()); err != nil {
		return err
	}
	body, err := encode(value)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_phase_execution_plans(tenant_id,attempt_id,participant_id,phase,revision,digest,expires_unix_nano,body) VALUES(?,?,?,?,?,?,?,?)`, value.Work.Attempt.TenantID, value.Work.Attempt.ID, value.Work.Participant.ID, value.Provider.Phase, value.Revision, value.Digest, value.ExpiresUnixNano, body)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	existing, err := s.InspectCheckpointPhaseExecutionPlanV2(ctx, value.Work.Attempt, value.Work.Participant, value.Provider.Phase)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(existing, value) {
		return ports.ErrConflict
	}
	return nil
}

func (s *Store) InspectCheckpointPhaseExecutionPlanV2(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2, phase runtimeports.CheckpointParticipantPhaseV2) (applicationadapter.CheckpointPhaseExecutionPlanV2, error) {
	if attempt.Validate() != nil || participant.Validate() != nil {
		return applicationadapter.CheckpointPhaseExecutionPlanV2{}, ports.ErrConflict
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_execution_plans WHERE tenant_id=? AND attempt_id=? AND participant_id=? AND phase=?`, attempt.TenantID, attempt.ID, participant.ID, phase).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return applicationadapter.CheckpointPhaseExecutionPlanV2{}, ports.ErrNotFound
		}
		return applicationadapter.CheckpointPhaseExecutionPlanV2{}, err
	}
	var value applicationadapter.CheckpointPhaseExecutionPlanV2
	if err := decode(body, &value); err != nil {
		return applicationadapter.CheckpointPhaseExecutionPlanV2{}, err
	}
	if err := value.ValidateCurrent(s.clock()); err != nil || value.Work.Attempt != attempt || value.Work.Participant != participant || value.Provider.Phase != phase {
		if err != nil {
			return applicationadapter.CheckpointPhaseExecutionPlanV2{}, err
		}
		return applicationadapter.CheckpointPhaseExecutionPlanV2{}, ports.ErrConflict
	}
	return value, nil
}

var _ applicationadapter.CheckpointPhaseExecutionPlanReaderV2 = (*Store)(nil)
