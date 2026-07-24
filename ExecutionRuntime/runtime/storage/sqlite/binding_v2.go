package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func loadBinding(ctx context.Context, source queryRower, id string) (control.BindingFactV2, error) {
	var revision uint64
	var digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT revision,digest,canonical_json FROM runtime_binding_facts WHERE id=?`, id).Scan(&revision, &digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return control.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding Fact does not exist")
	}
	if err != nil {
		return control.BindingFactV2{}, mapDBError(ctx, err, false)
	}
	fact, err := decodeRow[control.BindingFactV2](payload, digest, "BindingFactV2")
	if err != nil {
		return control.BindingFactV2{}, err
	}
	if fact.ID != id || uint64(fact.Revision) != revision || fact.Validate() != nil {
		return control.BindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding Fact row coordinates drifted")
	}
	return fact, nil
}

func loadBindingSet(ctx context.Context, source queryRower, id string) (control.BindingSetFactV2, error) {
	var revision uint64
	var digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT revision,digest,canonical_json FROM runtime_binding_sets WHERE id=?`, id).Scan(&revision, &digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "BindingSet does not exist")
	}
	if err != nil {
		return control.BindingSetFactV2{}, mapDBError(ctx, err, false)
	}
	set, err := decodeRow[control.BindingSetFactV2](payload, digest, "BindingSetFactV2")
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	if set.ID != id || uint64(set.Revision) != revision || set.Validate() != nil {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "BindingSet row coordinates drifted")
	}
	return set, nil
}

func insertBinding(ctx context.Context, tx *sql.Tx, fact control.BindingFactV2) error {
	payload, err := marshalStrict(fact)
	if err != nil {
		return err
	}
	digest, err := digestRow("BindingFactV2", fact)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_binding_facts(id,revision,digest,canonical_json) VALUES(?,?,?,?)`, fact.ID, fact.Revision, string(digest), payload)
	return mapDBError(ctx, err, true)
}

func updateBinding(ctx context.Context, tx *sql.Tx, expected core.Revision, fact control.BindingFactV2) error {
	payload, err := marshalStrict(fact)
	if err != nil {
		return err
	}
	digest, err := digestRow("BindingFactV2", fact)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_binding_facts SET revision=?,digest=?,canonical_json=? WHERE id=? AND revision=?`, fact.Revision, string(digest), payload, fact.ID, expected)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding Fact CAS lost its revision precondition")
	}
	return nil
}

func insertBindingSet(ctx context.Context, tx *sql.Tx, set control.BindingSetFactV2) error {
	payload, err := marshalStrict(set)
	if err != nil {
		return err
	}
	digest, err := digestRow("BindingSetFactV2", set)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_binding_sets(id,revision,digest,canonical_json) VALUES(?,?,?,?)`, set.ID, set.Revision, string(digest), payload)
	return mapDBError(ctx, err, true)
}

func updateBindingSet(ctx context.Context, tx *sql.Tx, expected core.Revision, set control.BindingSetFactV2) error {
	payload, err := marshalStrict(set)
	if err != nil {
		return err
	}
	digest, err := digestRow("BindingSetFactV2", set)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_binding_sets SET revision=?,digest=?,canonical_json=? WHERE id=? AND revision=?`, set.Revision, string(digest), payload, set.ID, expected)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BindingSet CAS lost its revision precondition")
	}
	return nil
}

func (s *Store) CreateBinding(ctx context.Context, fact control.BindingFactV2) (control.BindingFactV2, error) {
	if err := contextError(ctx, "Create Binding"); err != nil {
		return control.BindingFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.BindingFactV2{}, err
	}
	if fact.Revision != 1 || fact.State != control.BindingDeclared {
		return control.BindingFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new Binding Fact must be declared at revision one")
	}
	staged, err := cloneStrict(fact)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if s.consumeStageFailure() {
		return control.BindingFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Binding sqlite staged failure")
	}
	if err := insertBinding(ctx, tx, staged); err != nil {
		return control.BindingFactV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingFactV2{}, err
	}
	if s.consumeLostReply() {
		return control.BindingFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Binding sqlite Create reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectBinding(ctx context.Context, id string) (control.BindingFactV2, error) {
	if err := contextError(ctx, "Inspect Binding"); err != nil {
		return control.BindingFactV2{}, err
	}
	fact, err := loadBinding(ctx, s.db, id)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	return cloneStrict(fact)
}

func (s *Store) CompareAndSwapBinding(ctx context.Context, request control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	now := s.clock()
	if err := request.Validate(now); err != nil {
		return control.BindingFactV2{}, err
	}
	staged, err := cloneStrict(request.Next)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadBinding(ctx, tx, staged.ID)
	if err != nil {
		return control.BindingFactV2{}, err
	}
	if current.Revision != request.ExpectedRevision {
		return control.BindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding Fact revision does not match CAS precondition")
	}
	if err := control.ValidateBindingFactTransitionV2(current, staged, now); err != nil {
		return control.BindingFactV2{}, err
	}
	if s.consumeStageFailure() {
		return control.BindingFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Binding sqlite staged failure")
	}
	if err := updateBinding(ctx, tx, request.ExpectedRevision, staged); err != nil {
		return control.BindingFactV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingFactV2{}, err
	}
	if s.consumeLostReply() {
		return control.BindingFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Binding sqlite CAS reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectBindingSet(ctx context.Context, id string) (control.BindingSetFactV2, error) {
	if err := contextError(ctx, "Inspect BindingSet"); err != nil {
		return control.BindingSetFactV2{}, err
	}
	set, err := loadBindingSet(ctx, s.db, id)
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	return cloneStrict(set)
}

func (s *Store) CommitBindingSet(ctx context.Context, request control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	if err := request.Set.Validate(); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if request.Set.State != control.BindingSetActive || request.Set.Revision != 1 || len(request.Expected) != len(request.Set.Members) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "BindingSet commit requires active revision one and exact member watermarks")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < request.Set.CreatedUnixNano || !now.Before(time.Unix(0, request.Set.ExpiresUnixNano)) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "BindingSet commit clock regressed or reached expiry")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadBindingSet(ctx, tx, request.Set.ID); loadErr == nil {
		if bindingSetCommitReplay(existing, request) {
			return cloneStrict(existing)
		}
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "BindingSet ID already exists with different content")
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return control.BindingSetFactV2{}, loadErr
	}
	expected := make(map[string]core.Revision, len(request.Expected))
	for _, item := range request.Expected {
		if item.BindingID == "" || item.ExpectedRevision == 0 {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "BindingSet expected member watermark is incomplete")
		}
		if _, duplicate := expected[item.BindingID]; duplicate {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BindingSet expected member watermark is duplicated")
		}
		expected[item.BindingID] = item.ExpectedRevision
	}
	nextFacts := make(map[string]control.BindingFactV2, len(request.Set.Members))
	set := request.Set
	for index, member := range set.Members {
		fact, loadErr := loadBinding(ctx, tx, member.BindingID)
		expectedRevision, present := expected[member.BindingID]
		if loadErr != nil || !present || fact.Revision != expectedRevision || member.BindingRevision != expectedRevision || fact.State != control.BindingCertified || fact.ComponentID != member.ComponentID || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest || fact.GovernanceDigest != set.GovernanceDigest || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "BindingSet member is absent, stale, expired, uncertified or drifted")
		}
		next := fact
		next.State, next.Revision, next.BindingSetID = control.BindingBound, fact.Revision+1, set.ID
		if err := control.ValidateBindingFactTransitionV2(fact, next, now); err != nil {
			return control.BindingSetFactV2{}, err
		}
		stagedNext, err := cloneStrict(next)
		if err != nil {
			return control.BindingSetFactV2{}, err
		}
		nextFacts[member.BindingID] = stagedNext
		set.Members[index].BindingRevision = next.Revision
	}
	if err := set.Validate(); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if _, err := cloneStrict(set); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if s.consumeStageFailure() {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected BindingSet sqlite staged failure")
	}
	for id, next := range nextFacts {
		if err := updateBinding(ctx, tx, requestExpectedRevision(request.Expected, id), next); err != nil {
			return control.BindingSetFactV2{}, err
		}
	}
	if err := insertBindingSet(ctx, tx, set); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if s.consumeLostReply() {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected BindingSet sqlite commit reply loss")
	}
	return cloneStrict(set)
}

func (s *Store) CompareAndSwapBindingSet(ctx context.Context, request control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	if request.ExpectedRevision == 0 || request.Next.Revision != request.ExpectedRevision+1 {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "BindingSet CAS requires a consecutive revision")
	}
	staged, err := cloneStrict(request.Next)
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadBindingSet(ctx, tx, staged.ID)
	if err != nil {
		return control.BindingSetFactV2{}, err
	}
	if current.Revision != request.ExpectedRevision {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BindingSet revision does not match CAS precondition")
	}
	if err := control.ValidateBindingSetTransitionV2(current, staged); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if s.consumeStageFailure() {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected BindingSet sqlite staged failure")
	}
	if err := updateBindingSet(ctx, tx, request.ExpectedRevision, staged); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if s.consumeLostReply() {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected BindingSet sqlite CAS reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) RenewBindingSetV2(context.Context, control.RenewBindingSetRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "single-node Binding sqlite cannot atomically include an external renewal Attestation Owner")
}

func requestExpectedRevision(expected []control.ExpectedBindingRevisionV2, id string) core.Revision {
	for _, item := range expected {
		if item.BindingID == id {
			return item.ExpectedRevision
		}
	}
	return 0
}

func bindingSetCommitReplay(existing control.BindingSetFactV2, request control.CommitBindingSetRequestV2) bool {
	if existing.ID != request.Set.ID || existing.PlanID != request.Set.PlanID || existing.PlanDigest != request.Set.PlanDigest || existing.GovernanceDigest != request.Set.GovernanceDigest || existing.State != request.Set.State || existing.CreatedUnixNano != request.Set.CreatedUnixNano || existing.ExpiresUnixNano != request.Set.ExpiresUnixNano || len(existing.Members) != len(request.Set.Members) {
		return false
	}
	for index := range existing.Members {
		left, right := existing.Members[index], request.Set.Members[index]
		if left.BindingRevision != right.BindingRevision+1 {
			return false
		}
		left.BindingRevision = right.BindingRevision
		if !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return reflect.DeepEqual(existing.TopologicalOrder, request.Set.TopologicalOrder) && reflect.DeepEqual(existing.Residuals, request.Set.Residuals)
}
