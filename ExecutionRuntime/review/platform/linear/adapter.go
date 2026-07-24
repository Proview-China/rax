// Package linear verifies and normalizes Linear webhook observations. Free
// text and issue state never become a proposed Review resolution here.
package linear

import (
	"strconv"
	"time"

	platformcontract "github.com/Proview-China/rax/ExecutionRuntime/review/platform/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/parse"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/verify"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxSkewV1 = time.Minute

type HeadersV1 struct {
	Signature string
	Delivery  string
	Event     string
	Timestamp string
}
type ObservationV1 struct {
	platformcontract.ObservationBaseV1
	Binding        platformcontract.EnvelopeBindingV1 `json:"binding"`
	OrganizationID string                             `json:"organization_id"`
	EntityID       string                             `json:"entity_id"`
	Action         string                             `json:"action"`
}

func ParseObservationV1(secret []byte, tenant core.TenantID, binding platformcontract.EnvelopeBindingV1, headers HeadersV1, raw []byte, now time.Time) (ObservationV1, error) {
	if err := binding.Validate(); err != nil {
		return ObservationV1{}, err
	}
	if tenant != binding.TenantID {
		return ObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Linear envelope tenant drifted")
	}
	if err := verify.HMACSHA256(secret, raw, headers.Signature, ""); err != nil {
		return ObservationV1{}, err
	}
	object, err := parse.DecodeObject(raw)
	if err != nil {
		return ObservationV1{}, err
	}
	timestamp, err := parse.Int64(object, "webhookTimestamp")
	if err != nil {
		return ObservationV1{}, err
	}
	if err := verify.FreshMillis(timestamp, now, MaxSkewV1); err != nil {
		return ObservationV1{}, err
	}
	headerTimestamp, err := strconv.ParseInt(headers.Timestamp, 10, 64)
	if err != nil || headerTimestamp != timestamp {
		return ObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Linear header/body timestamp drifted")
	}
	if headers.Delivery == "" || headers.Event == "" {
		return ObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Linear delivery headers are incomplete")
	}
	actorObject, err := parse.ObjectField(object, "actor")
	if err != nil {
		return ObservationV1{}, err
	}
	actor, err := parse.String(actorObject, "id")
	if err != nil {
		return ObservationV1{}, err
	}
	data, err := parse.ObjectField(object, "data")
	if err != nil {
		return ObservationV1{}, err
	}
	entityID, err := parse.String(data, "id")
	if err != nil {
		return ObservationV1{}, err
	}
	organizationID, err := parse.String(object, "organizationId")
	if err != nil {
		return ObservationV1{}, err
	}
	action, err := parse.String(object, "action")
	if err != nil {
		return ObservationV1{}, err
	}
	digest := core.DigestBytes(raw)
	return ObservationV1{ObservationBaseV1: platformcontract.ObservationBaseV1{ContractVersion: platformcontract.ContractVersionV1, TenantID: tenant, SourceEventID: "linear:" + headers.Delivery, SourceSequence: uint64(timestamp), ActorHandle: actor, EventKind: headers.Event, PayloadDigest: digest, ObservedUnixNano: time.UnixMilli(timestamp).UnixNano(), ExpiresUnixNano: time.UnixMilli(timestamp).Add(MaxSkewV1).UnixNano()}, Binding: binding, OrganizationID: organizationID, EntityID: entityID, Action: action}, nil
}
func PrepareDeliveryV1(intent platformcontract.DeliveryIntentV1) (platformcontract.DeliveryIntentV1, error) {
	intent.Platform = platformcontract.LinearV1
	return platformcontract.SealDeliveryIntentV1(intent)
}
