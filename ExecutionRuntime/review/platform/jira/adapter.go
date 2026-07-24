// Package jira verifies and normalizes Jira Cloud webhook observations. Jira
// comments and issue status remain observations and never become Verdicts.
package jira

import (
	"time"

	platformcontract "github.com/Proview-China/rax/ExecutionRuntime/review/platform/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/parse"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/verify"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxSkewV1 = 5 * time.Minute

type HeadersV1 struct {
	Signature         string
	WebhookIdentifier string
	Retry             string
	Flow              string
}
type ObservationV1 struct {
	platformcontract.ObservationBaseV1
	Binding     platformcontract.EnvelopeBindingV1 `json:"binding"`
	IssueID     string                             `json:"issue_id"`
	IssueKey    string                             `json:"issue_key"`
	WebhookFlow string                             `json:"webhook_flow"`
}

func ParseObservationV1(secret []byte, tenant core.TenantID, binding platformcontract.EnvelopeBindingV1, headers HeadersV1, raw []byte, now time.Time) (ObservationV1, error) {
	if err := binding.Validate(); err != nil {
		return ObservationV1{}, err
	}
	if tenant != binding.TenantID {
		return ObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Jira envelope tenant drifted")
	}
	if err := verify.HMACSHA256(secret, raw, headers.Signature, "sha256="); err != nil {
		return ObservationV1{}, err
	}
	if headers.WebhookIdentifier == "" {
		return ObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Jira webhook identifier is missing")
	}
	if headers.Flow != "Primary" && headers.Flow != "Secondary" {
		return ObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Jira webhook flow is unsupported")
	}
	object, err := parse.DecodeObject(raw)
	if err != nil {
		return ObservationV1{}, err
	}
	timestamp, err := parse.Int64(object, "timestamp")
	if err != nil {
		return ObservationV1{}, err
	}
	if err := verify.FreshMillis(timestamp, now, MaxSkewV1); err != nil {
		return ObservationV1{}, err
	}
	event, err := parse.String(object, "webhookEvent")
	if err != nil {
		return ObservationV1{}, err
	}
	user, err := parse.ObjectField(object, "user")
	if err != nil {
		return ObservationV1{}, err
	}
	actor, err := parse.String(user, "accountId")
	if err != nil {
		return ObservationV1{}, err
	}
	issue, err := parse.ObjectField(object, "issue")
	if err != nil {
		return ObservationV1{}, err
	}
	issueID, err := parse.String(issue, "id")
	if err != nil {
		return ObservationV1{}, err
	}
	issueKey, err := parse.String(issue, "key")
	if err != nil {
		return ObservationV1{}, err
	}
	digest := core.DigestBytes(raw)
	return ObservationV1{ObservationBaseV1: platformcontract.ObservationBaseV1{ContractVersion: platformcontract.ContractVersionV1, TenantID: tenant, SourceEventID: "jira:" + headers.WebhookIdentifier, SourceSequence: uint64(timestamp), ActorHandle: actor, EventKind: event, PayloadDigest: digest, ObservedUnixNano: time.UnixMilli(timestamp).UnixNano(), ExpiresUnixNano: time.UnixMilli(timestamp).Add(MaxSkewV1).UnixNano()}, Binding: binding, IssueID: issueID, IssueKey: issueKey, WebhookFlow: headers.Flow}, nil
}
func PrepareDeliveryV1(intent platformcontract.DeliveryIntentV1) (platformcontract.DeliveryIntentV1, error) {
	intent.Platform = platformcontract.JiraV1
	return platformcontract.SealDeliveryIntentV1(intent)
}
