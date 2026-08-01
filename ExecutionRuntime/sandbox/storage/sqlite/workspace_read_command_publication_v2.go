package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) ApplyWorkspaceReadCommandPublicationV2(
	ctx context.Context,
	capability ownerworkspaceread.AuthorizedCommandPublicationV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error) {
	if ctx == nil || s == nil || s.db == nil || s.clock == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	for attempt := 0; ; attempt++ {
		// Once an initial winner is visible, late contenders must converge by
		// read-only exact closure inspection instead of repeatedly joining the
		// write-lock queue until their short current window expires. This path
		// never manufactures a winner: it validates the complete stored triple
		// against the caller's immutable Command/Publication closure.
		if winner, found, inspectErr := s.inspectInitialWorkspaceReadPublicationWinnerV2(ctx, capability); inspectErr != nil {
			if errors.Is(inspectErr, ports.ErrConflict) {
				return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, inspectErr
			}
		} else if found {
			return winner, false, nil
		}
		current, created, err := s.applyWorkspaceReadCommandPublicationOnceV2(ctx, capability)
		if err == nil {
			return current, created, nil
		}
		if !workspaceReadCommandPublicationSQLiteBusyV2(err) || attempt >= 63 {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false,
				workspaceReadCommandPublicationStorageErrorV2(err)
		}
		// A BUSY/LOCKED result before a write transaction commits proves no
		// winner for this attempt. Retry the same sealed owner capability only;
		// every retry reopens it against a fresh clock and the create-once
		// transaction still converges on the durable winner.
		delay := time.Duration(attempt+1) * time.Millisecond
		if delay > 10*time.Millisecond {
			delay = 10 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Store) inspectInitialWorkspaceReadPublicationWinnerV2(
	ctx context.Context,
	capability ownerworkspaceread.AuthorizedCommandPublicationV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error) {
	now := s.clock()
	if now.IsZero() {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	mutation, command, publication, _, _, err := capability.Open(now)
	if err != nil || mutation != ownerworkspaceread.CommandPublicationInitialV2 {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	storedCommand, storedPublication, current, err := s.InspectStoredWorkspaceReadCommandTripleV2(ctx, command.Meta.Ref())
	if errors.Is(err, ports.ErrNotFound) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, nil
	}
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, fmt.Errorf("inspect initial workspace read publication winner: %w", err)
	}
	if !sameWorkspaceReadCommandStableBodyV2(storedCommand, command) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, fmt.Errorf("%w: initial Command stable body drifted", ports.ErrConflict)
	}
	if !sameWorkspaceReadCommandPublicationStableBodyV2(storedPublication, publication) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, fmt.Errorf("%w: initial Publication stable body drifted", ports.ErrConflict)
	}
	if err = current.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, fmt.Errorf("%w: initial winner is not current: %v", ports.ErrConflict, err)
	}
	return current, true, nil
}

func (s *Store) applyWorkspaceReadCommandPublicationOnceV2(
	ctx context.Context,
	capability ownerworkspaceread.AuthorizedCommandPublicationV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error) {
	entryNow := s.clock()
	if entryNow.IsZero() {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	defer tx.Rollback()
	mutationNow := s.clock()
	if mutationNow.IsZero() || mutationNow.Before(entryNow) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	mutation, command, publication, expected, current, err := capability.Open(mutationNow)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	var stored contract.WorkspaceReadCommandOwnerCurrentV2
	var created bool
	switch mutation {
	case ownerworkspaceread.CommandPublicationInitialV2:
		stored, created, err = applyInitialWorkspaceReadCommandPublicationTxV2(
			ctx, tx, command, publication, current,
		)
		if err != nil {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
		}
	case ownerworkspaceread.CommandPublicationRefreshV2:
		stored, created, err = applyNextWorkspaceReadCommandPublicationTxV2(
			ctx, tx, command, publication, expected, current,
		)
		if err != nil {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
		}
	default:
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	precommitNow := s.clock()
	if precommitNow.IsZero() || precommitNow.Before(mutationNow) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	checkMutation, checkCommand, checkPublication, checkExpected, checkCurrent, checkErr := capability.Open(precommitNow)
	if checkErr != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, checkErr
	}
	if checkMutation != mutation ||
		!reflect.DeepEqual(checkCommand, command) ||
		!reflect.DeepEqual(checkPublication, publication) ||
		!reflect.DeepEqual(checkExpected, expected) ||
		!reflect.DeepEqual(checkCurrent, current) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	if err = commitWorkspaceReadCommandPublicationV2(tx, created); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	return stored, created, nil
}

func workspaceReadCommandPublicationSQLiteBusyV2(err error) bool {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return false
	}
	// SQLite extended result codes keep the primary code in the low byte.
	const (
		sqliteBusyPrimaryV2   = 5
		sqliteLockedPrimaryV2 = 6
	)
	primary := coded.Code() & 0xff
	return primary == sqliteBusyPrimaryV2 || primary == sqliteLockedPrimaryV2
}

type workspaceReadCommandPublicationCommitterV2 interface {
	Commit() error
}

func commitWorkspaceReadCommandPublicationV2(
	committer workspaceReadCommandPublicationCommitterV2,
	created bool,
) error {
	if committer == nil {
		return ports.ErrConflict
	}
	if err := committer.Commit(); err != nil {
		if !created {
			return err
		}
		return fmt.Errorf(
			"%w: commit workspace read Command publication: %v",
			ports.ErrUnknownOutcome,
			err,
		)
	}
	return nil
}

func applyInitialWorkspaceReadCommandPublicationTxV2(
	ctx context.Context,
	tx *sql.Tx,
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error) {
	storedCommand, commandErr := inspectWorkspaceReadCommandExactTxV1(ctx, tx, command.Meta.Ref())
	storedPublication, publicationErr := inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, tx, publication.Meta.Ref())
	pointer, pointerErr := inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(ctx, tx, command.Meta.Ref())
	history, historyErr := inspectStoredWorkspaceReadCommandOwnerHistoryTxV2(
		ctx,
		tx,
		current.Meta.ID,
		command.Meta.Ref(),
	)
	for _, err := range []error{commandErr, publicationErr, pointerErr, historyErr} {
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
		}
	}
	commandFound := commandErr == nil
	publicationFound := publicationErr == nil
	pointerFound := pointerErr == nil
	anyFound := commandFound || publicationFound || pointerFound || len(history) != 0
	if anyFound {
		if !commandFound || !publicationFound || !pointerFound || len(history) == 0 ||
			!sameWorkspaceReadCommandStableBodyV2(storedCommand, command) ||
			!sameWorkspaceReadCommandPublicationStableBodyV2(storedPublication, publication) ||
			validateStoredWorkspaceReadCommandOwnerHistoryV2(
				storedCommand,
				storedPublication,
				history,
				pointer,
			) != nil {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
		}
		for _, historical := range history {
			if historical.Meta.Ref() == current.Meta.Ref() &&
				pointer.Meta.Ref() != current.Meta.Ref() {
				return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
			}
		}
		return pointer, false, nil
	}
	if err := insertWorkspaceReadCommandTxV2(ctx, tx, command); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	if err := insertWorkspaceReadCommandPublicationTxV2(ctx, tx, publication); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	if err := insertWorkspaceReadCommandOwnerCurrentTxV2(ctx, tx, current); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_owner_current_pointer_v2(
			command_id,command_revision,command_digest,
			current_id,current_revision,current_digest,
			publication_id,publication_revision,publication_digest
		) VALUES(?,?,?,?,?,?,?,?,?)`,
		current.Command.ID, current.Command.Revision, current.Command.Digest,
		current.Meta.ID, current.Meta.Revision, current.Meta.Digest,
		current.Publication.ID, current.Publication.Revision, current.Publication.Digest,
	); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, classifyWrite(err)
	}
	return current, true, nil
}

func applyNextWorkspaceReadCommandPublicationTxV2(
	ctx context.Context,
	tx *sql.Tx,
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	expected contract.WorkspaceReadCommandOwnerCurrentV2,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error) {
	storedCommand, err := inspectWorkspaceReadCommandExactTxV1(ctx, tx, command.Meta.Ref())
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	storedPublication, err := inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, tx, publication.Meta.Ref())
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	storedExpected, err := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(
		ctx,
		tx,
		expected.Meta.Ref(),
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	if contract.ValidateWorkspaceReadCommandOwnerClosureV2(
		storedCommand,
		storedPublication,
		storedExpected,
	) != nil ||
		!reflect.DeepEqual(storedExpected, expected) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	resealed, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
		current,
		storedExpected,
		time.Unix(0, current.CheckedUnixNano),
	)
	if err != nil || !reflect.DeepEqual(resealed, current) ||
		contract.ValidateWorkspaceReadCommandOwnerClosureV2(
			storedCommand,
			storedPublication,
			current,
		) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	pointer, err := inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(ctx, tx, command.Meta.Ref())
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	history, err := inspectStoredWorkspaceReadCommandOwnerHistoryTxV2(
		ctx,
		tx,
		current.Meta.ID,
		command.Meta.Ref(),
	)
	if err != nil ||
		validateStoredWorkspaceReadCommandOwnerHistoryV2(
			storedCommand,
			storedPublication,
			history,
			pointer,
		) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	if pointer.Meta.Ref() == current.Meta.Ref() {
		stored, err := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, tx, current.Meta.Ref())
		if err != nil ||
			!reflect.DeepEqual(stored, current) ||
			contract.ValidateWorkspaceReadCommandOwnerClosureV2(storedCommand, storedPublication, stored) != nil {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
		}
		return stored, false, nil
	}
	if pointer.Meta.Ref() != expected.Meta.Ref() {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	if _, err = inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, tx, current.Meta.Ref()); err == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	} else if !errors.Is(err, ports.ErrNotFound) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	if err := insertWorkspaceReadCommandOwnerCurrentTxV2(ctx, tx, current); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE workspace_read_command_owner_current_pointer_v2
		    SET current_id=?,current_revision=?,current_digest=?,
		        publication_id=?,publication_revision=?,publication_digest=?
		  WHERE command_id=? AND command_revision=? AND command_digest=?
		    AND current_id=? AND current_revision=? AND current_digest=?`,
		current.Meta.ID, current.Meta.Revision, current.Meta.Digest,
		current.Publication.ID, current.Publication.Revision, current.Publication.Digest,
		current.Command.ID, current.Command.Revision, current.Command.Digest,
		expected.Meta.ID, expected.Meta.Revision, expected.Meta.Digest,
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, err
	}
	if rows != 1 {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, false, ports.ErrConflict
	}
	return current, true, nil
}

func inspectStoredWorkspaceReadCommandOwnerHistoryTxV2(
	ctx context.Context,
	tx *sql.Tx,
	currentID string,
	command contract.Ref,
) ([]contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT current_id,revision,digest,
		        command_id,command_revision,command_digest
		   FROM workspace_read_command_owner_current_history_v2
		  WHERE current_id=?
		     OR (command_id=? AND command_revision=? AND command_digest=?)
		  ORDER BY revision,digest`,
		currentID,
		command.ID,
		command.Revision,
		command.Digest,
	)
	if err != nil {
		return nil, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	defer rows.Close()
	var refs []contract.Ref
	for rows.Next() {
		var (
			storedCurrentID, digest  string
			revision                 uint64
			commandID, commandDigest string
			commandRevision          uint64
		)
		if err = rows.Scan(
			&storedCurrentID,
			&revision,
			&digest,
			&commandID,
			&commandRevision,
			&commandDigest,
		); err != nil {
			return nil, workspaceReadCommandPublicationStorageErrorV2(err)
		}
		if storedCurrentID != currentID ||
			commandID != command.ID ||
			commandRevision != command.Revision ||
			commandDigest != command.Digest {
			return nil, ports.ErrConflict
		}
		refs = append(refs, contract.Ref{
			ID: storedCurrentID, Revision: revision, Digest: digest,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	if err = rows.Close(); err != nil {
		return nil, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	history := make([]contract.WorkspaceReadCommandOwnerCurrentV2, 0, len(refs))
	for _, ref := range refs {
		current, inspectErr := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, tx, ref)
		if inspectErr != nil {
			return nil, referencedWorkspaceReadCommandStorageErrorV2(inspectErr)
		}
		history = append(history, current)
	}
	return history, nil
}

func validateStoredWorkspaceReadCommandOwnerHistoryV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	history []contract.WorkspaceReadCommandOwnerCurrentV2,
	pointer contract.WorkspaceReadCommandOwnerCurrentV2,
) error {
	if len(history) == 0 {
		return ports.ErrConflict
	}
	for index, current := range history {
		if current.Meta.Revision != uint64(index+1) ||
			contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current) != nil {
			return ports.ErrConflict
		}
		if index == 0 {
			if current.Meta.CreatedUnixNano != current.CheckedUnixNano {
				return ports.ErrConflict
			}
			continue
		}
		expected, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
			current,
			history[index-1],
			time.Unix(0, current.CheckedUnixNano),
		)
		if err != nil || !reflect.DeepEqual(expected, current) {
			return ports.ErrConflict
		}
	}
	if pointer.Meta.Ref() != history[len(history)-1].Meta.Ref() ||
		!reflect.DeepEqual(pointer, history[len(history)-1]) {
		return ports.ErrConflict
	}
	return nil
}

func sameWorkspaceReadCommandStableBodyV2(
	stored contract.WorkspaceReadCommandV1,
	candidate contract.WorkspaceReadCommandV1,
) bool {
	stored.Meta.CreatedUnixNano = 0
	stored.Meta.UpdatedUnixNano = 0
	candidate.Meta.CreatedUnixNano = 0
	candidate.Meta.UpdatedUnixNano = 0
	return reflect.DeepEqual(stored, candidate)
}

func sameWorkspaceReadCommandPublicationStableBodyV2(
	stored contract.WorkspaceReadCommandPublicationV2,
	candidate contract.WorkspaceReadCommandPublicationV2,
) bool {
	stored.Meta.CreatedUnixNano = 0
	stored.Meta.UpdatedUnixNano = 0
	candidate.Meta.CreatedUnixNano = 0
	candidate.Meta.UpdatedUnixNano = 0
	return reflect.DeepEqual(stored, candidate)
}

func insertWorkspaceReadCommandTxV2(
	ctx context.Context,
	tx *sql.Tx,
	command contract.WorkspaceReadCommandV1,
) error {
	body, err := encode(command)
	if err != nil {
		return err
	}
	seal, err := workspaceReadCommandCanonicalBodySealV1(command.Meta.Ref(), body)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_current(command_id,revision,digest,body)
		 VALUES(?,?,?,?)`,
		command.Meta.ID, command.Meta.Revision, command.Meta.Digest, body,
	); err != nil {
		return classifyWrite(err)
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_body_seal(
			command_id,revision,digest,canonical_body_digest
		) VALUES(?,?,?,?)`,
		command.Meta.ID, command.Meta.Revision, command.Meta.Digest, seal,
	); err != nil {
		return classifyWrite(err)
	}
	return nil
}

func insertWorkspaceReadCommandPublicationTxV2(
	ctx context.Context,
	tx *sql.Tx,
	publication contract.WorkspaceReadCommandPublicationV2,
) error {
	body, err := encode(publication)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_publication_v2(
			publication_id,revision,digest,
			command_id,command_revision,command_digest,
			source_id,source_revision,source_digest,
			runtime_attempt_digest,semantic_digest,
			created_unix_nano,expires_unix_nano,body
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		publication.Meta.ID, publication.Meta.Revision, publication.Meta.Digest,
		publication.Command.ID, publication.Command.Revision, publication.Command.Digest,
		publication.Semantic.SourceCommand.ID,
		publication.Semantic.SourceCommand.Revision,
		publication.Semantic.SourceCommand.Digest,
		publication.Semantic.RuntimeAttemptDigest,
		publication.Semantic.Digest,
		publication.Meta.CreatedUnixNano,
		publication.Meta.ExpiresUnixNano,
		body,
	); err != nil {
		return classifyWrite(err)
	}
	return nil
}

func insertWorkspaceReadCommandOwnerCurrentTxV2(
	ctx context.Context,
	tx *sql.Tx,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
) error {
	body, err := encode(current)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_owner_current_history_v2(
			current_id,revision,digest,
			command_id,command_revision,command_digest,
			publication_id,publication_revision,publication_digest,
			checked_unix_nano,expires_unix_nano,body
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		current.Meta.ID, current.Meta.Revision, current.Meta.Digest,
		current.Command.ID, current.Command.Revision, current.Command.Digest,
		current.Publication.ID, current.Publication.Revision, current.Publication.Digest,
		current.CheckedUnixNano, current.ExpiresUnixNano, body,
	); err != nil {
		return classifyWrite(err)
	}
	return nil
}

func (s *Store) InspectStoredWorkspaceReadCommandExactV1(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	if s == nil || s.db == nil || ctx == nil {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	command, err := inspectWorkspaceReadCommandExactTxV1(ctx, s.db, exact)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	return command, nil
}

func (s *Store) InspectStoredWorkspaceReadCommandPublicationExactV2(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandPublicationV2, error) {
	if s == nil || s.db == nil || ctx == nil {
		return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
	}
	return inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, s.db, exact)
}

func inspectStoredWorkspaceReadCommandPublicationExactTxV2(
	ctx context.Context,
	source queryer,
	exact contract.Ref,
) (contract.WorkspaceReadCommandPublicationV2, error) {
	if err := exact.ValidateShape("workspace read Command publication"); err != nil {
		return contract.WorkspaceReadCommandPublicationV2{}, err
	}
	var (
		revision, commandRevision, sourceRevision    uint64
		digest, commandID, commandDigest             string
		sourceID, sourceDigest, runtimeAttemptDigest string
		semanticDigest                               string
		created, expires                             int64
		body                                         []byte
	)
	if err := source.QueryRowContext(
		ctx,
		`SELECT revision,digest,
		        command_id,command_revision,command_digest,
		        source_id,source_revision,source_digest,
		        runtime_attempt_digest,semantic_digest,
		        created_unix_nano,expires_unix_nano,body
		   FROM workspace_read_command_publication_v2
		  WHERE publication_id=? AND revision=?`,
		exact.ID,
		exact.Revision,
	).Scan(
		&revision, &digest,
		&commandID, &commandRevision, &commandDigest,
		&sourceID, &sourceRevision, &sourceDigest,
		&runtimeAttemptDigest, &semanticDigest, &created, &expires, &body,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var siblings int
			if countErr := source.QueryRowContext(
				ctx,
				`SELECT COUNT(*) FROM workspace_read_command_publication_v2
				  WHERE publication_id=?`,
				exact.ID,
			).Scan(&siblings); countErr != nil {
				return contract.WorkspaceReadCommandPublicationV2{}, workspaceReadCommandPublicationStorageErrorV2(countErr)
			}
			if siblings != 0 {
				return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
			}
			return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandPublicationV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
	}
	var siblings int
	if err := source.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM workspace_read_command_publication_v2
		  WHERE publication_id=?`,
		exact.ID,
	).Scan(&siblings); err != nil {
		return contract.WorkspaceReadCommandPublicationV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	if siblings != 1 {
		return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
	}
	var publication contract.WorkspaceReadCommandPublicationV2
	if err := decode(body, &publication); err != nil {
		return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
	}
	canonical, err := encode(publication)
	if err != nil || !bytes.Equal(canonical, body) ||
		publication.ValidateShape() != nil ||
		publication.Meta.Ref() != exact ||
		publication.Command != (contract.Ref{ID: commandID, Revision: commandRevision, Digest: commandDigest}) ||
		publication.Semantic.SourceCommand.ID != sourceID ||
		uint64(publication.Semantic.SourceCommand.Revision) != sourceRevision ||
		publication.Semantic.SourceCommand.Digest != sourceDigest ||
		string(publication.Semantic.RuntimeAttemptDigest) != runtimeAttemptDigest ||
		string(publication.Semantic.Digest) != semanticDigest ||
		publication.Meta.CreatedUnixNano != created ||
		publication.Meta.ExpiresUnixNano != expires {
		return contract.WorkspaceReadCommandPublicationV2{}, ports.ErrConflict
	}
	return publication, nil
}

func (s *Store) InspectStoredWorkspaceReadCommandOwnerCurrentExactV2(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if s == nil || s.db == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	return inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, s.db, exact)
}

func (s *Store) InspectStoredWorkspaceReadCommandOwnerHistoryV2(
	ctx context.Context,
	currentID string,
	command contract.Ref,
) ([]contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if s == nil || s.db == nil || ctx == nil || currentID == "" {
		return nil, ports.ErrConflict
	}
	if err := command.ValidateShape("workspace read Command"); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	defer tx.Rollback()
	history, err := inspectStoredWorkspaceReadCommandOwnerHistoryTxV2(
		ctx,
		tx,
		currentID,
		command,
	)
	if err != nil {
		return nil, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	return history, nil
}

func inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(
	ctx context.Context,
	source queryer,
	exact contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if err := exact.ValidateShape("workspace read Command Owner current"); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	var (
		revision, commandRevision, publicationRevision uint64
		digest, commandID, commandDigest               string
		publicationID, publicationDigest               string
		checked, expires                               int64
		body                                           []byte
	)
	if err := source.QueryRowContext(
		ctx,
		`SELECT revision,digest,
		        command_id,command_revision,command_digest,
		        publication_id,publication_revision,publication_digest,
		        checked_unix_nano,expires_unix_nano,body
		   FROM workspace_read_command_owner_current_history_v2
		  WHERE current_id=? AND revision=?`,
		exact.ID, exact.Revision,
	).Scan(
		&revision, &digest,
		&commandID, &commandRevision, &commandDigest,
		&publicationID, &publicationRevision, &publicationDigest,
		&checked, &expires, &body,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	var current contract.WorkspaceReadCommandOwnerCurrentV2
	if err := decode(body, &current); err != nil {
		return current, ports.ErrConflict
	}
	canonical, err := encode(current)
	if err != nil || !bytes.Equal(canonical, body) ||
		revision != exact.Revision ||
		digest != exact.Digest ||
		current.Meta.Ref() != exact ||
		current.ValidateShape() != nil ||
		current.Command != (contract.Ref{ID: commandID, Revision: commandRevision, Digest: commandDigest}) ||
		current.Publication != (contract.Ref{ID: publicationID, Revision: publicationRevision, Digest: publicationDigest}) ||
		current.CheckedUnixNano != checked ||
		current.ExpiresUnixNano != expires {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	return current, nil
}

func workspaceReadCommandOwnerNamespaceExistsTxV2(
	ctx context.Context,
	source queryer,
	command contract.Ref,
) (bool, error) {
	var rows int
	if err := source.QueryRowContext(
		ctx,
		`SELECT
			(SELECT COUNT(*) FROM workspace_read_command_current WHERE command_id=?) +
			(SELECT COUNT(*) FROM workspace_read_command_body_seal WHERE command_id=?) +
			(SELECT COUNT(*) FROM workspace_read_command_publication_v2 WHERE command_id=?) +
			(SELECT COUNT(*) FROM workspace_read_command_owner_current_history_v2 WHERE command_id=?)`,
		command.ID,
		command.ID,
		command.ID,
		command.ID,
	).Scan(&rows); err != nil {
		return false, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	return rows != 0, nil
}

func (s *Store) InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(
	ctx context.Context,
	command contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if s == nil || s.db == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	defer tx.Rollback()
	return inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(ctx, tx, command)
}

func inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(
	ctx context.Context,
	source queryer,
	command contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if err := command.ValidateShape("workspace read Command"); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	var (
		commandRevision, currentRevision, publicationRevision uint64
		commandDigest, currentID, currentDigest               string
		publicationID, publicationDigest                      string
	)
	if err := source.QueryRowContext(
		ctx,
		`SELECT command_revision,command_digest,
		        current_id,current_revision,current_digest,
		        publication_id,publication_revision,publication_digest
		   FROM workspace_read_command_owner_current_pointer_v2
		  WHERE command_id=?`,
		command.ID,
	).Scan(
		&commandRevision, &commandDigest,
		&currentID, &currentRevision, &currentDigest,
		&publicationID, &publicationRevision, &publicationDigest,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			partial, inspectErr := workspaceReadCommandOwnerNamespaceExistsTxV2(
				ctx,
				source,
				command,
			)
			if inspectErr != nil {
				return contract.WorkspaceReadCommandOwnerCurrentV2{}, inspectErr
			}
			if partial {
				return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
			}
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	if commandRevision != command.Revision || commandDigest != command.Digest {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	current, err := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(
		ctx,
		source,
		contract.Ref{ID: currentID, Revision: currentRevision, Digest: currentDigest},
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	if current.Command != command ||
		current.Publication != (contract.Ref{
			ID: publicationID, Revision: publicationRevision, Digest: publicationDigest,
		}) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	return current, nil
}

func (s *Store) InspectStoredWorkspaceReadCommandTripleV2(
	ctx context.Context,
	command contract.Ref,
) (
	contract.WorkspaceReadCommandV1,
	contract.WorkspaceReadCommandPublicationV2,
	contract.WorkspaceReadCommandOwnerCurrentV2,
	error,
) {
	if s == nil || s.db == nil || ctx == nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, workspaceReadCommandPublicationStorageErrorV2(err)
	}
	defer tx.Rollback()
	current, err := inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(ctx, tx, command)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	storedCommand, err := inspectWorkspaceReadCommandExactTxV1(ctx, tx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	publication, err := inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, tx, current.Publication)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerClosureV2(storedCommand, publication, current); err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandPublicationV2{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, ports.ErrConflict
	}
	return storedCommand, publication, current, nil
}

func referencedWorkspaceReadCommandStorageErrorV2(err error) error {
	if errors.Is(err, ports.ErrNotFound) {
		return ports.ErrConflict
	}
	return workspaceReadCommandPublicationStorageErrorV2(err)
}

func workspaceReadCommandPublicationStorageErrorV2(err error) error {
	if err == nil ||
		errors.Is(err, ports.ErrConflict) ||
		errors.Is(err, ports.ErrNotFound) ||
		errors.Is(err, ports.ErrUnknownOutcome) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var domainErr *runtimecore.DomainError
	if errors.As(err, &domainErr) {
		return err
	}
	return runtimecore.NewError(
		runtimecore.ErrorUnavailable,
		runtimecore.ReasonComponentMissing,
		"workspace read Command publication State Plane is unavailable",
	)
}
