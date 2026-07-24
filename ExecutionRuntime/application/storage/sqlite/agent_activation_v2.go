package sqlite

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// V2 deliberately shares coordinationKindV1. The table's (kind,id) unique key
// is the V1/V2 version claim linearization point: either version may win, but
// the losing version cannot create a second aggregate for the same ActivationID.
func (s *StoreV1) CreateAgentActivationCoordinationV2(ctx context.Context, fact contract.AgentActivationCoordinationFactV2) (applicationports.AgentActivationCoordinationCreateReceiptV2, error) {
	if err := fact.Validate(); err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	if fact.Revision != 1 {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, conflict("Agent activation V2 create requires revision one")
	}
	if err := s.checkClock(fact.UpdatedUnixNano, fact.ExpiresUnixNano); err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	payload, err := s.ensure(ctx, coordinationKindV1, fact.ActivationID, fact.Revision, fact.Digest, "", fact.UpdatedUnixNano, fact.ExpiresUnixNano, fact)
	if err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	value, err := decodeAgentActivationCoordinationV2(payload, fact.ActivationID)
	if err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	// Created is conservative because the generic durable primitive intentionally
	// hides whether this call inserted or recovered an exact prior insert.
	return applicationports.AgentActivationCoordinationCreateReceiptV2{Fact: value, Created: false}, nil
}

func (s *StoreV1) InspectAgentActivationCoordinationV2(ctx context.Context, id string) (contract.AgentActivationCoordinationFactV2, error) {
	if strings.TrimSpace(id) == "" {
		return contract.AgentActivationCoordinationFactV2{}, invalid("Agent activation V2 Inspect identity is required")
	}
	row, err := s.readCurrent(ctx, coordinationKindV1, id)
	if err != nil {
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	value, err := decodeAgentActivationCoordinationV2(row.payload, id)
	if err != nil {
		// A well-formed V1 aggregate at the shared key is a version conflict, not
		// a corrupt V2 aggregate.
		if legacy, legacyErr := decodeCoordination(row.payload, coordinationKindV1, id); legacyErr == nil && legacy.Validate() == nil {
			return contract.AgentActivationCoordinationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation ID is claimed by V1")
		}
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	if value.Revision != row.revision || value.Digest != row.digest || value.UpdatedUnixNano != row.checked || value.ExpiresUnixNano != row.expires {
		return contract.AgentActivationCoordinationFactV2{}, corrupt("Agent activation V2 current row coordinates drifted")
	}
	return value, nil
}

func (s *StoreV1) CompareAndSwapAgentActivationCoordinationV2(ctx context.Context, request applicationports.AgentActivationCoordinationCASRequestV2) (applicationports.AgentActivationCoordinationCASReceiptV2, error) {
	if err := request.Validate(); err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	if err := s.checkClock(request.Next.UpdatedUnixNano, request.Next.ExpiresUnixNano); err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	payload, err := s.cas(ctx, coordinationKindV1, request.ActivationID, request.ExpectedRevision, request.ExpectedDigest, request.Next.Revision, request.Next.Digest, request.ExpectedDigest, request.Next.UpdatedUnixNano, request.Next.ExpiresUnixNano, request.Next, func(raw []byte) error {
		current, decodeErr := decodeAgentActivationCoordinationV2(raw, request.ActivationID)
		if decodeErr != nil {
			return decodeErr
		}
		return contract.ValidateAgentActivationCoordinationTransitionV2(current, request.Next)
	})
	if err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	value, err := decodeAgentActivationCoordinationV2(payload, request.ActivationID)
	if err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	return applicationports.AgentActivationCoordinationCASReceiptV2{Fact: value, Applied: true}, nil
}

func decodeAgentActivationCoordinationV2(payload []byte, id string) (contract.AgentActivationCoordinationFactV2, error) {
	value, err := strictDecodeV1[contract.AgentActivationCoordinationFactV2](payload)
	if err != nil {
		return value, err
	}
	if value.ActivationID != id || value.Validate() != nil {
		return contract.AgentActivationCoordinationFactV2{}, corrupt("Agent activation V2 payload drifted")
	}
	return value, nil
}

func (s *StoreV1) LoseNextAgentActivationCoordinationCreateReplyV2() {
	s.LoseNextAgentActivationCoordinationEnsureReplyV1()
}
func (s *StoreV1) LoseNextAgentActivationCoordinationCASReplyV2() {
	s.LoseNextAgentActivationCoordinationCASReplyV1()
}
func (s *StoreV1) FailNextAgentActivationCoordinationCASBeforeCommitV2(category core.ErrorCategory) {
	s.FailNextAgentActivationCoordinationCASBeforeCommitV1(category)
}

var _ applicationports.AgentActivationCoordinationFactPortV2 = (*StoreV1)(nil)
