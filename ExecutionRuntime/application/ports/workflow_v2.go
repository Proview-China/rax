// Package ports defines dependencies owned by the Application layer.
package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SubmissionFactPortV2 interface {
	CreateSubmissionBundleV2(context.Context, contract.SubmissionBundleV2) (contract.SubmissionBundleV2, error)
	InspectSubmissionBundleV2(context.Context, core.ExecutionScope, string) (contract.SubmissionBundleV2, error)
}

type WorkflowJournalCASRequestV2 struct {
	ExpectedRevision core.Revision              `json:"expected_revision"`
	Next             contract.WorkflowJournalV2 `json:"next"`
}

type WorkflowJournalFactPortV2 interface {
	CreateWorkflowJournalV2(context.Context, contract.WorkflowPlanV2, contract.WorkflowJournalV2) (contract.WorkflowJournalV2, error)
	InspectWorkflowJournalV2(context.Context, core.ExecutionScope, string) (contract.WorkflowJournalV2, error)
	CompareAndSwapWorkflowJournalV2(context.Context, contract.WorkflowPlanV2, WorkflowJournalCASRequestV2) (contract.WorkflowJournalV2, error)
}

type WorkflowJournalClaimStateV2 string

const (
	WorkflowJournalClaimActiveV2   WorkflowJournalClaimStateV2 = "active"
	WorkflowJournalClaimReleasedV2 WorkflowJournalClaimStateV2 = "released"
)

// WorkflowJournalClaimV2 is a coordination lease, not execution authority.
// Losing it may transfer orchestration to another worker, but never grants or
// revokes Effect, Review, Budget, Binding, Fence or provider authority.
type WorkflowJournalClaimV2 struct {
	Scope            core.ExecutionScope         `json:"scope"`
	JournalID        string                      `json:"journal_id"`
	PlanDigest       core.Digest                 `json:"plan_digest"`
	OwnerID          string                      `json:"owner_id"`
	PolicyDigest     core.Digest                 `json:"policy_digest"`
	Epoch            uint64                      `json:"epoch"`
	Revision         core.Revision               `json:"revision"`
	State            WorkflowJournalClaimStateV2 `json:"state"`
	AcquiredUnixNano int64                       `json:"acquired_unix_nano"`
	UpdatedUnixNano  int64                       `json:"updated_unix_nano"`
	ExpiresUnixNano  int64                       `json:"expires_unix_nano"`
}

func (c WorkflowJournalClaimV2) ValidateFor(journal contract.WorkflowJournalV2) error {
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.JournalID) == "" || c.JournalID != journal.ID || strings.TrimSpace(c.OwnerID) == "" || len(c.OwnerID) > 256 || c.PlanDigest != journal.PlanDigest || c.Epoch == 0 || c.Revision == 0 || c.AcquiredUnixNano <= 0 || c.UpdatedUnixNano < c.AcquiredUnixNano || c.ExpiresUnixNano <= c.AcquiredUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow journal claim must bind an exact journal, owner, epoch and lifetime")
	}
	if err := c.PolicyDigest.Validate(); err != nil {
		return err
	}
	if c.State != WorkflowJournalClaimActiveV2 && c.State != WorkflowJournalClaimReleasedV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "workflow journal claim state is unknown")
	}
	if c.State == WorkflowJournalClaimActiveV2 && c.ExpiresUnixNano <= c.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "active workflow journal claim must retain positive lifetime")
	}
	return nil
}

type WorkflowJournalClaimRequestV2 struct {
	Scope            core.ExecutionScope `json:"scope"`
	JournalID        string              `json:"journal_id"`
	OwnerID          string              `json:"owner_id"`
	PolicyDigest     core.Digest         `json:"policy_digest"`
	ExpectedRevision core.Revision       `json:"expected_revision"`
	LeaseNanos       int64               `json:"lease_nanos"`
}

func (r WorkflowJournalClaimRequestV2) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.JournalID != strings.TrimSpace(r.JournalID) || r.JournalID == "" || len(r.JournalID) > 512 || r.OwnerID != strings.TrimSpace(r.OwnerID) || r.OwnerID == "" || len(r.OwnerID) > 256 || r.LeaseNanos <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow journal claim request requires canonical journal, owner and positive TTL")
	}
	return r.PolicyDigest.Validate()
}

type WorkflowJournalReleaseRequestV2 struct {
	Scope            core.ExecutionScope `json:"scope"`
	JournalID        string              `json:"journal_id"`
	OwnerID          string              `json:"owner_id"`
	Epoch            uint64              `json:"epoch"`
	ExpectedRevision core.Revision       `json:"expected_revision"`
}

func (r WorkflowJournalReleaseRequestV2) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.JournalID != strings.TrimSpace(r.JournalID) || r.JournalID == "" || len(r.JournalID) > 512 || r.OwnerID != strings.TrimSpace(r.OwnerID) || r.OwnerID == "" || len(r.OwnerID) > 256 || r.Epoch == 0 || r.ExpectedRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow journal release request requires canonical journal, owner, epoch and expected revision")
	}
	return nil
}

type WorkflowJournalListRequestV2 struct {
	Scope   core.ExecutionScope `json:"scope"`
	AfterID string              `json:"after_id,omitempty"`
	Limit   uint16              `json:"limit"`
}

func (r WorkflowJournalListRequestV2) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.AfterID != strings.TrimSpace(r.AfterID) || len(r.AfterID) > 512 || r.Limit == 0 || r.Limit > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "workflow journal list request requires canonical cursor and limit between 1 and 512")
	}
	return nil
}

// WorkflowJournalRecoveryPortV2 provides bounded discovery and CAS leasing for
// restart-safe Application workers. Implementations must partition by the full
// ExecutionScope; a claim remains only a scheduler ownership hint.
type WorkflowJournalRecoveryPortV2 interface {
	ListWorkflowJournalsV2(context.Context, core.ExecutionScope, string, uint16) ([]contract.WorkflowJournalV2, error)
	ClaimWorkflowJournalV2(context.Context, WorkflowJournalClaimRequestV2) (WorkflowJournalClaimV2, error)
	InspectWorkflowJournalClaimV2(context.Context, core.ExecutionScope, string) (WorkflowJournalClaimV2, error)
	ReleaseWorkflowJournalClaimV2(context.Context, WorkflowJournalReleaseRequestV2) (WorkflowJournalClaimV2, error)
}

type StepKindDescriptorV2 struct {
	Kind               runtimeports.NamespacedNameV2  `json:"kind"`
	Revision           core.Revision                  `json:"revision"`
	ExecutionClass     contract.StepExecutionClassV2  `json:"execution_class"`
	RequiredCapability runtimeports.CapabilityNameV2  `json:"required_capability,omitempty"`
	Contract           runtimeports.ContractBindingV2 `json:"contract"`
	Schemas            []runtimeports.SchemaRefV2     `json:"schemas"`
	IssuedUnixNano     int64                          `json:"issued_unix_nano"`
	ExpiresUnixNano    int64                          `json:"expires_unix_nano"`
}

func (d StepKindDescriptorV2) Validate() error {
	if err := runtimeports.ValidateNamespacedNameV2(d.Kind); err != nil {
		return err
	}
	if d.Revision == 0 || d.IssuedUnixNano <= 0 || d.ExpiresUnixNano <= d.IssuedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "step descriptor revision and lifetime are required")
	}
	switch d.ExecutionClass {
	case contract.StepCoordinationV2:
		if d.RequiredCapability != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "coordination descriptor cannot require a provider capability")
		}
	case contract.StepGovernedEffectV2:
		if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(d.RequiredCapability)); err != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownCapability, "governed step descriptor requires a namespaced provider capability")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "step descriptor execution class is unknown")
	}
	if err := d.Contract.Validate(); err != nil {
		return err
	}
	if len(d.Schemas) > 64 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "step descriptor schema set exceeds its bound")
	}
	previous := ""
	for _, schema := range d.Schemas {
		if err := schema.Validate(); err != nil {
			return err
		}
		key := schema.Namespace + "/" + schema.Name + "@" + schema.Version
		if key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "step descriptor schemas must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (d StepKindDescriptorV2) DigestV2() (core.Digest, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	if d.Schemas == nil {
		d.Schemas = []runtimeports.SchemaRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.application.workflow", contract.WorkflowContractVersionV2, "StepKindDescriptorV2", d)
}

func (d StepKindDescriptorV2) RefV2() (contract.StepDescriptorRefV2, error) {
	digest, err := d.DigestV2()
	if err != nil {
		return contract.StepDescriptorRefV2{}, err
	}
	return contract.StepDescriptorRefV2{Kind: d.Kind, Revision: d.Revision, Digest: digest, ExpiresUnixNano: d.ExpiresUnixNano}, nil
}

func (d StepKindDescriptorV2) ValidateCurrent(now time.Time) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, d.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "step descriptor expired")
	}
	return nil
}

// StepCatalogV2 is discovery only. A resolved descriptor never grants Binding,
// Review, Budget, Effect or dispatch authority.
type StepCatalogV2 interface {
	ResolveStepKindV2(context.Context, runtimeports.NamespacedNameV2) (StepKindDescriptorV2, error)
}
