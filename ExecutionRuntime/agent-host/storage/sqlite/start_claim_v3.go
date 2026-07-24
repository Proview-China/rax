package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

const hostStartClaimInputBindingRowV3 = "HostStartClaimInputBindingV3"

// ClaimOrInspectHostStartV3 makes the version-neutral Claim and its V3 input
// sidecar visible at one SQLite commit. A lost commit reply is recovered only
// through InspectHostStartClaimInputV3 with the exact Claim Ref.
func (s *Store) ClaimOrInspectHostStartV3(ctx context.Context, desired contract.HostStartClaimV1, input contract.HostStartClaimInputV3) (contract.HostStartClaimInputBindingV3, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	binding, err := contract.NewHostStartClaimInputBindingV3(desired, input)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	claimPayload, claimRowDigest, err := encodeRow(hostStartClaimRowV1, desired)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	bindingPayload, bindingRowDigest, err := encodeRow(hostStartClaimInputBindingRowV3, binding)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_start_claims(host_id,start_id,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, desired.HostID, desired.StartID, string(desired.Digest), claimRowDigest, claimPayload); err != nil {
		return contract.HostStartClaimInputBindingV3{}, mapDBError(ctx, err, true)
	}
	actualClaim, err := inspectHostStartClaim(ctx, tx, desired.HostID, desired.StartID)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if !contract.SameHostStartClaimV1(actualClaim, desired) {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact claim")
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_start_claim_input_bindings_v3(host_id,start_id,claim_digest,input_digest,binding_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, desired.HostID, desired.StartID, string(desired.Digest), string(input.ContentDigest), string(binding.BindingDigest), bindingRowDigest, bindingPayload); err != nil {
		return contract.HostStartClaimInputBindingV3{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectHostStartClaimInputV3(ctx, tx, binding.ClaimRef)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if actual.BindingDigest != binding.BindingDigest {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_conflict", "HostStart InputV3 sidecar drifted")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	return actual, nil
}

func (s *Store) InspectHostStartClaimInputV3(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	return inspectHostStartClaimInputV3(ctx, s.db, expected)
}

func inspectHostStartClaimInputV3(ctx context.Context, query queryRowContext, expected contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error) {
	claim, err := inspectHostStartClaim(ctx, query, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if claimRef != expected {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	var payload []byte
	var rowDigest, claimDigest, inputDigest, bindingDigest string
	err = query.QueryRowContext(ctx, `SELECT canonical_json,row_digest,claim_digest,input_digest,binding_digest FROM agent_host_start_claim_input_bindings_v3 WHERE host_id=? AND start_id=?`, expected.HostID, expected.StartID).Scan(&payload, &rowDigest, &claimDigest, &inputDigest, &bindingDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnknownOutcome, "host_start_input_binding_v3_missing", "HostStart Claim exists without its required InputV3 sidecar")
		}
		return contract.HostStartClaimInputBindingV3{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostStartClaimInputBindingV3](payload, rowDigest, hostStartClaimInputBindingRowV3)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if err := value.ValidateV3(); err != nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_row_drift", "HostStart InputV3 sidecar failed validation")
	}
	if value.ClaimRef != expected || claimDigest != string(claim.Digest) || inputDigest != string(value.Input.ContentDigest) || bindingDigest != string(value.BindingDigest) {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_row_drift", "HostStart InputV3 sidecar coordinates drifted")
	}
	expectedClaim, err := value.Input.ClaimV1()
	if err != nil || !contract.SameHostStartClaimV1(expectedClaim, claim) {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_claim_drift", "HostStart InputV3 sidecar no longer matches its Claim")
	}
	return value, nil
}

var _ hostports.HostStartClaimPortV3 = (*Store)(nil)
