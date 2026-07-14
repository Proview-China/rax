package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ApplicationCommandContractVersionV2 = "2.0.0"

// ApplicationCommandKindV2 is intentionally closed legacy command intent.
// Custom modules use namespaced Workflow step kinds, never this enum.
type ApplicationCommandKindV2 string

const (
	ApplicationCommandStartV2         ApplicationCommandKindV2 = "start"
	ApplicationCommandResumeV2        ApplicationCommandKindV2 = "resume"
	ApplicationCommandStopInstanceV2  ApplicationCommandKindV2 = "stop_instance"
	ApplicationCommandFenceV2         ApplicationCommandKindV2 = "fence"
	ApplicationCommandRevokeV2        ApplicationCommandKindV2 = "revoke"
	ApplicationCommandProvideInputV2  ApplicationCommandKindV2 = "provide_input"
	ApplicationCommandCancelRunV2     ApplicationCommandKindV2 = "cancel_run"
	ApplicationCommandApproveEffectV2 ApplicationCommandKindV2 = "approve_effect"
	ApplicationCommandDenyEffectV2    ApplicationCommandKindV2 = "deny_effect"
)

type ApplicationCommandStatusV2 string

const (
	ApplicationCommandAcceptedV2      ApplicationCommandStatusV2 = "accepted"
	ApplicationCommandRejectedV2      ApplicationCommandStatusV2 = "rejected"
	ApplicationCommandExecutingV2     ApplicationCommandStatusV2 = "executing"
	ApplicationCommandCompletedV2     ApplicationCommandStatusV2 = "completed"
	ApplicationCommandSupersededV2    ApplicationCommandStatusV2 = "superseded"
	ApplicationCommandInvalidatedV2   ApplicationCommandStatusV2 = "invalidated"
	ApplicationCommandIndeterminateV2 ApplicationCommandStatusV2 = "indeterminate"
)

type ApplicationCommandEnvelopeV2 struct {
	ID                     string                      `json:"command_id"`
	Kind                   ApplicationCommandKindV2    `json:"command_kind"`
	Target                 core.ExecutionScope         `json:"target"`
	Actor                  string                      `json:"actor"`
	AuthorityRef           string                      `json:"authority_ref"`
	Reason                 string                      `json:"reason"`
	CanonicalPayloadDigest core.Digest                 `json:"canonical_payload_digest"`
	Preconditions          core.ExecutionPreconditions `json:"preconditions"`
	IdempotencyKey         string                      `json:"idempotency_key"`
	EffectIntentID         core.EffectIntentID         `json:"effect_intent_id,omitempty"`
	EffectIntentRevision   core.Revision               `json:"effect_intent_revision,omitempty"`
	SubmittedAt            time.Time                   `json:"submitted_at"`
	ExpiresAt              time.Time                   `json:"expires_at"`
}

func (c ApplicationCommandEnvelopeV2) Validate() error {
	if strings.TrimSpace(c.ID) == "" || !validApplicationCommandKindV2(c.Kind) || strings.TrimSpace(c.Actor) == "" || strings.TrimSpace(c.AuthorityRef) == "" || strings.TrimSpace(c.IdempotencyKey) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command id, kind, actor, authority and idempotency key are required")
	}
	if err := c.Target.Validate(); err != nil {
		return err
	}
	if err := c.CanonicalPayloadDigest.Validate(); err != nil {
		return err
	}
	if err := c.Preconditions.Validate(); err != nil {
		return err
	}
	if c.SubmittedAt.IsZero() || !c.ExpiresAt.After(c.SubmittedAt) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command requires a bounded validity interval")
	}
	if c.Kind == ApplicationCommandApproveEffectV2 || c.Kind == ApplicationCommandDenyEffectV2 {
		if strings.TrimSpace(string(c.EffectIntentID)) == "" || c.EffectIntentRevision == 0 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "effect decision must bind an intent revision")
		}
	}
	return nil
}

type ApplicationCommandRecordV2 struct {
	Envelope   ApplicationCommandEnvelopeV2 `json:"envelope"`
	Revision   core.Revision                `json:"revision"`
	Status     ApplicationCommandStatusV2   `json:"status"`
	RecordedAt time.Time                    `json:"recorded_at"`
}

type DesiredExecutionStateV2 string

const (
	DesiredStoppedV2 DesiredExecutionStateV2 = "stopped"
	DesiredRunningV2 DesiredExecutionStateV2 = "running"
	DesiredFencedV2  DesiredExecutionStateV2 = "fenced"
)

type DesiredStateSnapshotV2 struct {
	Scope         core.ExecutionScope     `json:"scope"`
	Desired       DesiredExecutionStateV2 `json:"desired"`
	Revision      core.Revision           `json:"revision"`
	LastCommandID string                  `json:"last_command_id,omitempty"`
}

type DesiredStateMutationV2 struct {
	Desired DesiredExecutionStateV2 `json:"desired"`
}

func (m DesiredStateMutationV2) ValidateFor(kind ApplicationCommandKindV2) error {
	switch kind {
	case ApplicationCommandStartV2, ApplicationCommandResumeV2:
		if m.Desired == DesiredRunningV2 {
			return nil
		}
	case ApplicationCommandStopInstanceV2:
		if m.Desired == DesiredStoppedV2 {
			return nil
		}
	case ApplicationCommandFenceV2, ApplicationCommandRevokeV2:
		if m.Desired == DesiredFencedV2 {
			return nil
		}
	case ApplicationCommandProvideInputV2, ApplicationCommandCancelRunV2, ApplicationCommandApproveEffectV2, ApplicationCommandDenyEffectV2:
		if ValidDesiredExecutionStateV2(m.Desired) {
			return nil
		}
	}
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "desired state is inconsistent with command kind")
}

type ApplicationCommandIntentV2 struct {
	Envelope ApplicationCommandEnvelopeV2 `json:"envelope"`
	Mutation DesiredStateMutationV2       `json:"mutation"`
}

type ApplicationOutboxRecordV2 struct {
	CommandID     string        `json:"command_id"`
	Revision      core.Revision `json:"revision"`
	PayloadDigest core.Digest   `json:"payload_digest"`
	RecordedAt    time.Time     `json:"recorded_at"`
	Dispatched    bool          `json:"dispatched"`
}

type ApplicationCommandAcceptanceV2 struct {
	Record       ApplicationCommandRecordV2 `json:"record"`
	DesiredState DesiredStateSnapshotV2     `json:"desired_state"`
	Outbox       ApplicationOutboxRecordV2  `json:"outbox"`
}

type ApplicationCommandFactPortV2 interface {
	CreateDesiredState(context.Context, DesiredStateSnapshotV2) (DesiredStateSnapshotV2, error)
	AcceptCommand(context.Context, ApplicationCommandIntentV2) (ApplicationCommandAcceptanceV2, error)
	ReadDesiredState(context.Context, core.ExecutionScope) (DesiredStateSnapshotV2, error)
	ListCommands(context.Context, core.ExecutionScope) ([]ApplicationCommandRecordV2, error)
	ListOutbox(context.Context, core.ExecutionScope) ([]ApplicationOutboxRecordV2, error)
	MarkOutboxDispatched(context.Context, core.ExecutionScope, string, core.Revision) (ApplicationOutboxRecordV2, error)
}

func CommandSupersedesV2(newer, older ApplicationCommandEnvelopeV2) bool {
	if newer.Target.Identity != older.Target.Identity || newer.Target.Lineage != older.Target.Lineage || newer.Target.Instance != older.Target.Instance {
		return false
	}
	switch newer.Kind {
	case ApplicationCommandRevokeV2, ApplicationCommandFenceV2:
		return older.Kind == ApplicationCommandApproveEffectV2 || older.Kind == ApplicationCommandStartV2 || older.Kind == ApplicationCommandResumeV2 || older.Kind == ApplicationCommandProvideInputV2
	case ApplicationCommandStopInstanceV2:
		return older.Kind == ApplicationCommandStartV2 || older.Kind == ApplicationCommandResumeV2 || older.Kind == ApplicationCommandProvideInputV2
	case ApplicationCommandCancelRunV2:
		return older.Kind == ApplicationCommandProvideInputV2
	case ApplicationCommandDenyEffectV2:
		return older.Kind == ApplicationCommandApproveEffectV2 && newer.EffectIntentID == older.EffectIntentID && newer.EffectIntentRevision == older.EffectIntentRevision
	default:
		return false
	}
}

func ValidDesiredExecutionStateV2(value DesiredExecutionStateV2) bool {
	return value == DesiredStoppedV2 || value == DesiredRunningV2 || value == DesiredFencedV2
}

func validApplicationCommandKindV2(kind ApplicationCommandKindV2) bool {
	switch kind {
	case ApplicationCommandStartV2, ApplicationCommandResumeV2, ApplicationCommandStopInstanceV2, ApplicationCommandFenceV2, ApplicationCommandRevokeV2, ApplicationCommandProvideInputV2, ApplicationCommandCancelRunV2, ApplicationCommandApproveEffectV2, ApplicationCommandDenyEffectV2:
		return true
	default:
		return false
	}
}
