package fakes_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/fakes"
)

func TestContentIntegrityAuditFakeRejectsNilAndTypedNilDelegate(t *testing.T) {
	if fake, err := fakes.NewContentIntegrityAuditGovernanceV1(nil); fake != nil || !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("nil delegate accepted: fake=%v err=%v", fake, err)
	}
	var controller *domain.ContentIntegrityAuditControllerV1
	if fake, err := fakes.NewContentIntegrityAuditGovernanceV1(controller); fake != nil || !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil delegate accepted: fake=%v err=%v", fake, err)
	}
}
