package contract

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ContinuityWorkflowContractVersionV1 = "praxis.application.continuity-workflow/v1"

type ContinuityWorkflowKindV1 string

const (
	ContinuityTimelineProjectV1  ContinuityWorkflowKindV1 = "praxis.continuity/timeline-project"
	ContinuityCheckpointCreateV1 ContinuityWorkflowKindV1 = "praxis.continuity/checkpoint-create"
	ContinuityForkV1             ContinuityWorkflowKindV1 = "praxis.continuity/fork"
	ContinuityRewindPlanV1       ContinuityWorkflowKindV1 = "praxis.continuity/rewind-plan"
	ContinuityRestoreV1          ContinuityWorkflowKindV1 = "praxis.continuity/restore"
	ContinuityArtifactAttachV1   ContinuityWorkflowKindV1 = "praxis.continuity/artifact-attach"
	ContinuityRetentionResolveV1 ContinuityWorkflowKindV1 = "praxis.continuity/retention-resolve"
)

type ExternalOwnerBindingV1 struct {
	BindingSetID       string                        `json:"binding_set_id"`
	BindingSetRevision core.Revision                 `json:"binding_set_revision"`
	ComponentID        runtimeports.ComponentIDV2    `json:"component_id"`
	ManifestDigest     core.Digest                   `json:"manifest_digest"`
	ArtifactDigest     core.Digest                   `json:"artifact_digest"`
	Capability         runtimeports.CapabilityNameV2 `json:"capability"`
	FactKind           runtimeports.NamespacedNameV2 `json:"fact_kind"`
}

func (o ExternalOwnerBindingV1) Validate() error {
	if o.BindingSetID == "" || o.BindingSetID != strings.TrimSpace(o.BindingSetID) || len(o.BindingSetID) > 256 || o.BindingSetRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "external owner binding set identity and revision are required")
	}
	for _, name := range []runtimeports.NamespacedNameV2{runtimeports.NamespacedNameV2(o.ComponentID), runtimeports.NamespacedNameV2(o.Capability), o.FactKind} {
		if err := runtimeports.ValidateNamespacedNameV2(name); err != nil {
			return err
		}
	}
	if err := o.ManifestDigest.Validate(); err != nil {
		return err
	}
	return o.ArtifactDigest.Validate()
}

// ExternalFactRefV1 is an opaque, fully exact cross-owner coordinate.
// Application validates its shape and target-scope binding but never
// interprets or upgrades the referenced domain fact.
type ExternalFactRefV1 struct {
	ContractVersion string                 `json:"contract_version"`
	SchemaRef       string                 `json:"schema_ref"`
	Owner           ExternalOwnerBindingV1 `json:"owner"`
	TenantID        core.TenantID          `json:"tenant_id"`
	ScopeDigest     core.Digest            `json:"scope_digest"`
	ID              string                 `json:"id"`
	Revision        core.Revision          `json:"revision"`
	Digest          core.Digest            `json:"digest"`
}

func (r ExternalFactRefV1) Validate() error {
	if r.ContractVersion == "" || r.ContractVersion != strings.TrimSpace(r.ContractVersion) || len(r.ContractVersion) > 128 || r.SchemaRef == "" || r.SchemaRef != strings.TrimSpace(r.SchemaRef) || len(r.SchemaRef) > 256 || strings.TrimSpace(string(r.TenantID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "external fact contract, schema and tenant are required")
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if err := r.ScopeDigest.Validate(); err != nil {
		return err
	}
	if r.ID == "" || r.ID != strings.TrimSpace(r.ID) || len(r.ID) > 512 || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "external fact ref requires canonical identity and revision")
	}
	return r.Digest.Validate()
}

// ContinuityWorkflowRequestV1 contains coordinates only. In particular it has
// no Permit, Review verdict, provider binding, Runtime outcome, trusted current
// flag, sequence, payload body, or raw SubmissionBundle field.
type ContinuityWorkflowRequestV1 struct {
	ContractVersion   string                   `json:"contract_version"`
	RequestID         string                   `json:"request_id"`
	IdempotencyKey    string                   `json:"idempotency_key"`
	Kind              ContinuityWorkflowKindV1 `json:"kind"`
	Target            core.ExecutionScope      `json:"target"`
	DomainRequest     ExternalFactRefV1        `json:"domain_request"`
	ExpectedRevision  core.Revision            `json:"expected_revision,omitempty"`
	CompiledGraph     ApplicationFactRefV2     `json:"compiled_graph"`
	Binding           ApplicationFactRefV2     `json:"binding"`
	Consumer          ApplicationFactRefV2     `json:"consumer"`
	RequestedUnixNano int64                    `json:"requested_unix_nano"`
	NotAfterUnixNano  int64                    `json:"not_after_unix_nano"`
}

func (r ContinuityWorkflowRequestV1) Validate(now time.Time) error {
	if r.ContractVersion != ContinuityWorkflowContractVersionV1 || r.RequestID == "" || r.RequestID != strings.TrimSpace(r.RequestID) || len(r.RequestID) > 256 || r.IdempotencyKey == "" || r.IdempotencyKey != strings.TrimSpace(r.IdempotencyKey) || len(r.IdempotencyKey) > 256 || !validContinuityWorkflowKindV1(r.Kind) || r.RequestedUnixNano <= 0 || r.NotAfterUnixNano <= r.RequestedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "continuity workflow request identity, kind and bounded lifetime are required")
	}
	if !now.IsZero() && !now.Before(time.Unix(0, r.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "continuity workflow request expired")
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.Target)
	if err != nil {
		return err
	}
	if r.DomainRequest.TenantID != r.Target.Identity.TenantID || r.DomainRequest.ScopeDigest != scopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "domain request tenant or execution scope differs from target")
	}
	for _, ref := range []ApplicationFactRefV2{r.CompiledGraph, r.Binding, r.Consumer} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return r.DomainRequest.Validate()
}

func (r ContinuityWorkflowRequestV1) CanonicalBodyV1() ([]byte, error) {
	if err := r.Validate(time.Time{}); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "continuity workflow request cannot be encoded")
	}
	return payload, nil
}

func (r ContinuityWorkflowRequestV1) DigestV1() (core.Digest, error) {
	if err := r.Validate(time.Time{}); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.continuity-workflow", ContinuityWorkflowContractVersionV1, "ContinuityWorkflowRequestV1", r)
}

type ContinuityWorkflowAssemblyV1 struct {
	RequestDigest core.Digest                         `json:"request_digest"`
	RootStepID    string                              `json:"root_step_id"`
	Bundle        SubmissionBundleV2                  `json:"bundle"`
	Mutation      runtimeports.DesiredStateMutationV2 `json:"mutation"`
}

func (a ContinuityWorkflowAssemblyV1) ValidateFor(request ContinuityWorkflowRequestV1, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	expectedDigest, err := request.DigestV1()
	if err != nil || a.RequestDigest != expectedDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "assembly request digest differs")
	}
	if a.RootStepID == "" || a.RootStepID != strings.TrimSpace(a.RootStepID) || len(a.RootStepID) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembly root step id is required")
	}
	if err := a.Bundle.Validate(now); err != nil {
		return err
	}
	if err := a.Mutation.ValidateFor(a.Bundle.Command.Kind); err != nil {
		return err
	}
	return ValidateContinuitySubmissionV1(request, a.Bundle, a.RootStepID, now)
}

// ValidateContinuitySubmissionV1 proves that a persisted Application bundle
// contains the exact canonical high-level request. It deliberately does not
// re-run current readers or reconstruct an assembler decision during recovery.
func ValidateContinuitySubmissionV1(request ContinuityWorkflowRequestV1, bundle SubmissionBundleV2, rootStepID string, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	if rootStepID == "" || rootStepID != strings.TrimSpace(rootStepID) || len(rootStepID) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembly root step id is required")
	}
	if err := bundle.Validate(now); err != nil {
		return err
	}
	command := bundle.Command
	if command.ID != request.RequestID || command.IdempotencyKey != request.IdempotencyKey || command.Kind != runtimeports.ApplicationCommandProvideInputV2 || !runtimeports.SameExecutionScopeV2(command.Target, request.Target) || command.SubmittedAt.UnixNano() != request.RequestedUnixNano || command.ExpiresAt.UnixNano() > request.NotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "assembly command does not exactly bind the request")
	}
	body, err := request.CanonicalBodyV1()
	if err != nil {
		return err
	}
	if !sameInlineRequestPayloadV1(bundle.Payload.Payload, body) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "assembly command payload does not contain the canonical request")
	}
	found := false
	for _, step := range bundle.Plan.Steps {
		if step.ID != rootStepID {
			continue
		}
		found = true
		if ContinuityWorkflowKindV1(step.Kind) != request.Kind || !sameInlineRequestPayloadV1(step.Payload, body) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "assembly root step does not exactly bind the request")
		}
	}
	if !found {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "assembly root step is absent")
	}
	return nil
}

type ContinuityWorkflowStepRefV1 struct {
	StepID     string                        `json:"step_id"`
	Kind       runtimeports.NamespacedNameV2 `json:"kind"`
	Descriptor StepDescriptorRefV2           `json:"descriptor"`
}

type ContinuityWorkflowInspectionV1 struct {
	RequestDigest core.Digest                   `json:"request_digest"`
	Submission    ApplicationFactRefV2          `json:"submission"`
	Command       ApplicationFactRefV2          `json:"command"`
	Outbox        ApplicationFactRefV2          `json:"outbox"`
	Plan          ApplicationFactRefV2          `json:"plan"`
	Journal       *ApplicationFactRefV2         `json:"journal,omitempty"`
	Status        WorkflowStatusV2              `json:"status"`
	Steps         []ContinuityWorkflowStepRefV1 `json:"steps"`
}

func (r ContinuityWorkflowInspectionV1) ValidateFor(request ContinuityWorkflowRequestV1) error {
	expected, err := request.DigestV1()
	if err != nil || r.RequestDigest != expected {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow inspection request digest differs")
	}
	for _, ref := range []ApplicationFactRefV2{r.Submission, r.Command, r.Outbox, r.Plan} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if r.Submission.Ref != request.RequestID || r.Command.Ref != request.RequestID || r.Outbox.Ref != request.RequestID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "workflow inspection coordinates another request")
	}
	if r.Journal != nil {
		if err := r.Journal.Validate(); err != nil {
			return err
		}
	}
	switch r.Status {
	case WorkflowAcceptedV2, WorkflowDispatchingV2, WorkflowWaitingInspectV2, WorkflowCompletedV2, WorkflowIndeterminateV2:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "workflow inspection status is unknown")
	}
	if len(r.Steps) == 0 || len(r.Steps) > MaxWorkflowStepsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "workflow inspection requires bounded step refs")
	}
	previous := ""
	for _, step := range r.Steps {
		if step.StepID == "" || step.StepID != strings.TrimSpace(step.StepID) || step.StepID <= previous || runtimeports.ValidateNamespacedNameV2(step.Kind) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "workflow inspection step refs must be sorted and unique")
		}
		if err := step.Descriptor.Validate(step.Kind); err != nil {
			return err
		}
		previous = step.StepID
	}
	return nil
}

func SubmissionBundleDigestV1(bundle SubmissionBundleV2) (core.Digest, error) {
	if err := bundle.Validate(time.Time{}); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.continuity-workflow", ContinuityWorkflowContractVersionV1, "SubmissionBundleV2", bundle)
}

func validContinuityWorkflowKindV1(kind ContinuityWorkflowKindV1) bool {
	switch kind {
	case ContinuityTimelineProjectV1, ContinuityCheckpointCreateV1, ContinuityForkV1, ContinuityRewindPlanV1, ContinuityRestoreV1, ContinuityArtifactAttachV1, ContinuityRetentionResolveV1:
		return true
	default:
		return false
	}
}

func sameInlineRequestPayloadV1(payload runtimeports.OpaquePayloadV2, body []byte) bool {
	return payload.Validate() == nil && payload.Ref == "" && string(payload.Inline) == string(body) && payload.ContentDigest == core.DigestBytes(body)
}
