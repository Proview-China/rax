package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const deploymentCurrentRowV2 = "HostDeploymentCurrentV2"

func (s *Store) InspectStoredHostDeploymentExactV2(
	ctx context.Context,
	ref contract.HostDeploymentCurrentRefV2,
) (contract.HostDeploymentCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	return inspectStoredDeploymentExactQueryV2(ctx, s.db, ref)
}

func (s *Store) InspectStoredHostDeploymentCurrentV2(
	ctx context.Context,
	hostID string,
	deploymentID string,
) (contract.HostDeploymentCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err := contract.ValidateIdentifierV1("deployment id", deploymentID); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, false)
	}
	defer tx.Rollback()
	pointer, found, err := inspectDeploymentCurrentPointerTxV2(ctx, tx, hostID, deploymentID)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if !found {
		return contract.HostDeploymentCurrentV2{}, deploymentNotFoundV2()
	}
	if err = validateDeploymentHighestTxV2(ctx, tx, hostID, deploymentID, pointer.Ref.Revision); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	value, err := inspectStoredDeploymentExactQueryV2(ctx, tx, pointer.Ref)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, false)
	}
	now := s.clock()
	if err = value.ValidateCurrentV2(pointer.Ref, now); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	return contract.CloneHostDeploymentCurrentV2(value), nil
}

func (s *Store) CompareAndSwapStoredHostDeploymentCurrentV2(
	ctx context.Context,
	expected contract.HostDeploymentCurrentRefV2,
	next contract.HostDeploymentCurrentV2,
) (contract.HostDeploymentCurrentV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	baseline := s.clock()
	if err := next.ValidateCurrentV2(next.Ref, baseline); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err := validateDeploymentCASShapeV2(expected, next); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	payload, rowDigest, err := encodeRow(deploymentCurrentRowV2, next)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	pointerRowDigest, err := deploymentPointerRowDigestV2(next.Ref)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}

	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	defer tx.Rollback()
	actual, found, err := inspectDeploymentCurrentPointerTxV2(ctx, tx, next.Ref.HostID, next.Ref.DeploymentID)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if found {
		if err = validateDeploymentHighestTxV2(ctx, tx, next.Ref.HostID, next.Ref.DeploymentID, actual.Ref.Revision); err != nil {
			return contract.HostDeploymentCurrentV2{}, err
		}
		existing, inspectErr := inspectStoredDeploymentExactQueryV2(ctx, tx, next.Ref)
		if inspectErr == nil && actual.Ref == next.Ref && reflect.DeepEqual(existing, next) {
			if !expected.IsZero() {
				predecessor, predecessorErr := inspectStoredDeploymentExactQueryV2(ctx, tx, expected)
				if predecessorErr != nil {
					return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 idempotent replay does not bind an exact predecessor")
				}
				if predecessorErr = predecessor.ValidateCurrentV2(expected, baseline); predecessorErr != nil {
					return contract.HostDeploymentCurrentV2{}, predecessorErr
				}
			}
			replayNow := s.clock()
			if replayNow.IsZero() || replayNow.Before(baseline) {
				return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 replay clock regressed")
			}
			if err = next.ValidateCurrentV2(next.Ref, replayNow); err != nil {
				return contract.HostDeploymentCurrentV2{}, err
			}
			if err = s.finishMutation(ctx, tx); err != nil {
				return contract.HostDeploymentCurrentV2{}, err
			}
			return contract.CloneHostDeploymentCurrentV2(existing), nil
		}
		if actual.Ref != expected {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 CAS predecessor changed")
		}
		predecessor, predecessorErr := inspectStoredDeploymentExactQueryV2(ctx, tx, expected)
		if predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 advance predecessor is unavailable")
		}
		if predecessorErr = predecessor.ValidateCurrentV2(expected, baseline); predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, predecessorErr
		}
	} else {
		if !expected.IsZero() {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 expected predecessor is absent")
		}
		var historyCount int
		if err = tx.QueryRowContext(
			ctx,
			`SELECT COUNT(1) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`,
			next.Ref.HostID,
			next.Ref.DeploymentID,
		).Scan(&historyCount); err != nil {
			return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, false)
		}
		if historyCount != 0 {
			return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_orphan_history", "Host deployment current V2 history exists without current pointer")
		}
	}

	actualNow := s.clock()
	if actualNow.IsZero() || actualNow.Before(baseline) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 CAS clock regressed")
	}
	if err = next.ValidateCurrentV2(next.Ref, actualNow); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if !expected.IsZero() {
		predecessor, predecessorErr := inspectStoredDeploymentExactQueryV2(ctx, tx, expected)
		if predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 predecessor disappeared before write")
		}
		if predecessorErr = predecessor.ValidateCurrentV2(expected, actualNow); predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, predecessorErr
		}
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO agent_host_deployment_current_history_v2(
			host_id,deployment_id,revision,digest,bootstrap_digest,
			selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
			checked_unix_nano,expires_unix_nano,row_digest,canonical_json
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		next.Ref.HostID,
		next.Ref.DeploymentID,
		next.Ref.Revision,
		string(next.Ref.Digest),
		string(next.Ref.BootstrapDigest),
		next.Ref.PackageSelectionRef.SelectionID,
		uint64(next.Ref.PackageSelectionRef.Revision),
		string(next.Ref.PackageSelectionRef.Digest),
		next.Ref.PackageSelectionRef.ExpiresUnixNano,
		next.CheckedUnixNano,
		next.ExpiresUnixNano,
		rowDigest,
		payload,
	); err != nil {
		return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, true)
	}
	if !found {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO agent_host_deployment_current_v2(
				host_id,deployment_id,revision,digest,
				selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
				row_digest
			) VALUES(?,?,?,?,?,?,?,?,?)`,
			next.Ref.HostID,
			next.Ref.DeploymentID,
			next.Ref.Revision,
			string(next.Ref.Digest),
			next.Ref.PackageSelectionRef.SelectionID,
			uint64(next.Ref.PackageSelectionRef.Revision),
			string(next.Ref.PackageSelectionRef.Digest),
			next.Ref.PackageSelectionRef.ExpiresUnixNano,
			string(pointerRowDigest),
		); err != nil {
			return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, true)
		}
	} else {
		result, updateErr := tx.ExecContext(
			ctx,
			`UPDATE agent_host_deployment_current_v2
			 SET revision=?,digest=?,
			     selection_id=?,selection_revision=?,selection_digest=?,selection_expires_unix_nano=?,
			     row_digest=?
			 WHERE host_id=? AND deployment_id=? AND revision=? AND digest=? AND row_digest=?`,
			next.Ref.Revision,
			string(next.Ref.Digest),
			next.Ref.PackageSelectionRef.SelectionID,
			uint64(next.Ref.PackageSelectionRef.Revision),
			string(next.Ref.PackageSelectionRef.Digest),
			next.Ref.PackageSelectionRef.ExpiresUnixNano,
			string(pointerRowDigest),
			expected.HostID,
			expected.DeploymentID,
			expected.Revision,
			string(expected.Digest),
			string(actual.RowDigest),
		)
		if updateErr != nil {
			return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, updateErr, true)
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, affectedErr, true)
		}
		if affected != 1 {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 CAS lost")
		}
	}
	commitNow := s.clock()
	if commitNow.IsZero() || commitNow.Before(actualNow) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "Host deployment current V2 commit clock regressed")
	}
	if err = next.ValidateCurrentV2(next.Ref, commitNow); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if !expected.IsZero() {
		predecessor, predecessorErr := inspectStoredDeploymentExactQueryV2(ctx, tx, expected)
		if predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, deploymentConflictV2("Host deployment current V2 predecessor disappeared before commit")
		}
		if predecessorErr = predecessor.ValidateCurrentV2(expected, commitNow); predecessorErr != nil {
			return contract.HostDeploymentCurrentV2{}, predecessorErr
		}
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	return contract.CloneHostDeploymentCurrentV2(next), nil
}

type deploymentCurrentPointerV2 struct {
	Ref       contract.HostDeploymentCurrentRefV2
	RowDigest core.Digest
}

type deploymentQueryV2 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectStoredDeploymentExactQueryV2(
	ctx context.Context,
	query deploymentQueryV2,
	ref contract.HostDeploymentCurrentRefV2,
) (contract.HostDeploymentCurrentV2, error) {
	var payload []byte
	var digest, bootstrapDigest, selectionID, selectionDigest, rowDigest string
	var revision, selectionRevision uint64
	var selectionExpires, checked, expires int64
	err := query.QueryRowContext(
		ctx,
		`SELECT revision,digest,bootstrap_digest,
		        selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
		        checked_unix_nano,expires_unix_nano,row_digest,canonical_json
		   FROM agent_host_deployment_current_history_v2
		  WHERE host_id=? AND deployment_id=? AND revision=? AND digest=?`,
		ref.HostID,
		ref.DeploymentID,
		ref.Revision,
		string(ref.Digest),
	).Scan(
		&revision,
		&digest,
		&bootstrapDigest,
		&selectionID,
		&selectionRevision,
		&selectionDigest,
		&selectionExpires,
		&checked,
		&expires,
		&rowDigest,
		&payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.HostDeploymentCurrentV2{}, deploymentNotFoundV2()
	}
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostDeploymentCurrentV2](payload, rowDigest, deploymentCurrentRowV2)
	if err != nil {
		return contract.HostDeploymentCurrentV2{}, err
	}
	if err = value.ValidateHistoricalV2(); err != nil {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_row_invalid", "Host deployment current V2 SQLite row validation failed")
	}
	if value.Ref != ref ||
		revision != ref.Revision ||
		digest != string(ref.Digest) ||
		bootstrapDigest != string(ref.BootstrapDigest) ||
		selectionID != value.Ref.PackageSelectionRef.SelectionID ||
		selectionRevision != uint64(value.Ref.PackageSelectionRef.Revision) ||
		selectionDigest != string(value.Ref.PackageSelectionRef.Digest) ||
		selectionExpires != value.Ref.PackageSelectionRef.ExpiresUnixNano ||
		checked != value.CheckedUnixNano ||
		expires != value.ExpiresUnixNano {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "host_deployment_v2_row_drift", "Host deployment current V2 SQLite coordinates drifted")
	}
	return contract.CloneHostDeploymentCurrentV2(value), nil
}

func inspectDeploymentCurrentPointerTxV2(
	ctx context.Context,
	tx *sql.Tx,
	hostID string,
	deploymentID string,
) (deploymentCurrentPointerV2, bool, error) {
	var revision, maxRevision uint64
	var digest, rowDigest, bootstrapDigest, selectionID, selectionDigest string
	var selectionRevision uint64
	var expires, selectionExpires int64
	err := tx.QueryRowContext(
		ctx,
		`SELECT c.revision,c.digest,c.row_digest,h.bootstrap_digest,h.expires_unix_nano,
		        c.selection_id,c.selection_revision,c.selection_digest,c.selection_expires_unix_nano,
		        (SELECT MAX(m.revision) FROM agent_host_deployment_current_history_v2 m
		          WHERE m.host_id=c.host_id AND m.deployment_id=c.deployment_id)
		   FROM agent_host_deployment_current_v2 c
		   JOIN agent_host_deployment_current_history_v2 h
		     ON h.host_id=c.host_id AND h.deployment_id=c.deployment_id
		    AND h.revision=c.revision AND h.digest=c.digest
		    AND h.selection_id=c.selection_id
		    AND h.selection_revision=c.selection_revision
		    AND h.selection_digest=c.selection_digest
		    AND h.selection_expires_unix_nano=c.selection_expires_unix_nano
		  WHERE c.host_id=? AND c.deployment_id=?`,
		hostID,
		deploymentID,
	).Scan(
		&revision,
		&digest,
		&rowDigest,
		&bootstrapDigest,
		&expires,
		&selectionID,
		&selectionRevision,
		&selectionDigest,
		&selectionExpires,
		&maxRevision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return deploymentCurrentPointerV2{}, false, nil
	}
	if err != nil {
		return deploymentCurrentPointerV2{}, false, mapDBError(ctx, err, false)
	}
	ref := contract.HostDeploymentCurrentRefV2{
		HostID:          hostID,
		DeploymentID:    deploymentID,
		Revision:        revision,
		BootstrapDigest: contract.DigestV1(bootstrapDigest),
		ExpiresUnixNano: expires,
		Digest:          contract.DigestV1(digest),
	}
	ref.PackageSelectionRef.SelectionID = selectionID
	ref.PackageSelectionRef.Revision = core.Revision(selectionRevision)
	ref.PackageSelectionRef.Digest = core.Digest(selectionDigest)
	ref.PackageSelectionRef.ExpiresUnixNano = selectionExpires
	if err = ref.Validate(); err != nil {
		return deploymentCurrentPointerV2{}, false, contract.NewError(contract.ErrorConflict, "host_deployment_v2_pointer_invalid", "Host deployment current V2 pointer Ref is invalid")
	}
	wantRowDigest, err := deploymentPointerRowDigestV2(ref)
	if err != nil || string(wantRowDigest) != rowDigest {
		return deploymentCurrentPointerV2{}, false, contract.NewError(contract.ErrorConflict, "host_deployment_v2_pointer_digest_drift", "Host deployment current V2 pointer row digest drifted")
	}
	if revision != maxRevision {
		return deploymentCurrentPointerV2{}, false, contract.NewError(contract.ErrorConflict, "host_deployment_v2_current_regressed", "Host deployment current V2 pointer regressed behind history")
	}
	return deploymentCurrentPointerV2{Ref: ref, RowDigest: wantRowDigest}, true, nil
}

func validateDeploymentHighestTxV2(
	ctx context.Context,
	tx *sql.Tx,
	hostID string,
	deploymentID string,
	revision uint64,
) error {
	var highest uint64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT MAX(revision) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`,
		hostID,
		deploymentID,
	).Scan(&highest); err != nil {
		return mapDBError(ctx, err, false)
	}
	if highest != revision {
		return contract.NewError(contract.ErrorConflict, "host_deployment_v2_current_regressed", "Host deployment current V2 pointer regressed behind history")
	}
	return nil
}

func validateDeploymentCASShapeV2(
	expected contract.HostDeploymentCurrentRefV2,
	next contract.HostDeploymentCurrentV2,
) error {
	if expected.IsZero() {
		if next.Ref.Revision != 1 {
			return deploymentConflictV2("Host deployment current V2 create requires revision one")
		}
		return nil
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if next.Ref.HostID != expected.HostID ||
		next.Ref.DeploymentID != expected.DeploymentID ||
		next.Ref.BootstrapDigest != expected.BootstrapDigest ||
		next.Ref.Revision != expected.Revision+1 {
		return deploymentConflictV2("Host deployment current V2 advance requires exact predecessor and revision plus one")
	}
	return nil
}

func deploymentPointerRowDigestV2(ref contract.HostDeploymentCurrentRefV2) (core.Digest, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.agent-host.deployment-current-pointer-sqlite",
		"v2",
		"HostDeploymentCurrentPointerRowV2",
		ref,
	)
}

func deploymentNotFoundV2() error {
	return contract.NewError(contract.ErrorNotFound, "host_deployment_v2_missing", "Host deployment current V2 is absent")
}

func deploymentConflictV2(message string) error {
	return contract.NewError(contract.ErrorConflict, "host_deployment_v2_cas_conflict", message)
}

var _ hostports.HostDeploymentCurrentRepositoryV2 = (*Store)(nil)
