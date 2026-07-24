// Package ports contains the narrow owner and consumer seams for immutable
// AgentDefinition facts. Consumers receive read-only exact/current readers;
// repository mutation remains inside the Definition owner.
package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CreateDefinitionRequestV1 struct {
	Definition              contract.AgentDefinitionV1 `json:"definition"`
	ExpectedCurrentRevision core.Revision              `json:"expected_current_revision"`
}

type CreateDefinitionResultV1 struct {
	Definition contract.AgentDefinitionV1   `json:"definition"`
	Current    contract.DefinitionCurrentV1 `json:"current"`
}

type RevokeDefinitionRequestV1 struct {
	DefinitionID            string        `json:"definition_id"`
	ExpectedCurrentRevision core.Revision `json:"expected_current_revision"`
	RevokedUnixNano         int64         `json:"revoked_unix_nano"`
	Reason                  string        `json:"reason"`
}

type DefinitionRepositoryV1 interface {
	CreateDefinitionV1(context.Context, CreateDefinitionRequestV1) (CreateDefinitionResultV1, error)
	InspectDefinitionRevisionV1(context.Context, string, core.Revision) (contract.AgentDefinitionV1, error)
	InspectExactDefinitionV1(context.Context, contract.AgentDefinitionRefV1) (contract.AgentDefinitionV1, error)
	InspectCurrentDefinitionV1(context.Context, string, int64) (contract.DefinitionCurrentV1, error)
	RevokeDefinitionV1(context.Context, RevokeDefinitionRequestV1) (contract.DefinitionCurrentV1, error)
}

type DefinitionCurrentReaderV1 interface {
	InspectExactDefinitionV1(context.Context, contract.AgentDefinitionRefV1) (contract.AgentDefinitionV1, error)
	InspectCurrentDefinitionV1(context.Context, string, int64) (contract.DefinitionCurrentV1, error)
}

type ApprovalCurrentV1 struct {
	Ref             contract.ObjectRefV1 `json:"ref"`
	Approved        bool                 `json:"approved"`
	CheckedUnixNano int64                `json:"checked_unix_nano"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
	Digest          core.Digest          `json:"digest"`
}

func (a ApprovalCurrentV1) Validate(expected contract.ObjectRefV1, nowUnixNano int64) error {
	if err := a.Ref.Validate(); err != nil {
		return err
	}
	if a.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "approval current ref does not match the definition source")
	}
	if !a.Approved {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "definition approval is not approved")
	}
	if nowUnixNano <= 0 || a.CheckedUnixNano <= 0 || a.CheckedUnixNano > nowUnixNano || nowUnixNano >= a.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "definition approval is stale or observed in the future")
	}
	if err := a.Digest.Validate(); err != nil {
		return err
	}
	copy := a
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(contract.DigestDomainV1, contract.DigestVersionV1, "ApprovalCurrentV1", copy)
	if err != nil {
		return err
	}
	if digest != a.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "approval current digest drifted")
	}
	return nil
}

func SealApprovalCurrentV1(value ApprovalCurrentV1) (ApprovalCurrentV1, error) {
	value.Digest = ""
	digest, err := core.CanonicalJSONDigest(contract.DigestDomainV1, contract.DigestVersionV1, "ApprovalCurrentV1", value)
	if err != nil {
		return ApprovalCurrentV1{}, err
	}
	value.Digest = digest
	return value, nil
}

type ApprovalCurrentReaderV1 interface {
	InspectApprovalCurrentV1(context.Context, contract.ObjectRefV1) (ApprovalCurrentV1, error)
}
