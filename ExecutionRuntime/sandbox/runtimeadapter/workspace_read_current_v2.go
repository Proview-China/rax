package runtimeadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceReadCurrentAdapterV2 struct {
	base         sandboxports.WorkspaceReadCurrentProjectionReaderV1
	reservations sandboxports.WorkspaceReadReservationExactReaderV1
	attempts     sandboxports.WorkspaceReadAttemptCurrentReaderV1
	now          func() time.Time
}

func NewWorkspaceReadCurrentAdapterV2(
	base sandboxports.WorkspaceReadCurrentProjectionReaderV1,
	reservations sandboxports.WorkspaceReadReservationExactReaderV1,
	attempts sandboxports.WorkspaceReadAttemptCurrentReaderV1,
	now func() time.Time,
) (*WorkspaceReadCurrentAdapterV2, error) {
	if base == nil || reservations == nil || attempts == nil || now == nil {
		return nil, errors.New("workspace read current v2 adapter requires every exact Owner reader and a clock")
	}
	return &WorkspaceReadCurrentAdapterV2{
		base: base, reservations: reservations, attempts: attempts, now: now,
	}, nil
}

var _ sandboxports.WorkspaceReadCurrentProjectionReaderV2 = (*WorkspaceReadCurrentAdapterV2)(nil)

type workspaceReadCurrentSnapshotV2 struct {
	base        sandboxports.WorkspaceReadCurrentProjectionV1
	reservation contract.WorkspaceReadReservationV1
	attempt     contract.WorkspaceReadAttemptV1
}

func (a *WorkspaceReadCurrentAdapterV2) InspectWorkspaceReadCurrentV2(ctx context.Context, query sandboxports.WorkspaceReadCurrentQueryV2) (sandboxports.WorkspaceReadCurrentProjectionV2, error) {
	started := a.now()
	if err := query.ValidateCurrent(started); err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	s1, s1Checked, err := a.readSnapshotV2(ctx, query)
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	if s1Checked.Before(started) {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, errors.New("workspace read current v2 clock regressed before S1")
	}
	s2, s2Checked, err := a.readSnapshotV2(ctx, query)
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	if s2Checked.Before(s1Checked) {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, errors.New("workspace read current v2 clock regressed between S1 and S2")
	}
	if !sameWorkspaceReadCurrentSnapshotV2(s1, s2) {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, errors.New("workspace read current v2 Owner closure drifted between S1 and S2")
	}
	ownerClosureDigest, err := contract.Digest("workspace-read-owner-closure-v2", struct {
		BaseSemantic string
		Reservation  contract.Ref
		Attempt      contract.WorkspaceReadAttemptRefV1
		Admission    string
	}{
		BaseSemantic: s2.base.SemanticDigest,
		Reservation:  s2.reservation.Meta.Ref(),
		Attempt:      query.Attempt,
		Admission:    query.AdmissionReceipt.Digest,
	})
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	expires := minimumWorkspaceReadCurrentExpiryV1(
		query.Base.ExpiresUnixNano,
		s2.base.ExpiresUnixNano,
		s2.reservation.Meta.ExpiresUnixNano,
		s2.attempt.Meta.ExpiresUnixNano,
		query.AdmissionReceipt.ExpiresUnixNano,
	)
	projection, err := sandboxports.SealWorkspaceReadCurrentProjectionV2(sandboxports.WorkspaceReadCurrentProjectionV2{
		Base:                      s2.base,
		Reservation:               s2.reservation,
		Attempt:                   s2.attempt,
		AttemptState:              s2.attempt.State,
		AdmissionReceipt:          query.AdmissionReceipt,
		SandboxOwnerClosureDigest: ownerClosureDigest,
		S1CheckedUnixNano:         s1Checked.UnixNano(),
		S2CheckedUnixNano:         s2Checked.UnixNano(),
		ExpiresUnixNano:           expires,
	})
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	if err := projection.ValidateCurrent(s2Checked); err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV2{}, err
	}
	return projection, nil
}

func (a *WorkspaceReadCurrentAdapterV2) readSnapshotV2(ctx context.Context, query sandboxports.WorkspaceReadCurrentQueryV2) (workspaceReadCurrentSnapshotV2, time.Time, error) {
	base, err := a.base.InspectWorkspaceReadCurrentV1(ctx, query.Base)
	if err != nil {
		return workspaceReadCurrentSnapshotV2{}, time.Time{}, err
	}
	now := a.now()
	if now.IsZero() {
		return workspaceReadCurrentSnapshotV2{}, now, errors.New("workspace read current v2 clock is zero")
	}
	if err := query.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	if err := base.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	reservation, err := a.reservations.InspectWorkspaceReadReservationExactV1(ctx, query.Reservation)
	if err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	if err := reservation.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	attempt, err := a.attempts.InspectWorkspaceReadAttemptCurrentV1(ctx, query.Attempt)
	if err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	if err := attempt.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	if err := validateWorkspaceReadCurrentBindingsV2(query, base, reservation, attempt); err != nil {
		return workspaceReadCurrentSnapshotV2{}, now, err
	}
	return workspaceReadCurrentSnapshotV2{base: base, reservation: reservation, attempt: attempt}, now, nil
}

func validateWorkspaceReadCurrentBindingsV2(
	query sandboxports.WorkspaceReadCurrentQueryV2,
	base sandboxports.WorkspaceReadCurrentProjectionV1,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
) error {
	if base.QueryDigest != query.Base.Digest ||
		reservation.Meta.Ref() != query.Reservation ||
		attempt.Meta.Ref() != query.Attempt.OwnerRef() ||
		attempt.State != contract.WorkspaceReadStartedV1 ||
		attempt.Reservation != query.Reservation ||
		reservation.AttemptID != attempt.Meta.ID ||
		reservation.Command != query.Base.Command ||
		reservation.WorkspaceView != query.Base.WorkspaceView ||
		reservation.StableKeyDigest != string(query.Base.StableKeyDigest) ||
		reservation.AuthorizationDigest != string(query.Base.AuthorizationDigest) ||
		attempt.StableKeyDigest != reservation.StableKeyDigest ||
		attempt.RequestDigest != reservation.RequestDigest ||
		attempt.PayloadDigest != reservation.PayloadDigest ||
		attempt.AdmissionReceipt != query.AdmissionReceipt {
		return errors.New("workspace read current v2 exact Owner coordinates drifted")
	}
	return nil
}

func sameWorkspaceReadCurrentSnapshotV2(left, right workspaceReadCurrentSnapshotV2) bool {
	left.base.S1CheckedUnixNano = 0
	left.base.S2CheckedUnixNano = 0
	left.base.RuntimeEnforcementDigest = ""
	left.base.SemanticDigest = ""
	left.base.ProjectionDigest = ""
	right.base.S1CheckedUnixNano = 0
	right.base.S2CheckedUnixNano = 0
	right.base.RuntimeEnforcementDigest = ""
	right.base.SemanticDigest = ""
	right.base.ProjectionDigest = ""
	return reflect.DeepEqual(left, right)
}
