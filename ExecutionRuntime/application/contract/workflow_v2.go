// Package contract defines transport-neutral Application workflow values.
package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	WorkflowContractVersionV2 = "praxis.application.workflow/v2"
	MaxWorkflowStepsV2        = 256
	MaxWorkflowDependenciesV2 = 64
)

type StepExecutionClassV2 string

const (
	StepCoordinationV2   StepExecutionClassV2 = "coordination"
	StepGovernedEffectV2 StepExecutionClassV2 = "governed_effect"
)

type CommandPayloadFactV2 struct {
	ContractVersion string                `json:"contract_version"`
	CommandID       string                `json:"command_id"`
	Revision        core.Revision         `json:"revision"`
	Payload         ports.OpaquePayloadV2 `json:"payload"`
	CreatedUnixNano int64                 `json:"created_unix_nano"`
}

func (f CommandPayloadFactV2) ValidateFor(command ports.ApplicationCommandEnvelopeV2) error {
	if f.ContractVersion != WorkflowContractVersionV2 || strings.TrimSpace(f.CommandID) == "" || f.CommandID != command.ID || f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command payload fact must bind one immutable command")
	}
	if err := command.Validate(); err != nil {
		return err
	}
	if err := f.Payload.Validate(); err != nil {
		return err
	}
	if f.Payload.ContentDigest != command.CanonicalPayloadDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "command payload content differs from accepted command digest")
	}
	return nil
}

func (f CommandPayloadFactV2) DigestV2() (core.Digest, error) {
	if f.ContractVersion != WorkflowContractVersionV2 || strings.TrimSpace(f.CommandID) == "" || f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command payload fact is incomplete")
	}
	if err := f.Payload.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.workflow", WorkflowContractVersionV2, "CommandPayloadFactV2", f)
}

type WorkflowStepV2 struct {
	ID             string                      `json:"id"`
	Kind           ports.NamespacedNameV2      `json:"kind"`
	Descriptor     StepDescriptorRefV2         `json:"descriptor"`
	ExecutionClass StepExecutionClassV2        `json:"execution_class"`
	Required       bool                        `json:"required"`
	Dependencies   []string                    `json:"dependencies"`
	Payload        ports.OpaquePayloadV2       `json:"payload"`
	Provider       *ports.ProviderBindingRefV2 `json:"provider,omitempty"`
	DomainAdapter  *ports.ProviderBindingRefV2 `json:"domain_adapter,omitempty"`
}

type StepDescriptorRefV2 struct {
	Kind            ports.NamespacedNameV2 `json:"kind"`
	Revision        core.Revision          `json:"revision"`
	Digest          core.Digest            `json:"digest"`
	ExpiresUnixNano int64                  `json:"expires_unix_nano"`
}

func (r StepDescriptorRefV2) Validate(kind ports.NamespacedNameV2) error {
	if err := ports.ValidateNamespacedNameV2(r.Kind); err != nil {
		return err
	}
	if r.Kind != kind || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "workflow step descriptor ref does not bind its exact kind, revision and lifetime")
	}
	return r.Digest.Validate()
}

func (s WorkflowStepV2) Validate() error {
	if strings.TrimSpace(s.ID) == "" || len(s.ID) > 256 || ports.ValidateNamespacedNameV2(s.Kind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow step id and namespaced kind are required")
	}
	if err := s.Descriptor.Validate(s.Kind); err != nil {
		return err
	}
	if err := s.Payload.Validate(); err != nil {
		return err
	}
	if len(s.Dependencies) > MaxWorkflowDependenciesV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "workflow dependency set exceeds its bound")
	}
	previous := ""
	for _, dependency := range s.Dependencies {
		if strings.TrimSpace(dependency) == "" || len(dependency) > 256 || dependency == s.ID || dependency <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "workflow dependencies must be sorted, unique and non-self")
		}
		previous = dependency
	}
	switch s.ExecutionClass {
	case StepCoordinationV2:
		if s.Provider != nil || s.DomainAdapter != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "coordination step cannot smuggle a provider")
		}
	case StepGovernedEffectV2:
		if s.Provider == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonProviderBindingStale, "governed effect step requires an exact provider binding")
		}
		if s.DomainAdapter == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonProviderBindingStale, "governed effect step requires an exact operation domain adapter binding")
		}
		if err := s.Provider.Validate(); err != nil {
			return err
		}
		if err := s.DomainAdapter.Validate(); err != nil {
			return err
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "workflow execution class is unknown")
	}
	return nil
}

type WorkflowPlanV2 struct {
	ContractVersion      string                      `json:"contract_version"`
	ID                   string                      `json:"id"`
	Revision             core.Revision               `json:"revision"`
	CommandID            string                      `json:"command_id"`
	CommandPayloadDigest core.Digest                 `json:"command_payload_digest"`
	Target               core.ExecutionScope         `json:"target"`
	Authority            ports.AuthorityBindingRefV2 `json:"authority"`
	Steps                []WorkflowStepV2            `json:"steps"`
	CreatedUnixNano      int64                       `json:"created_unix_nano"`
	ExpiresUnixNano      int64                       `json:"expires_unix_nano"`
}

func (p WorkflowPlanV2) Validate(now time.Time) error {
	if p.ContractVersion != WorkflowContractVersionV2 || strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.CommandID) == "" || p.Revision != 1 || p.CreatedUnixNano <= 0 || p.ExpiresUnixNano <= p.CreatedUnixNano || len(p.Steps) == 0 || len(p.Steps) > MaxWorkflowStepsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "workflow plan identity, lifetime and bounded steps are required")
	}
	if !now.IsZero() && !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "workflow plan expired")
	}
	if err := p.CommandPayloadDigest.Validate(); err != nil {
		return err
	}
	if err := p.Target.Validate(); err != nil {
		return err
	}
	if err := p.Authority.Validate(); err != nil {
		return err
	}
	byID := make(map[string]WorkflowStepV2, len(p.Steps))
	previous := ""
	for _, step := range p.Steps {
		if err := step.Validate(); err != nil {
			return err
		}
		if !now.IsZero() && !now.Before(time.Unix(0, step.Descriptor.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "workflow step descriptor expired")
		}
		if step.ID <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "workflow steps must be sorted and unique")
		}
		previous = step.ID
		byID[step.ID] = step
	}
	for _, step := range p.Steps {
		for _, dependency := range step.Dependencies {
			if _, ok := byID[dependency]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "workflow dependency is absent")
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "workflow dependency graph contains a cycle")
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range byID[id].Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id], visited[id] = false, true
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func (p WorkflowPlanV2) DigestV2() (core.Digest, error) {
	if err := p.Validate(time.Time{}); err != nil {
		return "", err
	}
	steps := make([]WorkflowStepV2, len(p.Steps))
	copy(steps, p.Steps)
	p.Steps = steps
	for index := range p.Steps {
		p.Steps[index].Dependencies = append([]string(nil), p.Steps[index].Dependencies...)
		if p.Steps[index].Dependencies == nil {
			p.Steps[index].Dependencies = []string{}
		}
	}
	return core.CanonicalJSONDigest("praxis.application.workflow", WorkflowContractVersionV2, "WorkflowPlanV2", p)
}

type SubmissionBundleV2 struct {
	Command ports.ApplicationCommandEnvelopeV2 `json:"command"`
	Payload CommandPayloadFactV2               `json:"payload"`
	Plan    WorkflowPlanV2                     `json:"plan"`
}

func (b SubmissionBundleV2) Validate(now time.Time) error {
	if err := b.Command.Validate(); err != nil {
		return err
	}
	if err := b.Payload.ValidateFor(b.Command); err != nil {
		return err
	}
	payloadDigest, err := b.Payload.DigestV2()
	if err != nil {
		return err
	}
	if err := b.Plan.Validate(now); err != nil {
		return err
	}
	if b.Plan.CommandID != b.Command.ID || b.Plan.CommandPayloadDigest != payloadDigest || !ports.SameExecutionScopeV2(b.Plan.Target, b.Command.Target) || b.Plan.Authority.Ref != b.Command.AuthorityRef || b.Plan.Authority.Epoch != b.Command.Target.AuthorityEpoch {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "workflow plan differs from accepted command, payload, target or authority")
	}
	return nil
}
