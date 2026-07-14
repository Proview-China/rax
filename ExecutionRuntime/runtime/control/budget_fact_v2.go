package control

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type BudgetBindingModeV2 string

const (
	BudgetReserved             BudgetBindingModeV2 = "reserved"
	BudgetOperationNotRequired BudgetBindingModeV2 = "operation_not_required"
)

type BudgetFactStateV2 string

const (
	BudgetFactActive   BudgetFactStateV2 = "active"
	BudgetFactConsumed BudgetFactStateV2 = "consumed"
	BudgetFactReleased BudgetFactStateV2 = "released"
	BudgetFactExpired  BudgetFactStateV2 = "expired"
	BudgetFactRevoked  BudgetFactStateV2 = "revoked"
)

// BudgetBindingFactV2 stores a Budget-owner decision. Runtime validates and
// binds it but does not calculate limits, prices or allocation policy.
type BudgetBindingFactV2 struct {
	Ref                  string              `json:"ref"`
	IntentID             core.EffectIntentID `json:"intent_id"`
	IntentRevision       core.Revision       `json:"intent_revision"`
	Scope                core.ExecutionScope `json:"scope"`
	Mode                 BudgetBindingModeV2 `json:"mode"`
	PolicyDigest         core.Digest         `json:"policy_digest"`
	PolicyDecisionRef    string              `json:"policy_decision_ref"`
	PolicyEvidenceDigest core.Digest         `json:"policy_evidence_digest"`
	ReservationRef       string              `json:"reservation_ref,omitempty"`
	ReservationRevision  core.Revision       `json:"reservation_revision,omitempty"`
	Limit                uint64              `json:"limit"`
	Consumed             uint64              `json:"consumed"`
	Unit                 string              `json:"unit,omitempty"`
	State                BudgetFactStateV2   `json:"state"`
	Revision             core.Revision       `json:"revision"`
	ExpiresUnixNano      int64               `json:"expires_unix_nano"`
}

type BudgetFactCASRequestV2 struct {
	ExpectedRevision core.Revision       `json:"expected_revision"`
	Next             BudgetBindingFactV2 `json:"next"`
}

type BudgetFactPortV2 interface {
	CreateBudgetBinding(context.Context, BudgetBindingFactV2) (BudgetBindingFactV2, error)
	InspectBudgetBinding(context.Context, string) (BudgetBindingFactV2, error)
	CompareAndSwapBudgetBinding(context.Context, BudgetFactCASRequestV2) (BudgetBindingFactV2, error)
}

func (f BudgetBindingFactV2) Validate() error {
	if strings.TrimSpace(f.Ref) == "" || strings.TrimSpace(string(f.IntentID)) == "" || f.IntentRevision == 0 || f.Revision == 0 || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBudgetBindingMissing, "budget fact ref, intent revision, fact revision and expiry are required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{f.PolicyDigest, f.PolicyEvidenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if strings.TrimSpace(f.PolicyDecisionRef) == "" {
		return core.NewError(core.ErrorForbidden, core.ReasonBudgetBindingMissing, "budget decision requires an explicit policy fact reference")
	}
	if !validBudgetFactStateV2(f.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "budget fact state is unknown")
	}
	switch f.Mode {
	case BudgetReserved:
		if strings.TrimSpace(f.ReservationRef) == "" || f.ReservationRevision == 0 || f.Limit == 0 || strings.TrimSpace(f.Unit) == "" || f.Consumed > f.Limit {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBudgetBindingMissing, "reserved budget requires owner reservation, limit and unit")
		}
	case BudgetOperationNotRequired:
		if f.ReservationRef != "" || f.ReservationRevision != 0 || f.Limit != 0 || f.Consumed != 0 || f.Unit != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingMissing, "not-required budget must be proved only by an explicit policy fact")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBudgetBindingMissing, "budget binding mode is unknown")
	}
	return nil
}

func (f BudgetBindingFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "BudgetBindingFactV2", f)
}

func ValidateBudgetFactTransitionV2(current, next BudgetBindingFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "budget transition time must be injected")
	}
	immutableCurrent := current
	immutableNext := next
	immutableCurrent.State, immutableNext.State = "", ""
	immutableCurrent.Revision, immutableNext.Revision = 0, 0
	immutableCurrent.Consumed, immutableNext.Consumed = 0, 0
	if !reflect.DeepEqual(immutableCurrent, immutableNext) || next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "budget owner decision identity is immutable and revision must advance once")
	}
	valid := false
	switch current.State {
	case BudgetFactActive:
		valid = next.State == BudgetFactConsumed || next.State == BudgetFactReleased || next.State == BudgetFactExpired || next.State == BudgetFactRevoked
	case BudgetFactConsumed:
		valid = next.State == BudgetFactReleased
	case BudgetFactReleased, BudgetFactExpired, BudgetFactRevoked:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "released, expired or revoked budget fact is terminal")
	}
	if !valid {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "budget fact transition is not allowed")
	}
	if next.State == BudgetFactConsumed && current.Mode != BudgetReserved {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "operation-not-required policy cannot be converted into consumption")
	}
	if next.State == BudgetFactExpired && now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "budget expiry cannot be asserted before its boundary")
	}
	return nil
}

func (f BudgetBindingFactV2) BindingRefV2() (ports.BudgetBindingRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return ports.BudgetBindingRefV2{}, err
	}
	return ports.BudgetBindingRefV2{Ref: f.Ref, Digest: digest, Revision: f.Revision, PolicyDigest: f.PolicyDigest}, nil
}

func (f BudgetBindingFactV2) ValidateCurrent(expected ports.BudgetBindingRefV2, intent ports.EffectIntentV2, now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	ref, err := f.BindingRefV2()
	if err != nil {
		return err
	}
	if now.IsZero() || f.State != BudgetFactActive || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || ref != expected || f.IntentID != intent.ID || f.IntentRevision != intent.Revision || !sameBudgetScopeV2(f.Scope, intent.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "budget fact expired or drifted from exact effect revision")
	}
	return nil
}

func sameBudgetScopeV2(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

func validBudgetFactStateV2(value BudgetFactStateV2) bool {
	switch value {
	case BudgetFactActive, BudgetFactConsumed, BudgetFactReleased, BudgetFactExpired, BudgetFactRevoked:
		return true
	default:
		return false
	}
}
