package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationProviderReceiptObservationContractVersionV1 = "1.0.0"
	OperationProviderReceiptEvidenceSourceIDV1           = NamespacedNameV2("praxis.runtime/provider-receipt-source")
	OperationProviderReceiptEvidenceKindV1               = NamespacedNameV2("praxis.runtime/provider-receipt")
	OperationProviderReceiptEvidenceClassV1              = NamespacedNameV2("praxis.runtime/provider-observation")
)

// OperationProviderReceiptRefV1 is a Runtime-neutral exact reference to a
// provider/domain-owned receipt. It is an Observation source, never authority,
// a Review verdict, a DomainResult or a Settlement.
type OperationProviderReceiptRefV1 struct {
	Owner    EffectOwnerRefV2 `json:"owner"`
	Kind     NamespacedNameV2 `json:"kind"`
	ID       string           `json:"id"`
	Revision core.Revision    `json:"revision"`
	Digest   core.Digest      `json:"digest"`
}

func (r OperationProviderReceiptRefV1) Validate() error {
	if r.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(r.Owner.ComponentID)) != nil || r.Owner.ManifestDigest.Validate() != nil || ValidateNamespacedNameV2(r.Kind) != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation Provider receipt ref is incomplete")
	}
	return nil
}

// OperationProviderReceiptProjectionV1 is an exact historical receipt
// projection. CheckedUnixNano proves when the exact owner store was read; it
// intentionally has no TTL and grants no execution authority.
type OperationProviderReceiptProjectionV1 struct {
	ContractVersion      string                        `json:"contract_version"`
	Ref                  OperationProviderReceiptRefV1 `json:"ref"`
	Operation            OperationSubjectV3            `json:"operation"`
	OperationDigest      core.Digest                   `json:"operation_digest"`
	Prepared             PreparedProviderAttemptRefV2  `json:"prepared"`
	Attempt              OperationDispatchAttemptRefV3 `json:"attempt"`
	Provider             ProviderBindingRefV2          `json:"provider"`
	ProviderOperationRef string                        `json:"provider_operation_ref"`
	Payload              OpaquePayloadV2               `json:"payload"`
	PayloadRevision      core.Revision                 `json:"payload_revision"`
	ObservedUnixNano     int64                         `json:"observed_unix_nano"`
	CheckedUnixNano      int64                         `json:"checked_unix_nano"`
	ProjectionDigest     core.Digest                   `json:"projection_digest"`
}

func (p OperationProviderReceiptProjectionV1) Validate() error {
	if p.ContractVersion != OperationProviderReceiptObservationContractVersionV1 || p.Ref.Validate() != nil || p.Operation.Validate() != nil || p.OperationDigest.Validate() != nil || p.Prepared.Validate() != nil || p.Attempt.Validate() != nil || p.Provider.Validate() != nil || validateEvidenceIDV2(p.ProviderOperationRef) != nil || p.Payload.Validate() != nil || p.PayloadRevision == 0 || p.ObservedUnixNano <= 0 || p.CheckedUnixNano < p.ObservedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "operation Provider receipt projection is incomplete")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || p.ProviderOperationRef != p.Ref.ID || p.Prepared.OperationDigest != p.OperationDigest || p.Attempt.OperationDigest != p.OperationDigest || p.Prepared.AttemptID != p.Attempt.AttemptID || p.Prepared.Provider != p.Provider || p.Prepared.IntentID != p.Attempt.EffectID || p.Prepared.IntentRevision != p.Attempt.IntentRevision || p.Prepared.IntentDigest != p.Attempt.IntentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "operation Provider receipt projection binds another operation, attempt or Provider")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "operation Provider receipt projection digest drifted")
	}
	return nil
}

func (p OperationProviderReceiptProjectionV1) ValidateExact(expected OperationProviderReceiptRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "operation Provider receipt exact ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation Provider receipt inspection clock regressed")
	}
	return nil
}

func (p OperationProviderReceiptProjectionV1) DigestV1() (core.Digest, error) {
	p.Payload.Inline = append([]byte(nil), p.Payload.Inline...)
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-provider-receipt", OperationProviderReceiptObservationContractVersionV1, "OperationProviderReceiptProjectionV1", p)
}

func SealOperationProviderReceiptProjectionV1(p OperationProviderReceiptProjectionV1) (OperationProviderReceiptProjectionV1, error) {
	p.ContractVersion = OperationProviderReceiptObservationContractVersionV1
	p.Payload.Inline = append([]byte(nil), p.Payload.Inline...)
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return OperationProviderReceiptProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return OperationProviderReceiptProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supplied operation Provider receipt projection digest drifted")
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type OperationProviderReceiptReaderV1 interface {
	InspectOperationProviderReceiptV1(context.Context, OperationProviderReceiptRefV1) (OperationProviderReceiptProjectionV1, error)
}

// EvidenceSourceRegistrationReaderV1 is the narrow read-only source seam used
// by the receipt coordinator. It grants neither registration nor append.
type EvidenceSourceRegistrationReaderV1 interface {
	InspectSource(context.Context, string) (EvidenceSourceRegistrationFactV2, error)
}

// OperationProviderReceiptObservationRequestV1 uses one dedicated Evidence V2
// source per Prepared attempt. Revision one and source sequence one are fixed;
// retries inspect that same source key and never allocate a new sequence.
type OperationProviderReceiptObservationRequestV1 struct {
	Intent             OperationEffectIntentV3         `json:"intent"`
	Permit             OperationDispatchPermitV3       `json:"permit"`
	Fence              core.ExecutionFence             `json:"fence"`
	Attempt            GovernedExecutionAttemptRefsV2  `json:"attempt"`
	Receipt            OperationProviderReceiptRefV1   `json:"receipt"`
	SourceRegistration EvidenceSourceRegistrationRefV1 `json:"source_registration"`
}

func (r OperationProviderReceiptObservationRequestV1) Validate() error {
	if r.Receipt.Validate() != nil || r.SourceRegistration.Validate() != nil || r.SourceRegistration.Revision != 1 || r.SourceRegistration.SourceID != OperationProviderReceiptEvidenceSourceIDV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation Provider receipt observation request references are incomplete")
	}
	execute := ExecutePreparedRequestV2{Delegation: r.Attempt.Delegation, Prepared: r.Attempt.Prepared, Enforcement: r.Attempt.Enforcement, Intent: r.Intent, Permit: r.Permit, Fence: r.Fence}
	if err := execute.Validate(); err != nil {
		return err
	}
	if r.Receipt.Owner.ComponentID != ComponentIDV2(r.Attempt.Prepared.Provider.ComponentID) || r.Receipt.Owner.ManifestDigest != r.Attempt.Prepared.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "operation Provider receipt Owner differs from the Prepared Provider")
	}
	return nil
}

type OperationProviderReceiptObservationPortV1 interface {
	RecordOperationProviderReceiptObservationV1(context.Context, OperationProviderReceiptObservationRequestV1) (ProviderAttemptObservationRefV2, error)
	InspectOperationProviderReceiptObservationV1(context.Context, ExecutionDelegationRefV2, string) (ProviderAttemptObservationRefV2, error)
}
