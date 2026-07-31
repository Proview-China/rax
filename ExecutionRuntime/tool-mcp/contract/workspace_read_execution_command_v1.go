package contract

import (
	"context"
	"reflect"
	"strconv"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	WorkspaceReadExecutionCommandContractVersionV1 = "praxis.tool-mcp/workspace-read-execution-command/v1"
	WorkspaceReadExecutionCommandKindV1            = runtimeports.NamespacedNameV2("praxis.tool/workspace-read-execution-command")
	WorkspaceReadExecutionStartCommittedV1         = "start_committed"
	WorkspaceReadExecutionDispatchIntentV1         = "dispatch_intent"
	MaxWorkspaceReadExecutionCommandCurrentTTLV1   = 15 * time.Second
)

const workspaceReadExecutionCommandCanonicalDomainV1 = "praxis.tool-mcp.workspace-read-execution-command"

type WorkspaceReadExecutionCommandRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r WorkspaceReadExecutionCommandRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("workspace.read execution command Ref is invalid")
	}
	return nil
}

func (r WorkspaceReadExecutionCommandRefV1) ObjectRefV1() ObjectRef {
	return ObjectRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

// WorkspaceReadExecutionCommandSourceV1 is the immutable nominal projection of
// Tool-owned Claim/ExecutionState/Binding/Candidate sources. The source bodies
// remain in their authoritative stores and are reread by the owner producer.
type WorkspaceReadExecutionCommandSourceV1 struct {
	RequestKey             applicationcontract.SingleCallToolActionInspectKeyV2 `json:"request_key"`
	ClaimRef               ObjectRef                                            `json:"claim_ref"`
	ExecutionStateRef      ObjectRef                                            `json:"execution_state_ref"`
	ExecutionStateKind     string                                               `json:"execution_state_kind"`
	ExecutionInputDigest   core.Digest                                          `json:"execution_input_digest"`
	ToolExecutionAttemptID string                                               `json:"tool_execution_attempt_id"`
	BindingCurrent         SingleCallToolActionBindingCurrentRefV2              `json:"binding_current"`
	Candidate              ObjectRef                                            `json:"candidate"`
	CandidateClosureDigest core.Digest                                          `json:"candidate_closure_digest"`
	InputContractCurrent   ToolInputContractCurrentRefV1                        `json:"input_contract_current"`
	Tool                   ObjectRef                                            `json:"tool"`
	ToolCurrent            ToolRegistryObjectCurrentRefV1                       `json:"tool_current"`
	Owner                  runtimeports.EffectOwnerRefV2                        `json:"owner"`
	Digest                 core.Digest                                          `json:"digest"`
}

func (s WorkspaceReadExecutionCommandSourceV1) Validate() error {
	if s.RequestKey.Validate() != nil || s.ClaimRef.Validate() != nil ||
		s.ClaimRef.Revision != 1 ||
		s.ExecutionStateRef.Validate() != nil || s.ExecutionStateKind != WorkspaceReadExecutionStartCommittedV1 ||
		s.ExecutionStateRef.Revision != 1 ||
		s.ExecutionInputDigest.Validate() != nil || ValidateStableID(s.ToolExecutionAttemptID) != nil ||
		s.BindingCurrent.Validate() != nil || s.Candidate.Validate() != nil ||
		s.CandidateClosureDigest.Validate() != nil || s.InputContractCurrent.Validate() != nil ||
		s.Tool.Validate() != nil || s.ToolCurrent.Validate() != nil ||
		s.ToolCurrent.Kind != ToolRegistryDescriptorCurrentKindV1 || validateEffectOwner(s.Owner) != nil {
		return invalid("workspace.read execution command source is incomplete")
	}
	expectedClaimID, err := StableID(
		"tool-owner-single-call-claim-v2",
		s.RequestKey.RequestID,
		string(s.RequestKey.RequestDigest),
		string(s.RequestKey.ScopeDigest),
	)
	if err != nil || expectedClaimID != s.ClaimRef.ID {
		return conflict("workspace.read Tool owner claim identity drifted")
	}
	expectedStateID, err := StableID("tool-owner-execution-state-v2", s.ClaimRef.ID, string(s.ExecutionInputDigest))
	if err != nil || expectedStateID != s.ExecutionStateRef.ID {
		return conflict("workspace.read execution state identity drifted")
	}
	expectedAttemptID, err := StableID("tool-owner-execution-attempt-v2", s.ClaimRef.ID, string(s.ExecutionInputDigest))
	if err != nil || expectedAttemptID != s.ToolExecutionAttemptID {
		return conflict("workspace.read Tool execution attempt identity drifted")
	}
	digest, err := s.ComputeDigestV1()
	if err != nil || digest != s.Digest {
		return conflict("workspace.read execution command source digest drifted")
	}
	return nil
}

func (s WorkspaceReadExecutionCommandSourceV1) ComputeDigestV1() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(
		workspaceReadExecutionCommandCanonicalDomainV1,
		WorkspaceReadExecutionCommandContractVersionV1,
		"WorkspaceReadExecutionCommandSourceV1",
		s,
	)
}

func SealWorkspaceReadExecutionCommandSourceV1(s WorkspaceReadExecutionCommandSourceV1) (WorkspaceReadExecutionCommandSourceV1, error) {
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigestV1()
	if err != nil {
		return WorkspaceReadExecutionCommandSourceV1{}, err
	}
	if provided != "" && provided != digest {
		return WorkspaceReadExecutionCommandSourceV1{}, conflict("supplied workspace.read execution command source digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type WorkspaceReadExecutionCommandTTLClosureV1 struct {
	ClaimCreatedUnixNano          int64       `json:"claim_created_unix_nano"`
	StateUpdatedUnixNano          int64       `json:"state_updated_unix_nano"`
	BindingCheckedUnixNano        int64       `json:"binding_checked_unix_nano"`
	CandidateCreatedUnixNano      int64       `json:"candidate_created_unix_nano"`
	InputCheckedUnixNano          int64       `json:"input_contract_checked_unix_nano"`
	PreparedUnixNano              int64       `json:"prepared_unix_nano"`
	EffectiveCreatedLowerUnixNano int64       `json:"effective_created_lower_unix_nano"`
	RequestedNotAfterUnixNano     int64       `json:"requested_not_after_unix_nano"`
	RequestExpiresUnixNano        int64       `json:"request_expires_unix_nano"`
	StateExpiresUnixNano          int64       `json:"state_expires_unix_nano"`
	BindingExpiresUnixNano        int64       `json:"binding_expires_unix_nano"`
	CandidateExpiresUnixNano      int64       `json:"candidate_expires_unix_nano"`
	InputExpiresUnixNano          int64       `json:"input_contract_expires_unix_nano"`
	EffectIntentExpiresUnixNano   int64       `json:"effect_intent_expires_unix_nano"`
	PreparedExpiresUnixNano       int64       `json:"prepared_expires_unix_nano"`
	EffectiveNotAfterUnixNano     int64       `json:"effective_not_after_unix_nano"`
	Digest                        core.Digest `json:"digest"`
}

func (c WorkspaceReadExecutionCommandTTLClosureV1) Validate() error {
	lowerBounds := c.lowerBoundsV1()
	for _, value := range lowerBounds {
		if value <= 0 {
			return invalid("workspace.read execution command time lower-bound closure is incomplete")
		}
	}
	if maxWorkspaceReadExecutionUnixNanoV1(lowerBounds...) != c.EffectiveCreatedLowerUnixNano {
		return conflict("workspace.read execution command time lower-bound closure drifted")
	}
	values := c.upperBoundsV1()
	for _, value := range values {
		if value <= 0 {
			return invalid("workspace.read execution command TTL closure is incomplete")
		}
	}
	if minWorkspaceReadExecutionUnixNanoV1(values...) != c.EffectiveNotAfterUnixNano {
		return conflict("workspace.read execution command TTL closure minimum drifted")
	}
	digest, err := c.ComputeDigestV1()
	if err != nil || digest != c.Digest {
		return conflict("workspace.read execution command TTL closure digest drifted")
	}
	return nil
}

func (c WorkspaceReadExecutionCommandTTLClosureV1) lowerBoundsV1() []int64 {
	return []int64{
		c.ClaimCreatedUnixNano,
		c.StateUpdatedUnixNano,
		c.BindingCheckedUnixNano,
		c.CandidateCreatedUnixNano,
		c.InputCheckedUnixNano,
		c.PreparedUnixNano,
	}
}

func (c WorkspaceReadExecutionCommandTTLClosureV1) upperBoundsV1() []int64 {
	return []int64{
		c.RequestedNotAfterUnixNano,
		c.RequestExpiresUnixNano,
		c.StateExpiresUnixNano,
		c.BindingExpiresUnixNano,
		c.CandidateExpiresUnixNano,
		c.InputExpiresUnixNano,
		c.EffectIntentExpiresUnixNano,
		c.PreparedExpiresUnixNano,
	}
}

func (c WorkspaceReadExecutionCommandTTLClosureV1) ComputeDigestV1() (core.Digest, error) {
	c.Digest = ""
	return core.CanonicalJSONDigest(
		workspaceReadExecutionCommandCanonicalDomainV1,
		WorkspaceReadExecutionCommandContractVersionV1,
		"WorkspaceReadExecutionCommandTTLClosureV1",
		c,
	)
}

func SealWorkspaceReadExecutionCommandTTLClosureV1(c WorkspaceReadExecutionCommandTTLClosureV1) (WorkspaceReadExecutionCommandTTLClosureV1, error) {
	c.EffectiveCreatedLowerUnixNano = maxWorkspaceReadExecutionUnixNanoV1(c.lowerBoundsV1()...)
	c.EffectiveNotAfterUnixNano = minWorkspaceReadExecutionUnixNanoV1(c.upperBoundsV1()...)
	provided := c.Digest
	c.Digest = ""
	digest, err := c.ComputeDigestV1()
	if err != nil {
		return WorkspaceReadExecutionCommandTTLClosureV1{}, err
	}
	if provided != "" && provided != digest {
		return WorkspaceReadExecutionCommandTTLClosureV1{}, conflict("supplied workspace.read execution command TTL closure digest drifted")
	}
	c.Digest = digest
	return c, c.Validate()
}

type WorkspaceReadExecutionCommandV1 struct {
	ContractVersion           string                                     `json:"contract_version"`
	Ref                       WorkspaceReadExecutionCommandRefV1         `json:"ref"`
	Source                    WorkspaceReadExecutionCommandSourceV1      `json:"source"`
	Operation                 runtimeports.OperationSubjectV3            `json:"operation"`
	OperationDigest           core.Digest                                `json:"operation_digest"`
	Prepared                  runtimeports.PreparedProviderAttemptRefV2  `json:"prepared"`
	PreparedSemanticDigest    core.Digest                                `json:"prepared_semantic_digest"`
	RuntimeAttempt            runtimeports.OperationDispatchAttemptRefV3 `json:"runtime_attempt"`
	RuntimeAttemptDigest      core.Digest                                `json:"runtime_attempt_digest"`
	RuntimeEffectIntentDigest core.Digest                                `json:"runtime_effect_intent_digest"`
	RuntimeEffectFactRevision core.Revision                              `json:"runtime_effect_fact_revision"`
	RuntimeEffectState        string                                     `json:"runtime_effect_state"`
	PayloadSchema             runtimeports.SchemaRefV2                   `json:"payload_schema"`
	PayloadDigest             core.Digest                                `json:"payload_digest"`
	PayloadRevision           core.Revision                              `json:"payload_revision"`
	TTL                       WorkspaceReadExecutionCommandTTLClosureV1  `json:"ttl"`
	CreatedUnixNano           int64                                      `json:"created_unix_nano"`
	NotAfterUnixNano          int64                                      `json:"not_after_unix_nano"`
}

func (f WorkspaceReadExecutionCommandV1) Validate() error {
	if f.ContractVersion != WorkspaceReadExecutionCommandContractVersionV1 ||
		f.Ref.Validate() != nil || f.Source.Validate() != nil || f.Operation.Validate() != nil ||
		f.OperationDigest.Validate() != nil || f.Prepared.Validate() != nil ||
		f.PreparedSemanticDigest.Validate() != nil ||
		f.RuntimeAttempt.Validate() != nil || f.RuntimeAttemptDigest.Validate() != nil ||
		f.RuntimeEffectIntentDigest.Validate() != nil || f.RuntimeEffectFactRevision == 0 ||
		f.RuntimeEffectState != WorkspaceReadExecutionDispatchIntentV1 || f.PayloadSchema.Validate() != nil ||
		f.PayloadDigest.Validate() != nil || f.PayloadRevision == 0 || f.TTL.Validate() != nil ||
		f.CreatedUnixNano <= 0 || f.NotAfterUnixNano <= f.CreatedUnixNano {
		return invalid("workspace.read execution command is incomplete")
	}
	operationDigest, err := f.Operation.DigestV3()
	if err != nil || operationDigest != f.OperationDigest ||
		f.RuntimeAttempt.OperationDigest != f.OperationDigest ||
		f.Prepared.OperationDigest != f.OperationDigest ||
		f.Prepared.IntentID != f.RuntimeAttempt.EffectID ||
		f.Prepared.IntentRevision != f.RuntimeAttempt.IntentRevision ||
		f.Prepared.IntentDigest != f.RuntimeAttempt.IntentDigest ||
		f.RuntimeEffectIntentDigest != f.RuntimeAttempt.IntentDigest ||
		f.Prepared.PermitID != f.RuntimeAttempt.PermitID ||
		f.Prepared.PermitRevision != f.RuntimeAttempt.PermitRevision ||
		f.Prepared.PermitDigest != f.RuntimeAttempt.PermitDigest ||
		f.Prepared.AttemptID != f.RuntimeAttempt.AttemptID {
		return conflict("workspace.read execution command Runtime operation or Attempt drifted")
	}
	if f.RuntimeAttempt.Delegation == nil ||
		f.RuntimeAttempt.Delegation.ID != f.Prepared.DeclaredDelegation.ID ||
		f.RuntimeAttempt.Delegation.Revision <= f.Prepared.DeclaredDelegation.Revision {
		return conflict("workspace.read execution command Runtime delegation drifted")
	}
	attemptDigest, err := DigestWorkspaceReadExecutionRuntimeAttemptV1(f.RuntimeAttempt)
	if err != nil || attemptDigest != f.RuntimeAttemptDigest {
		return conflict("workspace.read execution command Runtime Attempt digest drifted")
	}
	if f.Prepared.PayloadSchema != f.PayloadSchema ||
		f.Prepared.PayloadDigest != f.PayloadDigest ||
		f.Prepared.PayloadRevision != f.PayloadRevision ||
		f.Operation.ExecutionScopeDigest != f.Source.RequestKey.ScopeDigest {
		return conflict("workspace.read execution command payload or scope drifted")
	}
	if f.NotAfterUnixNano != f.TTL.EffectiveNotAfterUnixNano ||
		f.NotAfterUnixNano > f.Prepared.ExpiresUnixNano ||
		f.Prepared.PreparedUnixNano != f.TTL.PreparedUnixNano ||
		f.CreatedUnixNano < f.TTL.EffectiveCreatedLowerUnixNano {
		return conflict("workspace.read execution command time closure drifted")
	}
	if f.Source.Owner.ComponentID != f.Prepared.Provider.ComponentID ||
		f.Source.Owner.ManifestDigest != f.Prepared.Provider.ManifestDigest {
		return conflict("workspace.read execution command owner/provider drifted")
	}
	id, err := DeriveWorkspaceReadExecutionCommandIDV1(f.Source, f.Prepared, f.RuntimeAttempt)
	if err != nil || id != f.Ref.ID || f.Ref.Revision != 1 {
		return conflict("workspace.read execution command identity drifted")
	}
	digest, err := f.ComputeDigestV1()
	if err != nil || digest != f.Ref.Digest {
		return conflict("workspace.read execution command digest drifted")
	}
	return nil
}

func (f WorkspaceReadExecutionCommandV1) ValidateCurrent(now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < f.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command clock regressed")
	}
	if !now.Before(time.Unix(0, f.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "workspace.read execution command expired")
	}
	return nil
}

func (f WorkspaceReadExecutionCommandV1) ComputeDigestV1() (core.Digest, error) {
	f = CloneWorkspaceReadExecutionCommandV1(f)
	f.Ref.Digest = ""
	return core.CanonicalJSONDigest(
		workspaceReadExecutionCommandCanonicalDomainV1,
		WorkspaceReadExecutionCommandContractVersionV1,
		"WorkspaceReadExecutionCommandV1",
		f,
	)
}

func SealWorkspaceReadExecutionCommandV1(f WorkspaceReadExecutionCommandV1) (WorkspaceReadExecutionCommandV1, error) {
	f = CloneWorkspaceReadExecutionCommandV1(f)
	f.ContractVersion = WorkspaceReadExecutionCommandContractVersionV1
	id, err := DeriveWorkspaceReadExecutionCommandIDV1(f.Source, f.Prepared, f.RuntimeAttempt)
	if err != nil {
		return WorkspaceReadExecutionCommandV1{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id {
		return WorkspaceReadExecutionCommandV1{}, conflict("supplied workspace.read execution command ID drifted")
	}
	f.Ref.ID, f.Ref.Revision = id, 1
	provided := f.Ref.Digest
	f.Ref.Digest = ""
	digest, err := f.ComputeDigestV1()
	if err != nil {
		return WorkspaceReadExecutionCommandV1{}, err
	}
	if provided != "" && provided != digest {
		return WorkspaceReadExecutionCommandV1{}, conflict("supplied workspace.read execution command digest drifted")
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveWorkspaceReadExecutionCommandIDV1(
	source WorkspaceReadExecutionCommandSourceV1,
	prepared runtimeports.PreparedProviderAttemptRefV2,
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (string, error) {
	if source.Validate() != nil || prepared.Validate() != nil || attempt.Validate() != nil {
		return "", invalid("workspace.read execution command identity inputs are invalid")
	}
	attemptDigest, err := DigestWorkspaceReadExecutionRuntimeAttemptV1(attempt)
	if err != nil {
		return "", err
	}
	return StableID(
		"workspace-read-command-v1",
		string(source.Digest),
		source.Candidate.ID,
		string(source.Candidate.Digest),
		source.ToolExecutionAttemptID,
		prepared.ID,
		strconv.FormatUint(uint64(prepared.Revision), 10),
		string(prepared.Digest),
		string(attemptDigest),
	)
}

func DigestWorkspaceReadExecutionRuntimeAttemptV1(
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (core.Digest, error) {
	if err := attempt.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		workspaceReadExecutionCommandCanonicalDomainV1,
		WorkspaceReadExecutionCommandContractVersionV1,
		"OperationDispatchAttemptRefV3",
		attempt,
	)
}

func SameWorkspaceReadExecutionRuntimeAttemptV1(
	left, right runtimeports.OperationDispatchAttemptRefV3,
) bool {
	leftDigest, leftErr := DigestWorkspaceReadExecutionRuntimeAttemptV1(left)
	rightDigest, rightErr := DigestWorkspaceReadExecutionRuntimeAttemptV1(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest && reflect.DeepEqual(left, right)
}

// SameWorkspaceReadExecutionCommandStableClosureV1 compares every immutable
// semantic axis while deliberately excluding the Tool-owner creation instant
// and the resulting exact Fact digest. Concurrent contenders may create
// different candidates, but only one durable winner is authoritative.
func SameWorkspaceReadExecutionCommandStableClosureV1(
	left, right WorkspaceReadExecutionCommandV1,
) (bool, error) {
	if err := left.Validate(); err != nil {
		return false, err
	}
	if err := right.Validate(); err != nil {
		return false, err
	}
	left = CloneWorkspaceReadExecutionCommandV1(left)
	right = CloneWorkspaceReadExecutionCommandV1(right)
	left.Ref.Digest, right.Ref.Digest = "", ""
	left.CreatedUnixNano, right.CreatedUnixNano = 0, 0
	return reflect.DeepEqual(left, right), nil
}

func (f WorkspaceReadExecutionCommandV1) ObjectRefV1() ObjectRef {
	return f.Ref.ObjectRefV1()
}

func CloneWorkspaceReadExecutionCommandV1(f WorkspaceReadExecutionCommandV1) WorkspaceReadExecutionCommandV1 {
	if f.Operation.ExecutionScope.SandboxLease != nil {
		lease := *f.Operation.ExecutionScope.SandboxLease
		f.Operation.ExecutionScope.SandboxLease = &lease
	}
	if f.RuntimeAttempt.Delegation != nil {
		delegation := *f.RuntimeAttempt.Delegation
		f.RuntimeAttempt.Delegation = &delegation
	}
	return f
}

type WorkspaceReadExecutionCommandCurrentV1 struct {
	ContractVersion                string                          `json:"contract_version"`
	Fact                           WorkspaceReadExecutionCommandV1 `json:"fact"`
	ToolCurrentProjectionDigest    core.Digest                     `json:"tool_current_projection_digest"`
	ToolCurrentCheckedUnixNano     int64                           `json:"tool_current_checked_unix_nano"`
	RuntimeEffectCurrentDigest     core.Digest                     `json:"runtime_effect_current_digest"`
	RuntimeEffectCheckedUnixNano   int64                           `json:"runtime_effect_checked_unix_nano"`
	RuntimePreparedCurrentDigest   core.Digest                     `json:"runtime_prepared_current_digest"`
	RuntimePreparedCheckedUnixNano int64                           `json:"runtime_prepared_checked_unix_nano"`
	CheckedUnixNano                int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano                int64                           `json:"expires_unix_nano"`
	Digest                         core.Digest                     `json:"digest"`
}

func (p WorkspaceReadExecutionCommandCurrentV1) ValidateCurrent(
	expected WorkspaceReadExecutionCommandRefV1,
	now time.Time,
) error {
	if p.ContractVersion != WorkspaceReadExecutionCommandContractVersionV1 ||
		p.Fact.Validate() != nil || p.Fact.Ref != expected ||
		p.ToolCurrentProjectionDigest.Validate() != nil || p.ToolCurrentCheckedUnixNano <= 0 ||
		p.RuntimeEffectCurrentDigest.Validate() != nil || p.RuntimeEffectCheckedUnixNano <= 0 ||
		p.RuntimePreparedCurrentDigest.Validate() != nil || p.RuntimePreparedCheckedUnixNano <= 0 ||
		p.CheckedUnixNano < p.ToolCurrentCheckedUnixNano ||
		p.CheckedUnixNano < p.RuntimeEffectCheckedUnixNano ||
		p.CheckedUnixNano < p.RuntimePreparedCheckedUnixNano ||
		p.CheckedUnixNano <= 0 ||
		p.CheckedUnixNano < p.Fact.CreatedUnixNano ||
		p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.Fact.NotAfterUnixNano ||
		time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxWorkspaceReadExecutionCommandCurrentTTLV1 ||
		p.Digest.Validate() != nil {
		return invalid("workspace.read execution command current is incomplete")
	}
	if err := p.Fact.ValidateCurrent(time.Unix(0, p.CheckedUnixNano)); err != nil {
		return err
	}
	digest, err := p.ComputeDigestV1()
	if err != nil || digest != p.Digest {
		return conflict("workspace.read execution command current digest drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "workspace.read execution command current expired")
	}
	return nil
}

func (p WorkspaceReadExecutionCommandCurrentV1) ComputeDigestV1() (core.Digest, error) {
	p.Fact = CloneWorkspaceReadExecutionCommandV1(p.Fact)
	p.Digest = ""
	return core.CanonicalJSONDigest(
		workspaceReadExecutionCommandCanonicalDomainV1,
		WorkspaceReadExecutionCommandContractVersionV1,
		"WorkspaceReadExecutionCommandCurrentV1",
		p,
	)
}

type WorkspaceReadExecutionCommandExactReaderV1 interface {
	InspectWorkspaceReadExecutionCommandExactV1(context.Context, WorkspaceReadExecutionCommandRefV1) (WorkspaceReadExecutionCommandV1, error)
}

type WorkspaceReadExecutionCommandAttemptReaderV1 interface {
	InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(context.Context, runtimeports.OperationDispatchAttemptRefV3) (WorkspaceReadExecutionCommandV1, error)
}

type WorkspaceReadExecutionCommandCurrentReaderV1 interface {
	InspectWorkspaceReadExecutionCommandCurrentV1(context.Context, WorkspaceReadExecutionCommandRefV1) (WorkspaceReadExecutionCommandCurrentV1, error)
}

func minWorkspaceReadExecutionUnixNanoV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if result == 0 || value < result {
			result = value
		}
	}
	return result
}

func maxWorkspaceReadExecutionUnixNanoV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}
