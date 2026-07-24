package contract_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCommittedPendingActionSubjectV2HasNoContractVersion(t *testing.T) {
	if _, ok := reflect.TypeOf(contract.CommittedPendingActionSubjectV2{}).FieldByName("ContractVersion"); ok {
		t.Fatal("subject V2 must not carry or infer a ContractVersion")
	}
}

func TestCommittedPendingActionCurrentV2ThreeValidationContracts(t *testing.T) {
	now := time.Unix(1_750_000_010, 0)
	current, request := committedPendingActionCurrentV2Fixture(t, now)
	if err := request.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := current.Validate(now); err != nil {
		t.Fatal(err)
	}
	if err := current.ValidateAgainst(request, now); err != nil {
		t.Fatal(err)
	}

	zeroBound := request.Clone()
	zeroBound.RequestedNotAfterUnixNano = 0
	if err := zeroBound.Validate(now); err != nil {
		t.Fatalf("zero upper bound must mean caller adds no bound: %v", err)
	}
	negative := request.Clone()
	negative.RequestedNotAfterUnixNano = -1
	if err := negative.Validate(now); err == nil {
		t.Fatal("negative upper bound was accepted")
	}
	expiredBound := request.Clone()
	expiredBound.RequestedNotAfterUnixNano = now.UnixNano()
	if err := expiredBound.Validate(now); err == nil {
		t.Fatal("non-current positive upper bound was accepted")
	}

	if err := current.Validate(time.Unix(0, current.CheckedUnixNano-1)); err == nil {
		t.Fatal("clock rollback was accepted")
	}
	if err := current.Validate(time.Unix(0, current.ExpiresUnixNano)); err == nil {
		t.Fatal("expired projection was accepted")
	}
}

func TestCommittedPendingActionCurrentV2ValidateAgainstUsesExactFieldsNotDigestAliases(t *testing.T) {
	now := time.Unix(1_750_000_010, 0)
	current, request := committedPendingActionCurrentV2Fixture(t, now)
	spliced := request.Clone()
	// Preserve the settlement's outer Digest field while changing one nested
	// exact Evidence coordinate. A digest-field-only comparison would miss it.
	spliced.Subject.ModelTurnSettlement.Evidence[0].RecordDigest = testkit.Digest("spliced-evidence-record")
	if err := spliced.Validate(now); err != nil {
		t.Fatalf("splice fixture must remain intrinsically valid: %v", err)
	}
	if err := current.ValidateAgainst(spliced, now); err == nil {
		t.Fatal("nested Settlement splice with unchanged outer Digest was accepted")
	}

	alias := request.Clone()
	alias.Subject.Run.Scope.SandboxLease = &core.SandboxLeaseRef{ID: request.Subject.Run.Scope.SandboxLease.ID, Epoch: request.Subject.Run.Scope.SandboxLease.Epoch + 1}
	alias.Subject.ExecutionScopeDigest, _ = runtimeports.ExecutionScopeDigestV2(alias.Subject.Run.Scope)
	if err := alias.Validate(now); err != nil {
		t.Fatalf("scope alias fixture must remain intrinsically valid: %v", err)
	}
	if err := current.ValidateAgainst(alias, now); err == nil {
		t.Fatal("different full ExecutionScope was accepted")
	}
}

func committedPendingActionCurrentV2Fixture(t *testing.T, now time.Time) (contract.CommittedPendingActionCurrentV2, contract.CommittedPendingActionCurrentRequestV2) {
	t.Helper()
	session := governedSessionV3Steps(t, now.Add(-10*time.Second))[5]
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	subject := contract.CommittedPendingActionSubjectV2{ExecutionScopeDigest: scopeDigest, Run: session.Run, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Turn: session.Turn, PendingActionRef: session.PendingAction.Ref, IdentityRef: session.ApplicationBinding.IdentityRef, DomainResultFactRef: session.ApplicationBinding.DomainResultFactRef, ModelTurnSettlement: session.ApplicationBinding.ModelTurnSettlementRef}
	request := contract.CommittedPendingActionCurrentRequestV2{Subject: subject, RequestedNotAfterUnixNano: now.Add(5 * time.Second).UnixNano()}
	projection := contract.CommittedPendingActionCurrentV2{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, PendingAction: *session.PendingAction, ApplicationBinding: session.ApplicationBinding.Clone(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}
	sealed, err := contract.SealCommittedPendingActionCurrentV2(projection, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return sealed, request
}
