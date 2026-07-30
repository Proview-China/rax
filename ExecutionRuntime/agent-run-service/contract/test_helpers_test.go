package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func digestV1(t *testing.T, label string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(struct {
		Label string `json:"label"`
	}{label})
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func exactRefV1(t *testing.T, kind, id string, revision uint64) contract.ExactRefWireV1 {
	t.Helper()
	return contract.ExactRefWireV1{Kind: kind, ID: id, Revision: contract.NewWireUint64V1(revision), Digest: digestV1(t, kind+"/"+id)}
}

func targetV1(t *testing.T) contract.AgentRunTargetV1 {
	t.Helper()
	target, err := contract.SealAgentRunTargetV1(contract.AgentRunTargetV1{
		HostID:         "host-1",
		StartID:        "start-1",
		HostStartClaim: exactRefV1(t, contract.AgentRunHostStartClaimKindV1, "claim-1", 1),
		ExecutionScope: exactRefV1(t, contract.AgentRunExecutionScopeKindV1, "scope-1", 7),
		RunCurrent:     exactRefV1(t, contract.AgentRunCurrentKindV1, "run-1", 9),
		AuthorityEpoch: contract.NewWireUint64V1(7),
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func cancelCommandV1(t *testing.T, commandID, idempotencyKey, reason string) contract.AgentRunCommandEnvelopeV1 {
	t.Helper()
	target := targetV1(t)
	command, err := contract.SealAgentRunCommandEnvelopeV1(contract.AgentRunCommandEnvelopeV1{
		CommandID:           commandID,
		TraceID:             "trace-1",
		Kind:                contract.AgentRunCommandCancelRunV1,
		Target:              target,
		Actor:               "actor-1",
		AuthorityRef:        exactRefV1(t, "praxis.runtime/authority-current", "authority-1", 7),
		Payload:             contract.AgentRunCommandPayloadV1{Reason: reason},
		ExpectedCurrent:     contract.ExpectedCurrentV1{Ref: target.RunCurrent, Epoch: target.AuthorityEpoch},
		IdempotencyKey:      idempotencyKey,
		SubmittedAtUnixNano: contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:    contract.NewWireUnixNanoV1(200),
	})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func acceptedReceiptV1(t *testing.T, command contract.AgentRunCommandEnvelopeV1) contract.AgentRunCommandReceiptV1 {
	t.Helper()
	commandRef, err := command.CommandRefV1()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef:             commandRef,
		IdempotencyKey:         command.IdempotencyKey,
		CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest:  command.RequestDigest,
		Status:                 contract.AgentRunCommandAcceptedV1,
		TraceID:                command.TraceID,
		RecordedUnixNano:       contract.NewWireUnixNanoV1(120),
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
