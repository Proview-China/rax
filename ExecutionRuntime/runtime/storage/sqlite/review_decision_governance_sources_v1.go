package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	reviewGovernanceSourcePolicyV1    = "review_policy_v2"
	reviewGovernanceSourceAuthorityV1 = "dispatch_authority_v2"
	reviewGovernanceSourceScopeV1     = "execution_scope_current_v2"
)

type reviewGovernanceSourceRefV1 struct {
	Ref      string
	Revision core.Revision
	Digest   core.Digest
}

func loadReviewGovernanceSourceV1[T any](ctx context.Context, source queryRower, kind string, expected reviewGovernanceSourceRefV1, discriminator string) (T, error) {
	var zero T
	if err := contextError(ctx, "Inspect Review governance source history"); err != nil {
		return zero, err
	}
	var factDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT fact_digest,row_digest,canonical_json FROM runtime_review_governance_source_history WHERE kind=? AND source_ref=? AND revision=?`, kind, expected.Ref, expected.Revision).Scan(&factDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review governance source history is absent")
	}
	if err != nil {
		return zero, mapDBError(ctx, err, false)
	}
	if factDigest != string(expected.Digest) {
		return zero, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review governance source exact digest drifted")
	}
	value, err := decodeRow[T](payload, rowDigest, discriminator)
	if err != nil {
		return zero, err
	}
	return cloneStrict(value)
}

func loadReviewGovernanceSourceCurrentRefV1(ctx context.Context, source queryRower, kind, ref string) (reviewGovernanceSourceRefV1, error) {
	if err := contextError(ctx, "Inspect Review governance source current index"); err != nil {
		return reviewGovernanceSourceRefV1{}, err
	}
	var revision uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,fact_digest FROM runtime_review_governance_source_current WHERE kind=? AND source_ref=?`, kind, ref).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return reviewGovernanceSourceRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review governance source current index is absent")
	}
	if err != nil {
		return reviewGovernanceSourceRefV1{}, mapDBError(ctx, err, false)
	}
	return reviewGovernanceSourceRefV1{Ref: ref, Revision: core.Revision(revision), Digest: core.Digest(digest)}, nil
}

func insertReviewGovernanceSourceV1(ctx context.Context, tx *sql.Tx, kind, discriminator string, ref reviewGovernanceSourceRefV1, value any) error {
	payload, err := marshalStrict(value)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow(discriminator, value)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_governance_source_history(kind,source_ref,revision,fact_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, kind, ref.Ref, ref.Revision, string(ref.Digest), string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_governance_source_current(kind,source_ref,revision,fact_digest) VALUES(?,?,?,?)`, kind, ref.Ref, ref.Revision, string(ref.Digest))
	return mapDBError(ctx, err, true)
}

func advanceReviewGovernanceSourceV1(ctx context.Context, tx *sql.Tx, kind, discriminator string, expected, next reviewGovernanceSourceRefV1, value any) error {
	payload, err := marshalStrict(value)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow(discriminator, value)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_governance_source_history(kind,source_ref,revision,fact_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, kind, next.Ref, next.Revision, string(next.Digest), string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_review_governance_source_current SET revision=?,fact_digest=? WHERE kind=? AND source_ref=? AND revision=? AND fact_digest=?`, next.Revision, string(next.Digest), kind, expected.Ref, expected.Revision, string(expected.Digest))
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance source current full-ref CAS lost its precondition")
	}
	return nil
}

func validateReviewPolicySourceV1(f ports.ReviewPolicyFactV2) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || strings.TrimSpace(string(f.RunID)) == "" || strings.TrimSpace(string(f.RiskClass)) == "" || strings.TrimSpace(f.ActorAuthorityRef) == "" || strings.TrimSpace(f.ReviewerAuthorityRef) == "" || strings.TrimSpace(f.PolicyDecisionRef) == "" || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Policy source fact is incomplete")
	}
	if err := f.SubjectDigest.Validate(); err != nil {
		return err
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.CurrentScope.Validate(); err != nil {
		return err
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Policy source fact digest drifted")
	}
	return nil
}

func validateDispatchAuthoritySourceV1(f ports.DispatchAuthorityFactV2) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Dispatch Authority source fact is incomplete")
	}
	if err := f.Digest.Validate(); err != nil {
		return err
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	switch f.State {
	case ports.AuthorityFactActive, ports.AuthorityFactRevoked, ports.AuthorityFactExpired:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Dispatch Authority source state is invalid")
	}
}

func validateExecutionScopeSourceV1(f ports.ExecutionScopeCurrentFactV2) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || f.ExpiresUnixNano <= 0 || f.ProjectionWatermark == 0 || strings.TrimSpace(string(f.ActiveRunID)) == "" || strings.TrimSpace(f.RunState) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Execution Scope source fact is incomplete")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	for _, source := range []ports.GovernanceSourceFactRefV2{f.ActivationSource, f.InstanceSource, f.AuthoritySource, f.BindingSource, f.RunSource} {
		if err := source.Validate(); err != nil {
			return err
		}
	}
	if f.SandboxSource != nil {
		if err := f.SandboxSource.Validate(); err != nil {
			return err
		}
	}
	switch f.State {
	case ports.ExecutionScopeFactActive, ports.ExecutionScopeFactRevoked, ports.ExecutionScopeFactExpired:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Execution Scope source state is invalid")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Execution Scope source fact digest drifted")
	}
	return nil
}

// CreateReviewPolicyFactV2 is an Owner-only create-once mutation. It never
// infers a tenant Policy or supplies code defaults.
func (s *Store) CreateReviewPolicyFactV2(ctx context.Context, fact ports.ReviewPolicyFactV2) (ports.ReviewPolicyFactV2, error) {
	if err := validateReviewPolicySourceV1(fact); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	return createReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourcePolicyV1, "ReviewPolicyFactV2", reviewGovernanceSourceRefV1{fact.Ref, fact.Revision, fact.Digest}, fact)
}

func (s *Store) CompareAndSwapReviewPolicyFactV2(ctx context.Context, expected ports.ReviewPolicyBindingRefV2, next ports.ReviewPolicyFactV2) (ports.ReviewPolicyFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	if err := validateReviewPolicySourceV1(next); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	return casReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourcePolicyV1, "ReviewPolicyFactV2", reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, reviewGovernanceSourceRefV1{next.Ref, next.Revision, next.Digest}, next)
}

// InspectReviewPolicyFactV2 reads immutable source history by the exact
// Owner ref. It is the only recovery path after an indeterminate Owner write.
func (s *Store) InspectReviewPolicyFactV2(ctx context.Context, expected ports.ReviewPolicyBindingRefV2) (ports.ReviewPolicyFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	value, err := loadReviewGovernanceSourceV1[ports.ReviewPolicyFactV2](ctx, s.db, reviewGovernanceSourcePolicyV1, reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, "ReviewPolicyFactV2")
	if err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	if err := validateReviewPolicySourceV1(value); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	if value.Ref != expected.Ref || value.Revision != expected.Revision || value.Digest != expected.Digest {
		return ports.ReviewPolicyFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Policy historical source drifted from exact Ref")
	}
	return cloneStrict(value)
}

func (s *Store) InspectReviewPolicy(ctx context.Context, ref string) (ports.ReviewPolicyFactV2, error) {
	value, current, err := inspectReviewGovernanceSourceCurrentV1[ports.ReviewPolicyFactV2](ctx, s, reviewGovernanceSourcePolicyV1, strings.TrimSpace(ref), "ReviewPolicyFactV2", validateReviewPolicySourceV1)
	if err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	if value.Ref != current.Ref || value.Revision != current.Revision || value.Digest != current.Digest {
		return ports.ReviewPolicyFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Policy source row drifted from current full Ref")
	}
	return value, nil
}

func (s *Store) CreateDispatchAuthorityFactV2(ctx context.Context, fact ports.DispatchAuthorityFactV2) (ports.DispatchAuthorityFactV2, error) {
	if err := validateDispatchAuthoritySourceV1(fact); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	return createReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourceAuthorityV1, "DispatchAuthorityFactV2", reviewGovernanceSourceRefV1{fact.Ref, fact.Revision, fact.Digest}, fact)
}

// CompareAndSwapDispatchAuthorityFactV2 is an Authority Owner-only full-ref
// CAS. The Ref is stable and Revision advances exactly once.
func (s *Store) CompareAndSwapDispatchAuthorityFactV2(ctx context.Context, expected ports.AuthorityBindingRefV2, next ports.DispatchAuthorityFactV2) (ports.DispatchAuthorityFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	if err := validateDispatchAuthoritySourceV1(next); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	return casReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourceAuthorityV1, "DispatchAuthorityFactV2", reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, reviewGovernanceSourceRefV1{next.Ref, next.Revision, next.Digest}, next)
}

// InspectDispatchAuthorityFactV2 reads immutable Authority history exactly.
func (s *Store) InspectDispatchAuthorityFactV2(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	value, err := loadReviewGovernanceSourceV1[ports.DispatchAuthorityFactV2](ctx, s.db, reviewGovernanceSourceAuthorityV1, reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, "DispatchAuthorityFactV2")
	if err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	if err := validateDispatchAuthoritySourceV1(value); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	if value.Ref != expected.Ref || value.Revision != expected.Revision || value.Digest != expected.Digest || value.Scope.AuthorityEpoch != expected.Epoch {
		return ports.DispatchAuthorityFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Dispatch Authority historical source drifted from exact Ref")
	}
	return cloneStrict(value)
}

func (s *Store) InspectDispatchAuthority(ctx context.Context, ref string) (ports.DispatchAuthorityFactV2, error) {
	value, current, err := inspectReviewGovernanceSourceCurrentV1[ports.DispatchAuthorityFactV2](ctx, s, reviewGovernanceSourceAuthorityV1, strings.TrimSpace(ref), "DispatchAuthorityFactV2", validateDispatchAuthoritySourceV1)
	if err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	if value.Ref != current.Ref || value.Revision != current.Revision || value.Digest != current.Digest {
		return ports.DispatchAuthorityFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Dispatch Authority source row drifted from current full Ref")
	}
	return value, nil
}

func (s *Store) CreateExecutionScopeCurrentFactV2(ctx context.Context, fact ports.ExecutionScopeCurrentFactV2) (ports.ExecutionScopeCurrentFactV2, error) {
	if err := validateExecutionScopeSourceV1(fact); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	return createReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourceScopeV1, "ExecutionScopeCurrentFactV2", reviewGovernanceSourceRefV1{fact.Ref, fact.Revision, fact.Digest}, fact)
}

// CompareAndSwapExecutionScopeCurrentFactV2 is a Scope Owner-only full-ref
// CAS. It never synthesizes a scope from Review input.
func (s *Store) CompareAndSwapExecutionScopeCurrentFactV2(ctx context.Context, expected ports.ExecutionScopeBindingRefV2, next ports.ExecutionScopeCurrentFactV2) (ports.ExecutionScopeCurrentFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	if err := validateExecutionScopeSourceV1(next); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	return casReviewGovernanceSourceV1(ctx, s, reviewGovernanceSourceScopeV1, "ExecutionScopeCurrentFactV2", reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, reviewGovernanceSourceRefV1{next.Ref, next.Revision, next.Digest}, next)
}

// InspectExecutionScopeFactV2 reads immutable Scope history exactly.
func (s *Store) InspectExecutionScopeFactV2(ctx context.Context, expected ports.ExecutionScopeBindingRefV2) (ports.ExecutionScopeCurrentFactV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	value, err := loadReviewGovernanceSourceV1[ports.ExecutionScopeCurrentFactV2](ctx, s.db, reviewGovernanceSourceScopeV1, reviewGovernanceSourceRefV1{expected.Ref, expected.Revision, expected.Digest}, "ExecutionScopeCurrentFactV2")
	if err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	if err := validateExecutionScopeSourceV1(value); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	if value.Ref != expected.Ref || value.Revision != expected.Revision || value.Digest != expected.Digest {
		return ports.ExecutionScopeCurrentFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Execution Scope historical source drifted from exact Ref")
	}
	return cloneStrict(value)
}

func (s *Store) InspectCurrentExecutionScope(ctx context.Context, ref string) (ports.ExecutionScopeCurrentFactV2, error) {
	value, current, err := inspectReviewGovernanceSourceCurrentV1[ports.ExecutionScopeCurrentFactV2](ctx, s, reviewGovernanceSourceScopeV1, strings.TrimSpace(ref), "ExecutionScopeCurrentFactV2", validateExecutionScopeSourceV1)
	if err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	if value.Ref != current.Ref || value.Revision != current.Revision || value.Digest != current.Digest {
		return ports.ExecutionScopeCurrentFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Execution Scope source row drifted from current full Ref")
	}
	return value, nil
}

func createReviewGovernanceSourceV1[T any](ctx context.Context, s *Store, kind, discriminator string, ref reviewGovernanceSourceRefV1, value T) (T, error) {
	var zero T
	staged, err := cloneStrict(value)
	if err != nil {
		return zero, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback() }()
	if current, loadErr := loadReviewGovernanceSourceCurrentRefV1(ctx, tx, kind, ref.Ref); loadErr == nil {
		if current != ref {
			return zero, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance source already has another current ref")
		}
		existing, inspectErr := loadReviewGovernanceSourceV1[T](ctx, tx, kind, ref, discriminator)
		if inspectErr != nil {
			return zero, inspectErr
		}
		if !reflect.DeepEqual(existing, staged) {
			return zero, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review governance source create-once Ref binds different content")
		}
		return existing, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return zero, loadErr
	}
	if s.consumeStageFailure() {
		return zero, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review governance source staged failure")
	}
	if err := insertReviewGovernanceSourceV1(ctx, tx, kind, discriminator, ref, staged); err != nil {
		return zero, err
	}
	if err := commit(ctx, tx); err != nil {
		return zero, err
	}
	if s.consumeLostReply() {
		return zero, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review governance source Create reply was lost")
	}
	return cloneStrict(staged)
}

func casReviewGovernanceSourceV1[T any](ctx context.Context, s *Store, kind, discriminator string, expected, next reviewGovernanceSourceRefV1, value T) (T, error) {
	var zero T
	if expected.Ref != next.Ref || next.Revision != expected.Revision+1 {
		return zero, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance source CAS must preserve Ref and advance exactly once")
	}
	staged, err := cloneStrict(value)
	if err != nil {
		return zero, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, inspectErr := loadReviewGovernanceSourceV1[T](ctx, tx, kind, next, discriminator); inspectErr == nil {
		current, currentErr := loadReviewGovernanceSourceCurrentRefV1(ctx, tx, kind, next.Ref)
		if currentErr != nil || current != next {
			return zero, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance source replay is not current")
		}
		if !reflect.DeepEqual(existing, staged) {
			return zero, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review governance source CAS Ref binds different content")
		}
		return existing, nil
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return zero, inspectErr
	}
	current, err := loadReviewGovernanceSourceCurrentRefV1(ctx, tx, kind, expected.Ref)
	if err != nil {
		return zero, err
	}
	if current != expected {
		return zero, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance source CAS current ref drifted")
	}
	if s.consumeStageFailure() {
		return zero, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review governance source staged failure")
	}
	if err := advanceReviewGovernanceSourceV1(ctx, tx, kind, discriminator, expected, next, staged); err != nil {
		return zero, err
	}
	if err := commit(ctx, tx); err != nil {
		return zero, err
	}
	if s.consumeLostReply() {
		return zero, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review governance source CAS reply was lost")
	}
	return cloneStrict(staged)
}

func inspectReviewGovernanceSourceCurrentV1[T any](ctx context.Context, s *Store, kind, ref, discriminator string, validate func(T) error) (T, reviewGovernanceSourceRefV1, error) {
	var zero T
	if err := contextError(ctx, "Inspect Review governance source current"); err != nil {
		return zero, reviewGovernanceSourceRefV1{}, err
	}
	if ref == "" {
		return zero, reviewGovernanceSourceRefV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review governance source Ref is required")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return zero, reviewGovernanceSourceRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadReviewGovernanceSourceCurrentRefV1(ctx, tx, kind, ref)
	if err != nil {
		return zero, reviewGovernanceSourceRefV1{}, err
	}
	value, err := loadReviewGovernanceSourceV1[T](ctx, tx, kind, current, discriminator)
	if err != nil {
		return zero, reviewGovernanceSourceRefV1{}, err
	}
	if err := validate(value); err != nil {
		return zero, reviewGovernanceSourceRefV1{}, err
	}
	clone, err := cloneStrict(value)
	return clone, current, err
}

var _ ports.ReviewPolicyFactReaderV2 = (*Store)(nil)
var _ ports.AuthorityFactReaderV2 = (*Store)(nil)
var _ ports.ExecutionScopeFactReaderV2 = (*Store)(nil)
