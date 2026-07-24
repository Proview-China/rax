package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ActionCandidate struct {
	ContractVersion       string                          `json:"contract_version"`
	ID                    string                          `json:"id"`
	Revision              core.Revision                   `json:"revision"`
	Digest                core.Digest                     `json:"digest"`
	RunID                 string                          `json:"run_id"`
	SessionID             string                          `json:"session_id"`
	PendingActionRef      string                          `json:"pending_action_ref"`
	PendingActionDigest   core.Digest                     `json:"pending_action_digest"`
	SourceCandidateDigest core.Digest                     `json:"source_candidate_digest"`
	Capability            ObjectRef                       `json:"capability"`
	Tool                  ObjectRef                       `json:"tool"`
	Payload               runtimeports.OpaquePayloadV2    `json:"payload"`
	PayloadRevision       core.Revision                   `json:"payload_revision"`
	ActionScopeDigest     core.Digest                     `json:"action_scope_digest"`
	EffectKinds           []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	Risk                  RiskClass                       `json:"risk"`
	ExpectedOwner         runtimeports.EffectOwnerRefV2   `json:"expected_owner"`
	ConflictDomain        string                          `json:"conflict_domain"`
	IdempotencyKey        string                          `json:"idempotency_key"`
	CreatedUnixNano       int64                           `json:"created_unix_nano"`
	ExpiresUnixNano       int64                           `json:"expires_unix_nano"`
}

func (c ActionCandidate) validateShape() error {
	if c.ContractVersion != ActionContractVersion || ValidateStableID(c.ID) != nil || c.Revision == 0 || c.PayloadRevision == 0 || c.CreatedUnixNano <= 0 || c.ExpiresUnixNano <= c.CreatedUnixNano {
		return invalid("action candidate identity, revision or lifetime is invalid")
	}
	for _, value := range []string{c.RunID, c.SessionID, c.PendingActionRef, c.ConflictDomain, c.IdempotencyKey} {
		if strings.TrimSpace(value) == "" || len(value) > MaxStringBytes {
			return invalid("action candidate binding is blank or unbounded")
		}
	}
	if c.PendingActionDigest.Validate() != nil || c.SourceCandidateDigest.Validate() != nil || c.Capability.Validate() != nil || c.Tool.Validate() != nil || c.Payload.Validate() != nil || c.ActionScopeDigest.Validate() != nil || ValidateSortedUniqueNames(c.EffectKinds, MaxDescriptorEffects) != nil {
		return invalid("action candidate references, payload or effects are invalid")
	}
	if c.Risk != RiskLow && c.Risk != RiskModerate && c.Risk != RiskHigh {
		return invalid("action candidate risk is invalid")
	}
	if c.ExpectedOwner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.ExpectedOwner.ComponentID)) != nil || c.ExpectedOwner.ManifestDigest.Validate() != nil {
		return invalid("action candidate expected settlement owner is invalid")
	}
	return nil
}

// PendingActionProjectionDigest binds the Tool-owned projection of the
// Harness PendingAction subject. It does not replace or recompute Harness'
// RequestDigest; the Controller compares that owner-provided digest separately.
func (c ActionCandidate) PendingActionProjectionDigest() (core.Digest, error) {
	if strings.TrimSpace(c.PendingActionRef) == "" || c.PendingActionDigest.Validate() != nil || c.Capability.Validate() != nil || c.Payload.Validate() != nil || c.SourceCandidateDigest.Validate() != nil {
		return "", invalid("pending action projection is incomplete")
	}
	return Seal("praxis.tool-mcp.action", ActionContractVersion, "PendingActionProjection", struct {
		Ref             string                       `json:"pending_action_ref"`
		RequestDigest   core.Digest                  `json:"pending_action_digest"`
		Capability      ObjectRef                    `json:"capability"`
		Payload         runtimeports.OpaquePayloadV2 `json:"payload"`
		SourceCandidate core.Digest                  `json:"source_candidate_digest"`
	}{c.PendingActionRef, c.PendingActionDigest, c.Capability, c.Payload, c.SourceCandidateDigest})
}

func (c ActionCandidate) Validate() error {
	if err := c.validateShape(); err != nil {
		return err
	}
	if err := c.Digest.Validate(); err != nil {
		return err
	}
	expected, err := c.ComputeDigest()
	if err != nil || expected != c.Digest {
		return conflict("action candidate digest does not bind exact content")
	}
	return nil
}

func (c ActionCandidate) ComputeDigest() (core.Digest, error) {
	if err := c.validateShape(); err != nil {
		return "", err
	}
	c.Digest = ""
	return Seal("praxis.tool-mcp.action", ActionContractVersion, "ActionCandidate", c)
}

func SealActionCandidate(c ActionCandidate) (ActionCandidate, error) {
	c.ContractVersion = ActionContractVersion
	c.EffectKinds = SortedUniqueNames(c.EffectKinds)
	c.Digest = ""
	digest, err := c.ComputeDigest()
	if err != nil {
		return ActionCandidate{}, err
	}
	c.Digest = digest
	return c, nil
}

type ActionReservationFact struct {
	ContractVersion          string        `json:"contract_version"`
	ID                       string        `json:"id"`
	Revision                 core.Revision `json:"revision"`
	Digest                   core.Digest   `json:"digest"`
	Action                   ObjectRef     `json:"action"`
	ApplicationAttemptDigest core.Digest   `json:"application_attempt_digest"`
	IntentDigest             core.Digest   `json:"intent_digest"`
	SessionRef               string        `json:"session_ref"`
	DomainSubjectDigest      core.Digest   `json:"domain_subject_digest"`
	ReservedUnixNano         int64         `json:"reserved_unix_nano"`
	ExpiresUnixNano          int64         `json:"expires_unix_nano"`
}

func (f ActionReservationFact) validateShape() error {
	if f.ContractVersion != ActionContractVersion || ValidateStableID(f.ID) != nil || f.Revision != 1 || f.Action.Validate() != nil || strings.TrimSpace(f.SessionRef) == "" || f.ReservedUnixNano <= 0 || f.ExpiresUnixNano <= f.ReservedUnixNano {
		return invalid("action reservation identity or lifetime is invalid")
	}
	for _, digest := range []core.Digest{f.ApplicationAttemptDigest, f.IntentDigest, f.DomainSubjectDigest} {
		if digest.Validate() != nil {
			return invalid("action reservation digest is invalid")
		}
	}
	return nil
}

func (f ActionReservationFact) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	expected, err := f.ComputeDigest()
	if err != nil || f.Digest.Validate() != nil || expected != f.Digest {
		return conflict("reservation digest does not bind exact content")
	}
	return nil
}

func (f ActionReservationFact) ComputeDigest() (core.Digest, error) {
	if err := f.validateShape(); err != nil {
		return "", err
	}
	f.Digest = ""
	return Seal("praxis.tool-mcp.action", ActionContractVersion, "ActionReservationFact", f)
}

func SealReservation(f ActionReservationFact) (ActionReservationFact, error) {
	f.ContractVersion = ActionContractVersion
	f.Revision = 1
	f.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return ActionReservationFact{}, err
	}
	f.Digest = digest
	return f, nil
}

type DomainResultFact struct {
	ContractVersion   string                       `json:"contract_version"`
	ID                string                       `json:"id"`
	Revision          core.Revision                `json:"revision"`
	Digest            core.Digest                  `json:"digest"`
	Action            ObjectRef                    `json:"action"`
	AttemptID         string                       `json:"attempt_id"`
	ObservationDigest core.Digest                  `json:"observation_digest"`
	Payload           runtimeports.OpaquePayloadV2 `json:"payload"`
	Residuals         []Residual                   `json:"residuals,omitempty"`
	CreatedUnixNano   int64                        `json:"created_unix_nano"`
}

func (f DomainResultFact) validateShape() error {
	if f.ContractVersion != ResultContractVersion || ValidateStableID(f.ID) != nil || f.Revision != 1 || f.Action.Validate() != nil || strings.TrimSpace(f.AttemptID) == "" || f.ObservationDigest.Validate() != nil || f.Payload.Validate() != nil || f.CreatedUnixNano <= 0 || len(f.Residuals) > MaxResiduals {
		return invalid("domain result fact is incomplete")
	}
	for _, residual := range f.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f DomainResultFact) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	expected, err := f.ComputeDigest()
	if err != nil || f.Digest.Validate() != nil || expected != f.Digest {
		return conflict("domain result digest does not bind exact content")
	}
	return nil
}

func (f DomainResultFact) ComputeDigest() (core.Digest, error) {
	if err := f.validateShape(); err != nil {
		return "", err
	}
	f.Digest = ""
	return Seal("praxis.tool-mcp.result", ResultContractVersion, "DomainResultFact", f)
}

func SealDomainResult(f DomainResultFact) (DomainResultFact, error) {
	f.ContractVersion = ResultContractVersion
	f.Revision = 1
	f.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return DomainResultFact{}, err
	}
	f.Digest = digest
	return f, nil
}

type ToolResult struct {
	ContractVersion   string                                `json:"contract_version"`
	ID                string                                `json:"id"`
	Revision          core.Revision                         `json:"revision"`
	Digest            core.Digest                           `json:"digest"`
	Action            ObjectRef                             `json:"action"`
	DomainResult      ObjectRef                             `json:"domain_result"`
	Settlement        runtimeports.OperationSettlementRefV3 `json:"settlement"`
	FinalizedUnixNano int64                                 `json:"finalized_unix_nano"`
}

func (r ToolResult) validateShape() error {
	if r.ContractVersion != ResultContractVersion || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Action.Validate() != nil || r.DomainResult.Validate() != nil || r.Settlement.Validate() != nil || r.FinalizedUnixNano <= 0 {
		return invalid("tool result is incomplete")
	}
	return nil
}

func (r ToolResult) Validate() error {
	if err := r.validateShape(); err != nil {
		return err
	}
	expected, err := r.ComputeDigest()
	if err != nil || r.Digest.Validate() != nil || expected != r.Digest {
		return conflict("tool result digest does not bind exact content")
	}
	return nil
}

func (r ToolResult) ComputeDigest() (core.Digest, error) {
	if err := r.validateShape(); err != nil {
		return "", err
	}
	r.Digest = ""
	return Seal("praxis.tool-mcp.result", ResultContractVersion, "ToolResult", r)
}

func SealToolResult(r ToolResult) (ToolResult, error) {
	r.ContractVersion = ResultContractVersion
	r.Revision = 1
	r.Digest = ""
	digest, err := r.ComputeDigest()
	if err != nil {
		return ToolResult{}, err
	}
	r.Digest = digest
	return r, nil
}
