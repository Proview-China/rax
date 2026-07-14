package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type OperationIntentAdmissionFactV3 struct {
	Operation        OperationSubjectV3           `json:"operation_subject"`
	IntentID         core.EffectIntentID          `json:"intent_id"`
	IntentRevision   core.Revision                `json:"intent_revision"`
	IntentDigest     core.Digest                  `json:"intent_digest"`
	IntentOwner      EffectOwnerRefV2             `json:"intent_owner"`
	Provider         ProviderBindingRefV2         `json:"provider_binding"`
	Binding          OperationGovernanceFactRefV3 `json:"binding_fact"`
	OwnerAttestation OperationGovernanceFactRefV3 `json:"owner_attestation"`
	Active           bool                         `json:"active"`
	ExpiresUnixNano  int64                        `json:"expires_unix_nano"`
}

func (f OperationIntentAdmissionFactV3) ValidateCurrent(intent OperationEffectIntentV3, now time.Time) error {
	if !f.Active || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || !SameOperationSubjectV3(f.Operation, intent.Operation) || f.IntentID != intent.ID || f.IntentRevision != intent.Revision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "operation Intent admission projection is inactive, expired or drifted")
	}
	intentDigest, err := intent.DigestV3()
	if err != nil || f.IntentDigest != intentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Intent admission digest drifted")
	}
	if err := f.Binding.Validate(now); err != nil {
		return err
	}
	if err := f.OwnerAttestation.Validate(now); err != nil {
		return err
	}
	if f.Provider != intent.Provider || f.Binding.Ref != intent.Provider.BindingSetID || f.Binding.Revision != intent.Provider.BindingSetRevision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "operation Intent provider or BindingSet drifted")
	}
	found := false
	for _, owner := range intent.Owners {
		if owner.Role == OwnerEffect {
			found = true
			if owner != f.IntentOwner {
				return core.NewError(core.ErrorForbidden, core.ReasonOwnerConflict, "operation Intent Owner attestation binds another owner")
			}
		}
	}
	if !found {
		return core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "operation Intent has no bound Intent Owner")
	}
	return nil
}

type OperationIntentAdmissionReaderV3 interface {
	// InspectOperationIntentAdmission reconstructs current owner/binding facts;
	// it must not trust a caller-provided attestation.
	InspectOperationIntentAdmission(context.Context, OperationEffectIntentV3) (OperationIntentAdmissionFactV3, error)
}

type OperationEffectAdmissionReceiptV3 struct {
	OperationDigest core.Digest         `json:"operation_digest"`
	EffectID        core.EffectIntentID `json:"effect_id"`
	IntentRevision  core.Revision       `json:"intent_revision"`
	IntentDigest    core.Digest         `json:"intent_digest"`
	FactRevision    core.Revision       `json:"fact_revision"`
	State           string              `json:"state"`
}

func (r OperationEffectAdmissionReceiptV3) Validate() error {
	if r.OperationDigest.Validate() != nil || r.IntentDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.IntentRevision == 0 || r.FactRevision == 0 || r.State != "accepted" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "accepted operation Effect receipt is incomplete")
	}
	return nil
}

type OperationEffectAdmissionPortV3 interface {
	AdmitOperationEffectV3(context.Context, OperationEffectIntentV3) (OperationEffectAdmissionReceiptV3, error)
	InspectAcceptedOperationEffectV3(context.Context, OperationSubjectV3, core.EffectIntentID) (OperationEffectAdmissionReceiptV3, error)
}
