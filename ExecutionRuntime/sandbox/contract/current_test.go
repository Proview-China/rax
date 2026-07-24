package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestExactCurrentContractsRejectNonCurrentFacts(t *testing.T) {
	t.Parallel()
	lease := testkit.LeaseFact()
	lease.State = contract.CurrentFactRevoked
	if err := lease.ValidateShape(); err != nil {
		t.Fatalf("revoked historical lease shape: %v", err)
	}
	if err := lease.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("revoked lease fact remained current")
	}
	slot := testkit.Slot()
	slot.State = contract.CurrentFactIndeterminate
	if err := slot.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("indeterminate slot remained current")
	}
	slot = testkit.Slot()
	slot.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano()
	if err := slot.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("slot remained current at its expiry boundary")
	}
}

func TestDomainAttemptFactSeparatesHistoricalShapeFromCurrentUse(t *testing.T) {
	t.Parallel()
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "attempt-current")
	attempt := testkit.Attempt(reservation)
	if err := attempt.ValidateCurrent(testkit.FixedNow); err != nil {
		t.Fatal(err)
	}
	attempt.Meta.ExpiresUnixNano = testkit.FixedNow.Add(time.Hour).UnixNano()
	if err := attempt.ValidateShape(); err != nil {
		t.Fatalf("expired historical attempt shape: %v", err)
	}
	if err := attempt.ValidateCurrent(testkit.FixedNow.Add(time.Hour)); err == nil {
		t.Fatal("attempt remained current at its expiry boundary")
	}
}

func TestDomainAttemptFactRequiresExactIdentityAndBindings(t *testing.T) {
	t.Parallel()
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "attempt-shape")
	valid := testkit.Attempt(reservation)
	for name, mutate := range map[string]func(*contract.DomainAttemptFact){
		"fact id":     func(v *contract.DomainAttemptFact) { v.Meta.ID = "other-attempt" },
		"effect":      func(v *contract.DomainAttemptFact) { v.EffectID = "" },
		"intent":      func(v *contract.DomainAttemptFact) { v.IntentDigest = "bad" },
		"reservation": func(v *contract.DomainAttemptFact) { v.ReservationRef = contract.Ref{} },
		"lease":       func(v *contract.DomainAttemptFact) { v.RuntimeLeaseBindingRef = contract.Ref{} },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := value.ValidateShape(); err == nil {
				t.Fatal("invalid attempt fact was accepted")
			}
		})
	}
}

func TestExactCurrentProviderBindingIsStructuredAndFailClosed(t *testing.T) {
	t.Parallel()
	valid := testkit.ProviderBinding()
	if err := valid.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*contract.ProviderBindingRef){
		"revision":   func(v *contract.ProviderBindingRef) { v.BindingSetRevision = 0 },
		"manifest":   func(v *contract.ProviderBindingRef) { v.ManifestDigest = "bad" },
		"artifact":   func(v *contract.ProviderBindingRef) { v.ArtifactDigest = "bad" },
		"capability": func(v *contract.ProviderBindingRef) { v.Capability = "" },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := value.ValidateShape(); err == nil {
				t.Fatal("invalid provider binding was accepted")
			}
		})
	}
}
