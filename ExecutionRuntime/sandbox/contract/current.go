package contract

import (
	"errors"
	"strings"
	"time"
)

type CurrentFactState string

const (
	CurrentFactActive        CurrentFactState = "active"
	CurrentFactRevoked       CurrentFactState = "revoked"
	CurrentFactIndeterminate CurrentFactState = "indeterminate"
)

func (s CurrentFactState) Validate() error {
	switch s {
	case CurrentFactActive, CurrentFactRevoked, CurrentFactIndeterminate:
		return nil
	default:
		return errors.New("current fact state is invalid")
	}
}

type ProviderBindingRef struct {
	BindingSetID       string `json:"binding_set_id"`
	BindingSetRevision uint64 `json:"binding_set_revision"`
	ComponentID        string `json:"component_id"`
	ManifestDigest     string `json:"manifest_digest"`
	ArtifactDigest     string `json:"artifact_digest"`
	Capability         string `json:"capability"`
}

func (r ProviderBindingRef) ValidateShape() error {
	if strings.TrimSpace(r.BindingSetID) == "" || r.BindingSetRevision == 0 || strings.TrimSpace(r.ComponentID) == "" || strings.TrimSpace(r.Capability) == "" {
		return errors.New("provider binding identity is incomplete")
	}
	if !ValidDigest(r.ManifestDigest) || !ValidDigest(r.ArtifactDigest) {
		return errors.New("provider binding digests are invalid")
	}
	return nil
}

type RuntimeLeaseBindingFact struct {
	Meta    Meta                `json:"meta"`
	State   CurrentFactState    `json:"state"`
	Binding RuntimeLeaseBinding `json:"binding"`
}

// DomainAttemptFact is the Sandbox Owner's exact current attempt fact. It is
// deliberately separate from a reservation so callers must re-read both facts
// before a Provider execution point.
type DomainAttemptFact struct {
	Meta                   Meta             `json:"meta"`
	State                  CurrentFactState `json:"state"`
	OperationID            string           `json:"operation_id"`
	EffectID               string           `json:"effect_id"`
	IntentRevision         uint64           `json:"intent_revision"`
	IntentDigest           string           `json:"intent_digest"`
	AttemptID              string           `json:"attempt_id"`
	ReservationRef         Ref              `json:"reservation_ref"`
	RuntimeLeaseBindingRef Ref              `json:"runtime_lease_binding_ref"`
}

func (f DomainAttemptFact) ValidateShape() error {
	if err := f.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := f.State.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(f.OperationID) == "" || strings.TrimSpace(f.EffectID) == "" || f.IntentRevision == 0 || !ValidDigest(f.IntentDigest) || strings.TrimSpace(f.AttemptID) == "" {
		return errors.New("attempt operation, effect, intent, and identity are required")
	}
	if f.Meta.ID != f.AttemptID {
		return errors.New("attempt fact id does not match attempt identity")
	}
	if err := f.ReservationRef.ValidateShape("attempt reservation ref"); err != nil {
		return err
	}
	return f.RuntimeLeaseBindingRef.ValidateShape("attempt runtime lease binding ref")
}

func (f DomainAttemptFact) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	if f.State != CurrentFactActive {
		return errors.New("domain attempt fact is not active")
	}
	return f.Meta.ValidateCurrent(now)
}

func (f RuntimeLeaseBindingFact) ValidateShape() error {
	if err := f.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := f.State.Validate(); err != nil {
		return err
	}
	return f.Binding.ValidateShape()
}

func (f RuntimeLeaseBindingFact) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	if f.State != CurrentFactActive {
		return errors.New("runtime lease binding fact is not active")
	}
	if err := f.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	return f.Binding.ValidateCurrent(now)
}

type SlotCandidate struct {
	Meta            Meta               `json:"meta"`
	State           CurrentFactState   `json:"state"`
	PlacementRef    Ref                `json:"placement_ref"`
	BackendRef      Ref                `json:"backend_ref"`
	ProviderBinding ProviderBindingRef `json:"provider_binding"`
}

func (s SlotCandidate) ValidateShape() error {
	if err := s.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := s.State.Validate(); err != nil {
		return err
	}
	if err := s.PlacementRef.ValidateShape("slot placement ref"); err != nil {
		return err
	}
	if err := s.BackendRef.ValidateShape("slot backend ref"); err != nil {
		return err
	}
	return s.ProviderBinding.ValidateShape()
}

func (s SlotCandidate) ValidateCurrent(now time.Time) error {
	if err := s.ValidateShape(); err != nil {
		return err
	}
	if s.State != CurrentFactActive {
		return errors.New("slot candidate is not active")
	}
	return s.Meta.ValidateCurrent(now)
}
