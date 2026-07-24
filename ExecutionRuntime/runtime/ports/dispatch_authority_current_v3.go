package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const DispatchAuthorityCurrentContractVersionV3 = "praxis.runtime.dispatch-authority-current/v3"

const dispatchAuthorityCurrentCanonicalDomainV3 = "praxis.runtime.dispatch-authority-current"

// DispatchAuthorityFactV3 is the Runtime Authority Owner's run-bound exact
// current fact. V2 is intentionally unchanged and is never auto-upgraded.
type DispatchAuthorityFactV3 struct {
	ContractVersion   string                `json:"contract_version"`
	Ref               AuthorityBindingRefV2 `json:"ref"`
	Scope             core.ExecutionScope   `json:"scope"`
	RunID             core.AgentRunID       `json:"run_id"`
	ActionScopeDigest core.Digest           `json:"action_scope_digest"`
	State             AuthorityFactStateV2  `json:"state"`
	CheckedUnixNano   int64                 `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                 `json:"expires_unix_nano"`
	FactDigest        core.Digest           `json:"fact_digest"`
}

func (f DispatchAuthorityFactV3) Clone() DispatchAuthorityFactV3 {
	if f.Scope.SandboxLease != nil {
		lease := *f.Scope.SandboxLease
		f.Scope.SandboxLease = &lease
	}
	return f
}

func (f DispatchAuthorityFactV3) Validate() error {
	if f.ContractVersion != DispatchAuthorityCurrentContractVersionV3 || f.Ref.Validate() != nil || strings.TrimSpace(string(f.RunID)) == "" || f.CheckedUnixNano <= 0 || f.CheckedUnixNano >= f.ExpiresUnixNano || f.Ref.Digest != f.FactDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "dispatch authority V3 identity or sealed time is incomplete")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if f.Scope.AuthorityEpoch != f.Ref.Epoch {
		return core.NewError(core.ErrorConflict, core.ReasonStaleAuthorityEpoch, "dispatch authority V3 scope epoch drifted from exact Ref")
	}
	if err := f.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	switch f.State {
	case AuthorityFactActive, AuthorityFactRevoked, AuthorityFactExpired:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "dispatch authority V3 state is invalid")
	}
	digest, err := DigestDispatchAuthorityFactV3(f)
	if err != nil || digest != f.FactDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "dispatch authority V3 fact digest drifted")
	}
	return nil
}

func (f DispatchAuthorityFactV3) ValidateCurrent(expected AuthorityBindingRefV2, expectedScope core.ExecutionScope, expectedRunID core.AgentRunID, expectedActionScopeDigest core.Digest, now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if expected != f.Ref || !SameExecutionScopeV2(expectedScope, f.Scope) || expectedRunID != f.RunID || expectedActionScopeDigest != f.ActionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonStaleAuthorityEpoch, "dispatch authority V3 exact applicability drifted")
	}
	if now.IsZero() || now.UnixNano() < f.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "dispatch authority V3 currentness clock regressed")
	}
	if f.State != AuthorityFactActive || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "dispatch authority V3 is terminal or expired")
	}
	return nil
}

func DigestDispatchAuthorityFactV3(f DispatchAuthorityFactV3) (core.Digest, error) {
	f = f.Clone()
	f.Ref.Digest = ""
	f.FactDigest = ""
	return core.CanonicalJSONDigest(dispatchAuthorityCurrentCanonicalDomainV3, DispatchAuthorityCurrentContractVersionV3, "DispatchAuthorityFactV3", f)
}

func SealDispatchAuthorityFactV3(f DispatchAuthorityFactV3) (DispatchAuthorityFactV3, error) {
	f = f.Clone()
	f.ContractVersion = DispatchAuthorityCurrentContractVersionV3
	f.Ref.Digest = ""
	f.FactDigest = ""
	digest, err := DigestDispatchAuthorityFactV3(f)
	if err != nil {
		return DispatchAuthorityFactV3{}, err
	}
	f.Ref.Digest, f.FactDigest = digest, digest
	return f, f.Validate()
}

func SameDispatchAuthorityStableIdentityV3(left, right DispatchAuthorityFactV3) bool {
	leftScope, rightScope := left.Scope, right.Scope
	leftScope.AuthorityEpoch, rightScope.AuthorityEpoch = 1, 1
	return left.Ref.Ref == right.Ref.Ref && SameExecutionScopeV2(leftScope, rightScope) && left.RunID == right.RunID && left.ActionScopeDigest == right.ActionScopeDigest
}

type DispatchAuthorityFactPublishRequestV3 struct {
	Previous *AuthorityBindingRefV2  `json:"previous,omitempty"`
	Value    DispatchAuthorityFactV3 `json:"value"`
}

func (r DispatchAuthorityFactPublishRequestV3) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		if r.Value.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial dispatch authority V3 revision must be one")
		}
		return nil
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	if r.Value.Ref.Ref != r.Previous.Ref || r.Value.Ref.Revision != r.Previous.Revision+1 || r.Value.Ref.Epoch < r.Previous.Epoch {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 full-ref CAS revision or epoch regressed")
	}
	return nil
}

type DispatchAuthorityFactPublishReceiptV3 struct {
	Ref     AuthorityBindingRefV2 `json:"ref"`
	Created bool                  `json:"created"`
}

// DispatchAuthorityCurrentReaderV3 accepts only a full exact Authority Ref.
// Current Inspect atomically compares the current index with that Ref;
// historical Inspect never consults the current index. Success is a deep clone.
type DispatchAuthorityCurrentReaderV3 interface {
	InspectCurrentDispatchAuthorityV3(context.Context, AuthorityBindingRefV2) (DispatchAuthorityFactV3, error)
	InspectHistoricalDispatchAuthorityV3(context.Context, AuthorityBindingRefV2) (DispatchAuthorityFactV3, error)
}

// DispatchAuthorityCurrentPublisherV3 is Authority-Owner-only. Publication is
// create-once, append-only and advances current by a full-ref revision+1 CAS.
// Lost reply recovery is exact historical Inspect of the same canonical Ref.
type DispatchAuthorityCurrentPublisherV3 interface {
	PublishDispatchAuthorityFactV3(context.Context, DispatchAuthorityFactPublishRequestV3) (DispatchAuthorityFactPublishReceiptV3, error)
}
