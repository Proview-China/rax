package contract

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const SettledTurnResultContractV2 = "praxis.harness.settled-turn/v2"

type SettledTurnResultStateV2 string

const (
	SettledTurnCompletedV2      SettledTurnResultStateV2 = "completed"
	SettledTurnActionRequiredV2 SettledTurnResultStateV2 = "action_required"
	SettledTurnInputRequiredV2  SettledTurnResultStateV2 = "input_required"
	SettledTurnFailedV2         SettledTurnResultStateV2 = "failed"
)

var (
	settledTurnSchemaDigestV2 = core.DigestBytes([]byte("praxis.harness/settled-turn-result@2.0.0"))
	settledTurnLimitDigestV2  = core.DigestBytes([]byte("praxis.inline/bounded@1.0.0"))
)

// SettledTurnResultV2 is authored by the bound Settlement Owner, not by the
// provider Observation or Application coordinator. Harness applies it only
// after Runtime returns an exact OperationSettlementRefV3.
type SettledTurnResultV2 struct {
	ContractVersion string                        `json:"contract_version"`
	Candidate       CandidateRefV2                `json:"candidate"`
	State           SettledTurnResultStateV2      `json:"state"`
	Output          *runtimeports.OpaquePayloadV2 `json:"output,omitempty"`
	Action          *PendingActionV2              `json:"action,omitempty"`
	Input           *PendingInputV2               `json:"input,omitempty"`
	FailureCode     runtimeports.NamespacedNameV2 `json:"failure_code,omitempty"`
	FailureDigest   core.Digest                   `json:"failure_digest,omitempty"`
}

func (r SettledTurnResultV2) Validate() error {
	if r.ContractVersion != SettledTurnResultContractV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "settled turn result contract is unsupported")
	}
	if err := r.Candidate.Validate(); err != nil {
		return err
	}
	switch r.State {
	case SettledTurnCompletedV2:
		if r.Output == nil || r.Action != nil || r.Input != nil || r.FailureCode != "" || r.FailureDigest != "" {
			return invalidSettledTurnResultV2()
		}
		return r.Output.Validate()
	case SettledTurnActionRequiredV2:
		if r.Output != nil || r.Action == nil || r.Input != nil || r.FailureCode != "" || r.FailureDigest != "" || r.Action.SourceCandidate != r.Candidate {
			return invalidSettledTurnResultV2()
		}
		return r.Action.Validate()
	case SettledTurnInputRequiredV2:
		if r.Output != nil || r.Action != nil || r.Input == nil || r.FailureCode != "" || r.FailureDigest != "" || r.Input.SourceCandidate != r.Candidate {
			return invalidSettledTurnResultV2()
		}
		return r.Input.Validate()
	case SettledTurnFailedV2:
		if r.Output != nil || r.Action != nil || r.Input != nil || runtimeports.ValidateNamespacedNameV2(r.FailureCode) != nil || r.FailureDigest.Validate() != nil {
			return invalidSettledTurnResultV2()
		}
		return nil
	default:
		return invalidSettledTurnResultV2()
	}
}

func invalidSettledTurnResultV2() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "settled turn result fields are inconsistent")
}

func (r SettledTurnResultV2) DigestV2() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.harness.settled-turn", SettledTurnResultContractV2, "SettledTurnResultV2", r)
}

func SettledTurnResultSchemaV2() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "settled-turn-result", Version: "2.0.0", MediaType: "application/json", ContentDigest: settledTurnSchemaDigestV2}
}

func NewSettledTurnDomainResultV2(result SettledTurnResultV2) (runtimeports.OpaquePayloadV2, error) {
	if err := result.Validate(); err != nil {
		return runtimeports.OpaquePayloadV2{}, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return runtimeports.OpaquePayloadV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "settled turn result cannot be encoded")
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema: SettledTurnResultSchemaV2(), ContentDigest: core.DigestBytes(body), Length: uint64(len(body)), Inline: body,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.inline/bounded", Digest: settledTurnLimitDigestV2},
	}
	return payload, payload.Validate()
}

func DecodeSettledTurnDomainResultV2(payload runtimeports.OpaquePayloadV2) (SettledTurnResultV2, error) {
	if err := payload.Validate(); err != nil {
		return SettledTurnResultV2{}, err
	}
	if payload.Schema != SettledTurnResultSchemaV2() || payload.Inline == nil || payload.LimitPolicy.Policy != "praxis.inline/bounded" || payload.LimitPolicy.Digest != settledTurnLimitDigestV2 {
		return SettledTurnResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "settled turn result schema, limit policy or locality is unsupported")
	}
	var result SettledTurnResultV2
	if err := core.DecodeStrictJSON(payload.Inline, &result); err != nil {
		return SettledTurnResultV2{}, err
	}
	if err := result.Validate(); err != nil {
		return SettledTurnResultV2{}, err
	}
	return result, nil
}

func NewSettledTurnFailureV2(candidate CandidateRefV2, code runtimeports.NamespacedNameV2, detail []byte) SettledTurnResultV2 {
	if len(detail) == 0 {
		detail = []byte("unspecified")
	}
	return SettledTurnResultV2{ContractVersion: SettledTurnResultContractV2, Candidate: candidate, State: SettledTurnFailedV2, FailureCode: code, FailureDigest: core.DigestBytes(detail)}
}

func (r SettledTurnResultV2) IsTerminalClaimV2() CompletionClaim {
	switch r.State {
	case SettledTurnCompletedV2:
		return ClaimCompleted
	case SettledTurnFailedV2:
		return ClaimFailed
	default:
		return ""
	}
}
