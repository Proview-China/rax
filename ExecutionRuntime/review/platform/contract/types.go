// Package platformcontract defines Review-owned, provider-neutral delivery
// candidates. They are not provider calls, receipts, attestations or Verdicts.
package platformcontract

import (
	"sort"
	"strings"
	"time"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ContractVersionV1 = "praxis.review/platform-v1"

type KindV1 string

const (
	SlackV1  KindV1 = "slack"
	LinearV1 KindV1 = "linear"
	JiraV1   KindV1 = "jira"
)

type HumanReviewEnvelopeV1 struct {
	ContractVersion    string                            `json:"contract_version"`
	TenantID           core.TenantID                     `json:"tenant_id"`
	EnvelopeID         string                            `json:"envelope_id"`
	Case               reviewcontract.ExactResourceRefV1 `json:"case"`
	Target             reviewcontract.ExactResourceRefV1 `json:"target"`
	Title              string                            `json:"title"`
	Summary            string                            `json:"summary"`
	DeepLink           string                            `json:"deep_link"`
	AllowedResolutions []reviewcontract.ResolutionV1     `json:"allowed_resolutions"`
	CreatedUnixNano    int64                             `json:"created_unix_nano"`
	ExpiresUnixNano    int64                             `json:"expires_unix_nano"`
	Digest             core.Digest                       `json:"digest"`
}

func (v HumanReviewEnvelopeV1) digestValue() HumanReviewEnvelopeV1 { v.Digest = ""; return v }
func (v HumanReviewEnvelopeV1) validateShape() error {
	if v.ContractVersion != ContractVersionV1 || strings.TrimSpace(string(v.TenantID)) == "" || strings.TrimSpace(v.EnvelopeID) == "" || len(v.EnvelopeID) > 512 || strings.TrimSpace(v.Title) == "" || len(v.Title) > 512 || strings.TrimSpace(v.Summary) == "" || len(v.Summary) > reviewcontract.MaxStringBytesV1 || strings.TrimSpace(v.DeepLink) == "" || len(v.DeepLink) > reviewcontract.MaxStringBytesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human review envelope is incomplete")
	}
	if err := v.Case.Validate(); err != nil {
		return err
	}
	if err := v.Target.Validate(); err != nil {
		return err
	}
	if len(v.AllowedResolutions) == 0 || len(v.AllowedResolutions) > 6 || !sort.SliceIsSorted(v.AllowedResolutions, func(i, j int) bool { return v.AllowedResolutions[i] < v.AllowedResolutions[j] }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human review resolutions must be sorted and bounded")
	}
	for i, resolution := range v.AllowedResolutions {
		if i > 0 && v.AllowedResolutions[i-1] == resolution {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human review resolution is duplicated")
		}
		switch resolution {
		case reviewcontract.ResolutionAcceptV1, reviewcontract.ResolutionConditionalV1, reviewcontract.ResolutionRequestChangesV1, reviewcontract.ResolutionEscalateHumanV1, reviewcontract.ResolutionRejectV1, reviewcontract.ResolutionInsufficientEvidenceV1:
		default:
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human review resolution is unsupported")
		}
	}
	if v.CreatedUnixNano <= 0 || v.ExpiresUnixNano <= v.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human review envelope TTL is invalid")
	}
	return nil
}

func SealHumanReviewEnvelopeV1(v HumanReviewEnvelopeV1) (HumanReviewEnvelopeV1, error) {
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	sort.Slice(v.AllowedResolutions, func(i, j int) bool { return v.AllowedResolutions[i] < v.AllowedResolutions[j] })
	if err := v.validateShape(); err != nil {
		return HumanReviewEnvelopeV1{}, err
	}
	digest, err := core.CanonicalJSONDigest("praxis.review.platform", ContractVersionV1, "HumanReviewEnvelopeV1", v.digestValue())
	if err != nil {
		return HumanReviewEnvelopeV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}
func (v HumanReviewEnvelopeV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	expected, err := core.CanonicalJSONDigest("praxis.review.platform", ContractVersionV1, "HumanReviewEnvelopeV1", v.digestValue())
	if err != nil || expected != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human review envelope digest drifted")
	}
	return nil
}
func (v HumanReviewEnvelopeV1) ValidateCurrent(now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "human review envelope clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human review envelope expired")
	}
	return nil
}

type DeliveryIntentV1 struct {
	ContractVersion string                `json:"contract_version"`
	TenantID        core.TenantID         `json:"tenant_id"`
	ID              string                `json:"id"`
	Revision        core.Revision         `json:"revision"`
	Platform        KindV1                `json:"platform"`
	DestinationRef  string                `json:"destination_ref"`
	Envelope        HumanReviewEnvelopeV1 `json:"envelope"`
	IdempotencyKey  string                `json:"idempotency_key"`
	CreatedUnixNano int64                 `json:"created_unix_nano"`
	ExpiresUnixNano int64                 `json:"expires_unix_nano"`
	Digest          core.Digest           `json:"digest"`
}

// EnvelopeBindingV1 is the immutable, host-resolved association used when a
// provider event is normalized. The raw provider payload cannot choose another
// Review Case or Target merely because its signature is valid.
type EnvelopeBindingV1 struct {
	TenantID       core.TenantID                     `json:"tenant_id"`
	EnvelopeID     string                            `json:"envelope_id"`
	EnvelopeDigest core.Digest                       `json:"envelope_digest"`
	Revision       core.Revision                     `json:"revision"`
	Case           reviewcontract.ExactResourceRefV1 `json:"case"`
	Target         reviewcontract.ExactResourceRefV1 `json:"target"`
}

func BindingFromEnvelopeV1(value HumanReviewEnvelopeV1) (EnvelopeBindingV1, error) {
	if err := value.Validate(); err != nil {
		return EnvelopeBindingV1{}, err
	}
	return EnvelopeBindingV1{TenantID: value.TenantID, EnvelopeID: value.EnvelopeID, EnvelopeDigest: value.Digest, Revision: 1, Case: value.Case, Target: value.Target}, nil
}

func (v EnvelopeBindingV1) Validate() error {
	if strings.TrimSpace(string(v.TenantID)) == "" || strings.TrimSpace(v.EnvelopeID) == "" || v.Revision != 1 || v.EnvelopeDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human review envelope binding is incomplete")
	}
	if err := v.Case.Validate(); err != nil {
		return err
	}
	return v.Target.Validate()
}

func (v DeliveryIntentV1) digestValue() DeliveryIntentV1 { v.Digest = ""; return v }
func (v DeliveryIntentV1) validateShape() error {
	if v.ContractVersion != ContractVersionV1 || v.TenantID == "" || strings.TrimSpace(v.ID) == "" || v.Revision != 1 || strings.TrimSpace(v.DestinationRef) == "" || strings.TrimSpace(v.IdempotencyKey) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform delivery intent is incomplete")
	}
	switch v.Platform {
	case SlackV1, LinearV1, JiraV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "platform delivery kind is unsupported")
	}
	if err := v.Envelope.Validate(); err != nil {
		return err
	}
	if v.TenantID != v.Envelope.TenantID || v.CreatedUnixNano < v.Envelope.CreatedUnixNano || v.ExpiresUnixNano > v.Envelope.ExpiresUnixNano || v.ExpiresUnixNano <= v.CreatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "platform delivery intent envelope or TTL drifted")
	}
	return nil
}
func SealDeliveryIntentV1(v DeliveryIntentV1) (DeliveryIntentV1, error) {
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	if err := v.validateShape(); err != nil {
		return DeliveryIntentV1{}, err
	}
	digest, err := core.CanonicalJSONDigest("praxis.review.platform", ContractVersionV1, "DeliveryIntentV1", v.digestValue())
	if err != nil {
		return DeliveryIntentV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}
func (v DeliveryIntentV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	expected, err := core.CanonicalJSONDigest("praxis.review.platform", ContractVersionV1, "DeliveryIntentV1", v.digestValue())
	if err != nil || expected != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "platform delivery intent digest drifted")
	}
	return nil
}

type ObservationBaseV1 struct {
	ContractVersion  string        `json:"contract_version"`
	TenantID         core.TenantID `json:"tenant_id"`
	SourceEventID    string        `json:"source_event_id"`
	SourceSequence   uint64        `json:"source_sequence"`
	ActorHandle      string        `json:"actor_handle"`
	EventKind        string        `json:"event_kind"`
	PayloadDigest    core.Digest   `json:"payload_digest"`
	ObservedUnixNano int64         `json:"observed_unix_nano"`
	ExpiresUnixNano  int64         `json:"expires_unix_nano"`
}

func (v ObservationBaseV1) Validate(now time.Time) error {
	if v.ContractVersion != ContractVersionV1 || v.TenantID == "" || strings.TrimSpace(v.SourceEventID) == "" || v.SourceSequence == 0 || strings.TrimSpace(v.ActorHandle) == "" || strings.TrimSpace(v.EventKind) == "" || v.PayloadDigest.Validate() != nil || v.ObservedUnixNano <= 0 || v.ExpiresUnixNano <= v.ObservedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform observation is incomplete")
	}
	if now.IsZero() || now.UnixNano() < v.ObservedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "platform observation clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "platform observation expired")
	}
	return nil
}
