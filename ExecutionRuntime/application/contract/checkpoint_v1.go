package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const CheckpointCoordinationContractVersionV1 = "praxis.application/checkpoint-coordination/v1"

type CheckpointExternalExactRefV1 struct {
	ContractVersion string                            `json:"contract_version"`
	ExactSchemaRef  string                            `json:"exact_schema_ref"`
	FactKind        string                            `json:"fact_kind"`
	Schema          runtimeports.SchemaRefV2          `json:"schema"`
	Owner           runtimeports.ProviderBindingRefV2 `json:"owner"`
	TenantID        core.TenantID                     `json:"tenant_id"`
	ScopeDigest     core.Digest                       `json:"scope_digest"`
	RunID           core.AgentRunID                   `json:"run_id"`
	ID              string                            `json:"id"`
	Revision        core.Revision                     `json:"revision"`
	Digest          core.Digest                       `json:"digest"`
}

func (r CheckpointExternalExactRefV1) Validate() error {
	if strings.TrimSpace(r.ContractVersion) == "" || len(r.ContractVersion) > 192 || strings.TrimSpace(r.ExactSchemaRef) == "" || len(r.ExactSchemaRef) > 192 || strings.TrimSpace(r.FactKind) == "" || len(r.FactKind) > 192 || r.Schema.Validate() != nil || r.Owner.Validate() != nil || strings.TrimSpace(string(r.TenantID)) == "" || r.ScopeDigest.Validate() != nil || strings.TrimSpace(string(r.RunID)) == "" || strings.TrimSpace(r.ID) == "" || len(r.ID) > 192 || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint external exact ref is incomplete")
	}
	return nil
}

type CheckpointGateStateV1 string

const (
	CheckpointGateAcquiredV1    CheckpointGateStateV1 = "acquired"
	CheckpointGateBoundV1       CheckpointGateStateV1 = "runtime_bound"
	CheckpointGateReleasedV1    CheckpointGateStateV1 = "released"
	CheckpointGateInvalidatedV1 CheckpointGateStateV1 = "invalidated"
)

type AcquireCheckpointGateRequestV1 struct {
	StableID          string                       `json:"stable_id"`
	IntentDigest      core.Digest                  `json:"intent_digest"`
	Scope             core.ExecutionScope          `json:"scope"`
	RunID             core.AgentRunID              `json:"run_id"`
	Subject           CheckpointExternalExactRefV1 `json:"subject"`
	RequestedNotAfter int64                        `json:"requested_not_after_unix_nano"`
}

func (r AcquireCheckpointGateRequestV1) Validate(now time.Time) error {
	scopeDigest, scopeErr := runtimeports.ExecutionScopeDigestV2(r.Scope)
	if strings.TrimSpace(r.StableID) == "" || len(r.StableID) > 192 || r.IntentDigest.Validate() != nil || r.Scope.Validate() != nil || strings.TrimSpace(string(r.RunID)) == "" || r.Subject.Validate() != nil || r.RequestedNotAfter <= 0 || now.IsZero() || !now.Before(time.Unix(0, r.RequestedNotAfter)) || scopeErr != nil || scopeDigest != r.Subject.ScopeDigest || r.Scope.Identity.TenantID != r.Subject.TenantID || r.RunID != r.Subject.RunID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Application checkpoint Gate request is invalid or stale")
	}
	return nil
}

type CheckpointGateCommitV1 struct {
	ContractVersion  string                                    `json:"contract_version"`
	State            CheckpointGateStateV1                     `json:"state"`
	IntentDigest     core.Digest                               `json:"intent_digest"`
	Subject          CheckpointExternalExactRefV1              `json:"subject"`
	Gate             CheckpointExternalExactRefV1              `json:"gate"`
	Snapshot         CheckpointExternalExactRefV1              `json:"snapshot"`
	RuntimeAttempt   *runtimeports.CheckpointAttemptRefV2      `json:"runtime_attempt,omitempty"`
	RuntimeBarrier   *runtimeports.CheckpointBarrierLeaseRefV2 `json:"runtime_barrier,omitempty"`
	RuntimeEffectCut *runtimeports.EffectCutRefV2              `json:"runtime_effect_cut,omitempty"`
	CheckedUnixNano  int64                                     `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                     `json:"expires_unix_nano"`
	Digest           core.Digest                               `json:"digest"`
}

func (c CheckpointGateCommitV1) Clone() CheckpointGateCommitV1 {
	if c.RuntimeAttempt != nil {
		value := *c.RuntimeAttempt
		c.RuntimeAttempt = &value
	}
	if c.RuntimeBarrier != nil {
		value := *c.RuntimeBarrier
		c.RuntimeBarrier = &value
	}
	if c.RuntimeEffectCut != nil {
		value := *c.RuntimeEffectCut
		c.RuntimeEffectCut = &value
	}
	return c
}

func (c CheckpointGateCommitV1) DigestV1() (core.Digest, error) {
	copy := c.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.checkpoint-gate", CheckpointCoordinationContractVersionV1, "CheckpointGateCommitV1", copy)
}

func (c CheckpointGateCommitV1) Validate(now time.Time) error {
	if c.ContractVersion != CheckpointCoordinationContractVersionV1 || c.IntentDigest.Validate() != nil || c.Subject.Validate() != nil || c.Gate.Validate() != nil || c.Snapshot.Validate() != nil || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= 0 || now.IsZero() || now.UnixNano() < c.CheckedUnixNano || c.Digest.Validate() != nil || c.Subject.TenantID != c.Gate.TenantID || c.Subject.TenantID != c.Snapshot.TenantID || c.Subject.ScopeDigest != c.Gate.ScopeDigest || c.Subject.ScopeDigest != c.Snapshot.ScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "Application checkpoint Gate commit is invalid or stale")
	}
	switch c.State {
	case CheckpointGateAcquiredV1:
		if c.ExpiresUnixNano <= c.CheckedUnixNano || !now.Before(time.Unix(0, c.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "acquired checkpoint Gate is stale")
		}
		if c.RuntimeAttempt != nil || c.RuntimeBarrier != nil || c.RuntimeEffectCut != nil {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "acquired checkpoint Gate cannot carry Runtime refs")
		}
	case CheckpointGateBoundV1, CheckpointGateReleasedV1:
		if c.RuntimeAttempt == nil || c.RuntimeBarrier == nil || c.RuntimeEffectCut == nil || c.RuntimeAttempt.Validate() != nil || c.RuntimeBarrier.Validate() != nil || c.RuntimeEffectCut.Validate() != nil || c.RuntimeAttempt.ID != c.RuntimeBarrier.AttemptID || c.RuntimeAttempt.ID != c.RuntimeEffectCut.Attempt.ID || c.RuntimeAttempt.TenantID != c.Subject.TenantID {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Gate Runtime binding is incomplete")
		}
		if c.State == CheckpointGateBoundV1 && (c.ExpiresUnixNano <= c.CheckedUnixNano || !now.Before(time.Unix(0, c.ExpiresUnixNano))) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Runtime-bound checkpoint Gate is stale")
		}
	case CheckpointGateInvalidatedV1:
		if c.RuntimeAttempt != nil || c.RuntimeBarrier != nil || c.RuntimeEffectCut != nil {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "invalidated checkpoint Gate cannot carry Runtime refs")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "checkpoint Gate state is invalid")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Application checkpoint Gate commit drifted")
	}
	return nil
}

func SealCheckpointGateCommitV1(c CheckpointGateCommitV1, now time.Time) (CheckpointGateCommitV1, error) {
	c = c.Clone()
	c.ContractVersion = CheckpointCoordinationContractVersionV1
	c.Digest = ""
	digest, err := c.DigestV1()
	if err != nil {
		return CheckpointGateCommitV1{}, err
	}
	c.Digest = digest
	return c.Clone(), c.Validate(now)
}

type BindCheckpointGateRuntimeRequestV1 struct {
	Gate      CheckpointGateCommitV1                   `json:"gate"`
	Attempt   runtimeports.CheckpointAttemptRefV2      `json:"attempt"`
	Barrier   runtimeports.CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut runtimeports.EffectCutRefV2              `json:"effect_cut"`
}

type CheckpointParticipantWorkRequestV1 struct {
	Attempt     runtimeports.CheckpointAttemptRefV2      `json:"attempt"`
	Barrier     runtimeports.CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut   runtimeports.EffectCutRefV2              `json:"effect_cut"`
	Participant runtimeports.CheckpointParticipantRefV2  `json:"participant"`
	Gate        CheckpointExternalExactRefV1             `json:"gate"`
	Snapshot    CheckpointExternalExactRefV1             `json:"snapshot"`
	NotAfter    int64                                    `json:"not_after_unix_nano"`
}

func (r CheckpointParticipantWorkRequestV1) Validate(now time.Time) error {
	if r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.Participant.Validate() != nil || r.Gate.Validate() != nil || r.Snapshot.Validate() != nil || r.NotAfter <= 0 || now.IsZero() || !now.Before(time.Unix(0, r.NotAfter)) || r.Attempt.ID != r.Barrier.AttemptID || r.Attempt.ID != r.EffectCut.Attempt.ID || r.Attempt.TenantID != r.Gate.TenantID || r.Gate.TenantID != r.Snapshot.TenantID || r.Gate.ScopeDigest != r.Snapshot.ScopeDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "checkpoint Participant work request is invalid or stale")
	}
	return nil
}

type CreateCheckpointManifestSealRequestV1 struct {
	StableID                string                                                   `json:"stable_id"`
	SealID                  string                                                   `json:"seal_id"`
	IdempotencyKey          string                                                   `json:"idempotency_key"`
	Scope                   core.ExecutionScope                                      `json:"scope"`
	RunStableIdentityDigest core.Digest                                              `json:"run_stable_identity_digest"`
	Attempt                 runtimeports.CheckpointAttemptRefV2                      `json:"attempt"`
	Barrier                 runtimeports.CheckpointBarrierLeaseRefV2                 `json:"barrier"`
	EffectCut               runtimeports.EffectCutRefV2                              `json:"effect_cut"`
	Gate                    CheckpointExternalExactRefV1                             `json:"gate"`
	Snapshot                CheckpointExternalExactRefV1                             `json:"snapshot"`
	ParticipantSet          runtimeports.CheckpointParticipantSetCurrentProjectionV2 `json:"participant_set"`
	Closures                []runtimeports.CheckpointParticipantClosureRefV2         `json:"closures"`
	Input                   CheckpointManifestInputCurrentProjectionV1               `json:"input"`
	Participants            []CheckpointParticipantCommitV1                          `json:"participants"`
	RequestedNotAfter       int64                                                    `json:"requested_not_after_unix_nano"`
}

func (r CreateCheckpointManifestSealRequestV1) Clone() CreateCheckpointManifestSealRequestV1 {
	r.ParticipantSet.Participants = append([]runtimeports.CheckpointParticipantRefV2(nil), r.ParticipantSet.Participants...)
	r.Closures = append([]runtimeports.CheckpointParticipantClosureRefV2(nil), r.Closures...)
	r.Input = r.Input.Clone()
	r.Participants = make([]CheckpointParticipantCommitV1, len(r.Participants))
	for i := range r.Participants {
		r.Participants[i] = r.Participants[i].Clone()
	}
	return r
}

func (r CreateCheckpointManifestSealRequestV1) Validate(now time.Time) error {
	scopeDigest, scopeErr := runtimeports.ExecutionScopeDigestV2(r.Scope)
	if strings.TrimSpace(r.StableID) == "" || strings.TrimSpace(r.SealID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || r.Scope.Validate() != nil || r.RunStableIdentityDigest.Validate() != nil || scopeErr != nil || scopeDigest != r.Gate.ScopeDigest || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.Gate.Validate() != nil || r.Snapshot.Validate() != nil || r.ParticipantSet.Validate(now) != nil || r.Input.Validate(now) != nil || r.Input.Attempt != r.Attempt || len(r.Closures) == 0 || len(r.Closures) != len(r.ParticipantSet.Participants) || len(r.Participants) != len(r.Closures) || r.RequestedNotAfter <= 0 || now.IsZero() || !now.Before(time.Unix(0, r.RequestedNotAfter)) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "create checkpoint Manifest Seal request is invalid or stale")
	}
	participantSet := make(map[string]runtimeports.CheckpointParticipantRefV2, len(r.ParticipantSet.Participants))
	for _, participant := range r.ParticipantSet.Participants {
		participantSet[participant.ID] = participant
	}
	seen := make(map[string]struct{}, len(r.Closures))
	for index, closure := range r.Closures {
		expected, exists := participantSet[closure.Participant.ID]
		if closure.Validate() != nil || !exists || closure.Participant != expected || closure.Prepare.DomainResult.Attempt != r.Attempt || closure.Terminal == nil || closure.Terminal.DomainResult.Attempt != r.Attempt || (index > 0 && closure.ID <= r.Closures[index-1].ID) || r.Participants[index].RuntimeClosure != closure || r.Participants[index].ValidateForAttemptV1(closure.Participant, r.Attempt) != nil {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Manifest closures are not the exact canonical Participant set")
		}
		if _, duplicate := seen[closure.Participant.ID]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint Manifest repeats a Participant closure")
		}
		seen[closure.Participant.ID] = struct{}{}
	}
	return nil
}

type InspectCheckpointManifestSealRequestV1 struct {
	Ref runtimeports.CheckpointManifestSealRefV2 `json:"ref"`
}

func (r InspectCheckpointManifestSealRequestV1) Validate() error { return r.Ref.Validate() }
