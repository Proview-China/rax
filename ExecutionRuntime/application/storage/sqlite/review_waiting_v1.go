package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const reviewWaitingCoordinationKindV1 = "review_waiting_coordination"

type reviewWaitingSQLiteFaultV1 struct {
	mu                  sync.Mutex
	loseCreate, loseCAS bool
	failCreate, failCAS core.ErrorCategory
}

var reviewWaitingSQLiteFaultsV1 sync.Map

func (s *StoreV1) reviewWaitingFaultV1() *reviewWaitingSQLiteFaultV1 {
	value, _ := reviewWaitingSQLiteFaultsV1.LoadOrStore(s, &reviewWaitingSQLiteFaultV1{})
	return value.(*reviewWaitingSQLiteFaultV1)
}

func (s *StoreV1) LoseNextReviewWaitingCreateReplyV1() {
	f := s.reviewWaitingFaultV1()
	f.mu.Lock()
	f.loseCreate = true
	f.mu.Unlock()
}
func (s *StoreV1) LoseNextReviewWaitingCASReplyV1() {
	f := s.reviewWaitingFaultV1()
	f.mu.Lock()
	f.loseCAS = true
	f.mu.Unlock()
}
func (s *StoreV1) FailNextReviewWaitingCreateBeforeCommitV1(category core.ErrorCategory) {
	f := s.reviewWaitingFaultV1()
	f.mu.Lock()
	f.failCreate = category
	f.mu.Unlock()
}
func (s *StoreV1) FailNextReviewWaitingCASBeforeCommitV1(category core.ErrorCategory) {
	f := s.reviewWaitingFaultV1()
	f.mu.Lock()
	f.failCAS = category
	f.mu.Unlock()
}

func (s *StoreV1) takeReviewWaitingFaultV1(cas, before bool) core.ErrorCategory {
	f := s.reviewWaitingFaultV1()
	f.mu.Lock()
	defer f.mu.Unlock()
	if before {
		if cas {
			value := f.failCAS
			f.failCAS = ""
			return value
		}
		value := f.failCreate
		f.failCreate = ""
		return value
	}
	if cas {
		if f.loseCAS {
			f.loseCAS = false
			return core.ErrorIndeterminate
		}
		return ""
	}
	if f.loseCreate {
		f.loseCreate = false
		return core.ErrorIndeterminate
	}
	return ""
}

func (s *StoreV1) CreateReviewWaitingCoordinationV1(ctx context.Context, value contract.ReviewWaitingCoordinationFactV1) (applicationports.ReviewWaitingCoordinationCreateReceiptV1, error) {
	if err := s.writeReady(ctx); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if err := value.Validate(); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if value.Revision != 1 || value.State != contract.ReviewWaitingStateV1 {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, conflict("Review waiting create requires revision-one waiting_review")
	}
	if err := s.checkClock(value.UpdatedUnixNano, value.ExpiresUnixNano); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if category := s.takeReviewWaitingFaultV1(false, true); category != "" {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, reviewWaitingSQLiteFaultErrorV1(category, "Review waiting create failed before commit")
	}
	payload, rowDigest, err := encodeRowV1(reviewWaitingCoordinationKindV1, value.ID, value.Revision, value.Digest, value.PreviousDigest, value.UpdatedUnixNano, value.ExpiresUnixNano, value)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	defer tx.Rollback()
	current, found, err := readReviewWaitingCurrentTxV1(ctx, tx, value.ID)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if found {
		existing, err := decodeReviewWaitingCoordinationV1(current.payload, value.ID)
		if err != nil {
			return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
		}
		if reflect.DeepEqual(existing, value) {
			return applicationports.ReviewWaitingCoordinationCreateReceiptV1{Fact: existing.Clone(), Created: false}, nil
		}
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, conflict("Review waiting ID already binds different content")
	}
	var historyCount int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, value.ID).Scan(&historyCount); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, mapDBError(ctx, err, false)
	}
	if historyCount != 0 {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, conflict("Review waiting history exists without current index")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_history_v1(fact_type,fact_id,revision,digest,previous_digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?)`, reviewWaitingCoordinationKindV1, value.ID, uint64(value.Revision), string(value.Digest), "", string(rowDigest), payload, value.UpdatedUnixNano, value.ExpiresUnixNano); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_current_v1(fact_type,fact_id,revision,digest,row_digest) VALUES(?,?,?,?,?)`, reviewWaitingCoordinationKindV1, value.ID, uint64(value.Revision), string(value.Digest), string(rowDigest)); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	if s.takeReviewWaitingFaultV1(false, false) != "" {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review waiting create committed but reply was lost")
	}
	return applicationports.ReviewWaitingCoordinationCreateReceiptV1{Fact: value.Clone(), Created: true}, nil
}

func (s *StoreV1) InspectCurrentReviewWaitingCoordinationV1(ctx context.Context, scope core.ExecutionScope, id string) (contract.ReviewWaitingCoordinationFactV1, error) {
	if err := scope.Validate(); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	row, err := s.readCurrent(ctx, reviewWaitingCoordinationKindV1, id)
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	value, err := decodeReviewWaitingCoordinationV1(row.payload, id)
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	if value.Revision != row.revision || value.Digest != row.digest || value.PreviousDigest != row.previous || value.UpdatedUnixNano != row.checked || value.ExpiresUnixNano != row.expires || !sameReviewWaitingSQLiteScopeV1(scope, value.Request.ExecutionScope) {
		return contract.ReviewWaitingCoordinationFactV1{}, corrupt("Review waiting current row coordinates drifted")
	}
	var highest uint64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(revision) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, id).Scan(&highest); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, mapDBError(ctx, err, false)
	}
	if core.Revision(highest) != value.Revision {
		return contract.ReviewWaitingCoordinationFactV1{}, corrupt("Review waiting current index regressed behind history")
	}
	return value.Clone(), nil
}

func (s *StoreV1) InspectHistoricalReviewWaitingCoordinationV1(ctx context.Context, scope core.ExecutionScope, ref contract.ReviewWaitingCoordinationRefV1) (contract.ReviewWaitingCoordinationFactV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	if err := scope.Validate(); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	var row storedRowV1
	var revision uint64
	var digest, previous, rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT revision,digest,previous_digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=? AND revision=? AND digest=?`, reviewWaitingCoordinationKindV1, ref.ID, uint64(ref.Revision), string(ref.Digest)).Scan(&revision, &digest, &previous, &rowDigest, &row.payload, &row.checked, &row.expires)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review waiting historical coordination is absent")
	}
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, mapDBError(ctx, err, false)
	}
	row.revision, row.digest, row.previous, row.rowDigest = core.Revision(revision), core.Digest(digest), core.Digest(previous), core.Digest(rowDigest)
	if validateStoredRowV1(reviewWaitingCoordinationKindV1, ref.ID, row) != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, corrupt("Review waiting historical row digest drifted")
	}
	value, err := decodeReviewWaitingCoordinationV1(row.payload, ref.ID)
	if err != nil || value.RefV1() != ref || value.PreviousDigest != row.previous || value.UpdatedUnixNano != row.checked || value.ExpiresUnixNano != row.expires || !sameReviewWaitingSQLiteScopeV1(scope, value.Request.ExecutionScope) {
		if err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, err
		}
		return contract.ReviewWaitingCoordinationFactV1{}, corrupt("Review waiting historical coordinates drifted")
	}
	return value.Clone(), nil
}

func (s *StoreV1) CompareAndSwapReviewWaitingCoordinationV1(ctx context.Context, request applicationports.ReviewWaitingCoordinationCASRequestV1) (applicationports.ReviewWaitingCoordinationCASReceiptV1, error) {
	if err := s.writeReady(ctx); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if err := s.checkClock(request.Next.UpdatedUnixNano, request.Next.ExpiresUnixNano); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if category := s.takeReviewWaitingFaultV1(true, true); category != "" {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingSQLiteFaultErrorV1(category, "Review waiting CAS failed before commit")
	}
	next := request.Next.Clone()
	payload, rowDigest, err := encodeRowV1(reviewWaitingCoordinationKindV1, next.ID, next.Revision, next.Digest, next.PreviousDigest, next.UpdatedUnixNano, next.ExpiresUnixNano, next)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	defer tx.Rollback()
	currentRow, found, err := readReviewWaitingCurrentTxV1(ctx, tx, next.ID)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if !found {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review waiting coordination is absent")
	}
	current, err := decodeReviewWaitingCoordinationV1(currentRow.payload, next.ID)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	var highest uint64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(revision) FROM application_fact_history_v1 WHERE fact_type=? AND fact_id=?`, reviewWaitingCoordinationKindV1, next.ID).Scan(&highest); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, mapDBError(ctx, err, false)
	}
	if core.Revision(highest) != current.Revision {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, corrupt("Review waiting CAS current index regressed")
	}
	if current.RefV1() == next.RefV1() && reflect.DeepEqual(current, next) {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{Fact: current.Clone(), Applied: false}, nil
	}
	if current.RefV1() != request.Expected || !sameReviewWaitingSQLiteScopeV1(request.Scope, current.Request.ExecutionScope) {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, conflict("Review waiting CAS predecessor changed")
	}
	if err := contract.ValidateReviewWaitingCoordinationTransitionV1(current, next); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_history_v1(fact_type,fact_id,revision,digest,previous_digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?)`, reviewWaitingCoordinationKindV1, next.ID, uint64(next.Revision), string(next.Digest), string(next.PreviousDigest), string(rowDigest), payload, next.UpdatedUnixNano, next.ExpiresUnixNano); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE application_fact_current_v1 SET revision=?,digest=?,row_digest=? WHERE fact_type=? AND fact_id=? AND revision=? AND digest=?`, uint64(next.Revision), string(next.Digest), string(rowDigest), reviewWaitingCoordinationKindV1, next.ID, uint64(request.Expected.Revision), string(request.Expected.Digest))
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, conflict("Review waiting CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	if s.takeReviewWaitingFaultV1(true, false) != "" {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review waiting CAS committed but reply was lost")
	}
	return applicationports.ReviewWaitingCoordinationCASReceiptV1{Fact: next.Clone(), Applied: true}, nil
}

func readReviewWaitingCurrentTxV1(ctx context.Context, tx *sql.Tx, id string) (storedRowV1, bool, error) {
	var row storedRowV1
	var revision uint64
	var digest, previous, rowDigest, currentRowDigest string
	err := tx.QueryRowContext(ctx, `SELECT h.revision,h.digest,h.previous_digest,h.row_digest,c.row_digest,h.payload_json,h.checked_unix_nano,h.expires_unix_nano FROM application_fact_current_v1 c JOIN application_fact_history_v1 h ON h.fact_type=c.fact_type AND h.fact_id=c.fact_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.fact_type=? AND c.fact_id=?`, reviewWaitingCoordinationKindV1, id).Scan(&revision, &digest, &previous, &rowDigest, &currentRowDigest, &row.payload, &row.checked, &row.expires)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, mapDBError(ctx, err, false)
	}
	row.revision, row.digest, row.previous, row.rowDigest = core.Revision(revision), core.Digest(digest), core.Digest(previous), core.Digest(rowDigest)
	if currentRowDigest != rowDigest || validateStoredRowV1(reviewWaitingCoordinationKindV1, id, row) != nil {
		return storedRowV1{}, false, corrupt("Review waiting current row digest drifted")
	}
	return row, true, nil
}

func decodeReviewWaitingCoordinationV1(payload []byte, id string) (contract.ReviewWaitingCoordinationFactV1, error) {
	value, err := strictDecodeV1[contract.ReviewWaitingCoordinationFactV1](payload)
	if err != nil {
		return value, err
	}
	if value.ID != id || value.Validate() != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, corrupt("Review waiting payload drifted")
	}
	return value.Clone(), nil
}

func sameReviewWaitingSQLiteScopeV1(left, right core.ExecutionScope) bool {
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil && left == right
	}
	leftLease, rightLease := *left.SandboxLease, *right.SandboxLease
	left.SandboxLease, right.SandboxLease = nil, nil
	return left == right && leftLease == rightLease
}

func reviewWaitingSQLiteFaultErrorV1(category core.ErrorCategory, message string) error {
	switch category {
	case core.ErrorConflict:
		return core.NewError(category, core.ReasonRevisionConflict, message)
	case core.ErrorUnavailable:
		return core.NewError(category, core.ReasonEvidenceUnavailable, message)
	default:
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
	}
}

var _ applicationports.ReviewWaitingCoordinationFactPortV1 = (*StoreV1)(nil)
