package sqlite

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

const hostStartPackageSelectionBindingRowV1 = "HostStartPackageSelectionBindingV1"

func (store *Store) ClaimOrInspectHostStartPackageSelectionV1(
	ctx context.Context,
	desired contract.HostStartClaimV1,
	input contract.HostStartClaimInputV3,
	desiredBinding contract.HostStartPackageSelectionBindingV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if err := store.writeReady(ctx); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	inputBinding, err := contract.NewHostStartClaimInputBindingV3(desired, input)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if err = desiredBinding.ValidateAgainstClaimInputV1(desired, inputBinding); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	claimPayload, claimRowDigest, err := encodeRow(hostStartClaimRowV1, desired)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	inputPayload, inputRowDigest, err := encodeRow(hostStartClaimInputBindingRowV3, inputBinding)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	bindingPayload, bindingRowDigest, err := encodeRow(hostStartPackageSelectionBindingRowV1, desiredBinding)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	tx, err := store.beginMutation(ctx)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var claimCount int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM agent_host_start_claims WHERE host_id=? AND start_id=?`, desired.HostID, desired.StartID).Scan(&claimCount); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, mapDBError(ctx, err, false)
	}
	if claimCount != 0 {
		actualClaim, inspectErr := inspectHostStartClaim(ctx, tx, desired.HostID, desired.StartID)
		if inspectErr != nil {
			return contract.HostStartPackageSelectionBindingV1{}, inspectErr
		}
		if !contract.SameHostStartClaimV1(actualClaim, desired) {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact Claim")
		}
		actualInput, inspectErr := inspectHostStartClaimInputV3(ctx, tx, desiredBinding.ClaimRef)
		if inspectErr != nil {
			return contract.HostStartPackageSelectionBindingV1{}, inspectErr
		}
		if actualInput.BindingDigest != inputBinding.BindingDigest {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_conflict", "HostStart InputV3 sidecar drifted")
		}
		actualBinding, inspectErr := inspectHostStartPackageSelectionBindingForClaimV1(ctx, tx, desiredBinding.ClaimRef)
		if inspectErr != nil {
			return contract.HostStartPackageSelectionBindingV1{}, inspectErr
		}
		if actualBinding.Ref != desiredBinding.Ref || !reflect.DeepEqual(actualBinding, desiredBinding) {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_conflict", "HostStart Claim is permanently bound to another Package Selection association")
		}
		if err = store.finishMutation(ctx, tx); err != nil {
			return contract.HostStartPackageSelectionBindingV1{}, err
		}
		return actualBinding, nil
	}

	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_start_claims(host_id,start_id,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`,
		desired.HostID, desired.StartID, string(desired.Digest), claimRowDigest, claimPayload); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_start_claim_input_bindings_v3(host_id,start_id,claim_digest,input_digest,binding_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`,
		desired.HostID, desired.StartID, string(desired.Digest), string(input.ContentDigest), string(inputBinding.BindingDigest), inputRowDigest, inputPayload); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, mapDBError(ctx, err, true)
	}
	deployment := desiredBinding.DeploymentCurrentRef
	selection := desiredBinding.PackageSelectionRef
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_start_package_selection_bindings_v1(
		host_id,start_id,claim_digest,claim_input_binding_digest,
		deployment_id,deployment_revision,deployment_digest,deployment_expires_unix_nano,
		selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
		closure_digest,revision,created_unix_nano,expires_unix_nano,binding_digest,row_digest,canonical_json
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		desiredBinding.Ref.HostID,
		desiredBinding.Ref.StartID,
		string(desiredBinding.ClaimRef.Digest),
		string(desiredBinding.ClaimInputBindingDigest),
		deployment.DeploymentID,
		deployment.Revision,
		string(deployment.Digest),
		deployment.ExpiresUnixNano,
		selection.SelectionID,
		uint64(selection.Revision),
		string(selection.Digest),
		selection.ExpiresUnixNano,
		string(desiredBinding.VerifiedPackageClosureDigest),
		desiredBinding.Ref.Revision,
		desiredBinding.CreatedUnixNano,
		desiredBinding.ExpiresUnixNano,
		string(desiredBinding.BindingDigest),
		bindingRowDigest,
		bindingPayload,
	); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectHostStartPackageSelectionBindingForClaimV1(ctx, tx, desiredBinding.ClaimRef)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if actual.Ref != desiredBinding.Ref || !reflect.DeepEqual(actual, desiredBinding) {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_conflict", "HostStart Package Selection association drifted before commit")
	}
	if err = store.finishMutation(ctx, tx); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	return actual, nil
}

func (store *Store) InspectHostStartPackageSelectionBindingV1(
	ctx context.Context,
	expected contract.HostStartPackageSelectionBindingRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if err := store.readReady(ctx); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	actual, err := inspectHostStartPackageSelectionBindingForClaimV1(ctx, store.db, expected.ClaimRef)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if actual.Ref != expected {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_ref_drift", "HostStart Package Selection binding exact Ref drifted")
	}
	return actual, nil
}

func (store *Store) InspectHostStartPackageSelectionBindingForClaimV1(
	ctx context.Context,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if err := store.readReady(ctx); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	return inspectHostStartPackageSelectionBindingForClaimV1(ctx, store.db, expected)
}

func inspectHostStartPackageSelectionBindingForClaimV1(
	ctx context.Context,
	query queryRowContext,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	claim, err := inspectHostStartClaim(ctx, query, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if claimRef != expected {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	input, err := inspectHostStartClaimInputV3(ctx, query, expected)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	var payload []byte
	var (
		claimDigest, inputBindingDigest                       string
		deploymentID, deploymentDigest                        string
		selectionID, selectionDigest, closureDigest           string
		bindingDigest, rowDigest                              string
		deploymentRevision, selectionRevision, revision       uint64
		deploymentExpires, selectionExpires, created, expires int64
	)
	err = query.QueryRowContext(ctx, `SELECT
		claim_digest,claim_input_binding_digest,
		deployment_id,deployment_revision,deployment_digest,deployment_expires_unix_nano,
		selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
		closure_digest,revision,created_unix_nano,expires_unix_nano,binding_digest,row_digest,canonical_json
		FROM agent_host_start_package_selection_bindings_v1 WHERE host_id=? AND start_id=?`,
		expected.HostID, expected.StartID,
	).Scan(
		&claimDigest, &inputBindingDigest,
		&deploymentID, &deploymentRevision, &deploymentDigest, &deploymentExpires,
		&selectionID, &selectionRevision, &selectionDigest, &selectionExpires,
		&closureDigest, &revision, &created, &expires, &bindingDigest, &rowDigest, &payload,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_package_selection_atomic_proof_missing", "HostStart Claim lacks its atomic Package Selection proof")
		}
		return contract.HostStartPackageSelectionBindingV1{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostStartPackageSelectionBindingV1](payload, rowDigest, hostStartPackageSelectionBindingRowV1)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if err = value.ValidateAgainstClaimInputV1(claim, input); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	deployment := value.DeploymentCurrentRef
	selection := value.PackageSelectionRef
	if value.ClaimRef != expected ||
		claimDigest != string(expected.Digest) ||
		inputBindingDigest != string(input.BindingDigest) ||
		deploymentID != deployment.DeploymentID ||
		deploymentRevision != deployment.Revision ||
		deploymentDigest != string(deployment.Digest) ||
		deploymentExpires != deployment.ExpiresUnixNano ||
		selectionID != selection.SelectionID ||
		selectionRevision != uint64(selection.Revision) ||
		selectionDigest != string(selection.Digest) ||
		selectionExpires != selection.ExpiresUnixNano ||
		closureDigest != string(value.VerifiedPackageClosureDigest) ||
		revision != value.Ref.Revision ||
		created != value.CreatedUnixNano ||
		expires != value.ExpiresUnixNano ||
		bindingDigest != string(value.BindingDigest) {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_row_drift", "HostStart Package Selection binding row coordinates drifted")
	}
	return value, nil
}

var _ hostports.HostStartClaimPackageSelectionPortV1 = (*Store)(nil)
