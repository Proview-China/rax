package sqlite

import (
	"context"
	"database/sql"
	"errors"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// PutLifecyclePlanV4 is the Sandbox Plan Owner's create-once durable write.
// The Application-facing interface remains read-only.
func (s *Store) PutLifecyclePlanV4(ctx context.Context, value applicationadapter.LifecyclePlanEnvelopeV4) error {
	if err := value.ValidateCurrent(value.Ref, s.clock()); err != nil {
		return err
	}
	body, err := encode(value)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO lifecycle_plans(plan_id,revision,digest,expires_unix_nano,body) VALUES(?,?,?,?,?)`, value.Ref.ID, value.Ref.Revision, value.Ref.Digest, value.Ref.ExpiresUnixNano, body)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows == 1 {
		return nil
	}
	existing, err := s.InspectLifecyclePlanV4(ctx, value.Ref)
	if err != nil {
		return err
	}
	if existing.Ref != value.Ref {
		return ports.ErrConflict
	}
	existingBody, _ := encode(existing)
	if string(existingBody) != string(body) {
		return ports.ErrConflict
	}
	return nil
}

func (s *Store) InspectLifecyclePlanV4(ctx context.Context, expected applicationcontract.SandboxLifecyclePlanRefV4) (applicationadapter.LifecyclePlanEnvelopeV4, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM lifecycle_plans WHERE plan_id=?`, expected.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return applicationadapter.LifecyclePlanEnvelopeV4{}, ports.ErrNotFound
		}
		return applicationadapter.LifecyclePlanEnvelopeV4{}, err
	}
	var value applicationadapter.LifecyclePlanEnvelopeV4
	if err := decode(body, &value); err != nil {
		return applicationadapter.LifecyclePlanEnvelopeV4{}, err
	}
	return value, value.ValidateCurrent(expected, s.clock())
}

func (s *Store) CreateLifecycleApplicationResultV4(ctx context.Context, value applicationcontract.SandboxLifecycleResultV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	if err := value.ValidateCurrent(s.clock()); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	body, err := encode(value)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO lifecycle_results(result_id,request_digest,body) VALUES(?,?,?)`, value.ID, value.RequestDigest, body)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	} else if rows == 1 {
		return value, nil
	}
	existing, err := s.InspectLifecycleApplicationResultV4(ctx, value.ID)
	if err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	existingBody, _ := encode(existing)
	if string(existingBody) != string(body) {
		return applicationcontract.SandboxLifecycleResultV4{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectLifecycleApplicationResultV4(ctx context.Context, id string) (applicationcontract.SandboxLifecycleResultV4, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM lifecycle_results WHERE result_id=?`, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return applicationcontract.SandboxLifecycleResultV4{}, ports.ErrNotFound
		}
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	var value applicationcontract.SandboxLifecycleResultV4
	if err := decode(body, &value); err != nil {
		return applicationcontract.SandboxLifecycleResultV4{}, err
	}
	return value, value.ValidateCurrent(s.clock())
}

func (s *Store) CreateDomainResultRuntimeBindingV4(ctx context.Context, value runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	if err := value.Validate(); err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	body, err := encode(value)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO domain_result_runtime_bindings(binding_id,digest,body) VALUES(?,?,?)`, value.ID, value.Digest, body)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	} else if rows == 1 {
		return value, nil
	}
	existing, err := s.InspectDomainResultRuntimeBindingV4(ctx, value.ID)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(existing, value) {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectDomainResultRuntimeBindingV4(ctx context.Context, id string) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM domain_result_runtime_bindings WHERE binding_id=?`, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return runtimeports.OperationSettlementDomainResultFactRefV4{}, ports.ErrNotFound
		}
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	var value runtimeports.OperationSettlementDomainResultFactRefV4
	if err := decode(body, &value); err != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, err
	}
	return value, value.Validate()
}

var _ applicationadapter.LifecyclePlanReaderV4 = (*Store)(nil)
var _ applicationadapter.LifecycleApplicationResultStoreV4 = (*Store)(nil)
var _ runtimeadapter.DomainResultRuntimeBindingStoreV4 = (*Store)(nil)
