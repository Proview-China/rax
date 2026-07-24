// Package slack verifies and normalizes Slack-signed Review observations. It
// deliberately contains no HTTP client or provider execution point.
package slack

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	platformcontract "github.com/Proview-China/rax/ExecutionRuntime/review/platform/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/parse"
	"github.com/Proview-China/rax/ExecutionRuntime/review/platform/internal/verify"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxSkewV1 = 5 * time.Minute

type HeadersV1 struct {
	Signature        string
	RequestTimestamp string
	ContentType      string
}
type ObservationV1 struct {
	platformcontract.ObservationBaseV1
	Binding            platformcontract.EnvelopeBindingV1 `json:"binding"`
	TeamID             string                             `json:"team_id"`
	ActionID           string                             `json:"action_id"`
	ProposedResolution reviewcontract.ResolutionV1        `json:"proposed_resolution,omitempty"`
}

func ParseObservationV1(secret []byte, tenant core.TenantID, binding platformcontract.EnvelopeBindingV1, headers HeadersV1, raw []byte, now time.Time) (ObservationV1, error) {
	if err := binding.Validate(); err != nil {
		return ObservationV1{}, err
	}
	if tenant != binding.TenantID {
		return ObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Slack envelope tenant drifted")
	}
	seconds, err := verify.FreshSeconds(headers.RequestTimestamp, now, MaxSkewV1)
	if err != nil {
		return ObservationV1{}, err
	}
	base := []byte("v0:" + headers.RequestTimestamp + ":")
	base = append(base, raw...)
	if err := verify.HMACSHA256(secret, base, headers.Signature, "v0="); err != nil {
		return ObservationV1{}, err
	}
	payload := raw
	if strings.HasPrefix(strings.ToLower(headers.ContentType), "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(string(raw))
		if err != nil || len(values["payload"]) != 1 {
			return ObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Slack webhook form payload is invalid")
		}
		payload = []byte(values["payload"][0])
	}
	object, err := parse.DecodeObject(payload)
	if err != nil {
		return ObservationV1{}, err
	}
	kind, err := parse.String(object, "type")
	if err != nil {
		return ObservationV1{}, err
	}
	user, err := parse.ObjectField(object, "user")
	if err != nil {
		return ObservationV1{}, err
	}
	actor, err := parse.String(user, "id")
	if err != nil {
		return ObservationV1{}, err
	}
	team, err := parse.ObjectField(object, "team")
	if err != nil {
		return ObservationV1{}, err
	}
	teamID, err := parse.String(team, "id")
	if err != nil {
		return ObservationV1{}, err
	}
	action, err := parse.FirstObject(object, "actions")
	if err != nil {
		return ObservationV1{}, err
	}
	actionID, err := parse.String(action, "action_id")
	if err != nil {
		return ObservationV1{}, err
	}
	resolution := map[string]reviewcontract.ResolutionV1{"praxis_review_accept": reviewcontract.ResolutionAcceptV1, "praxis_review_reject": reviewcontract.ResolutionRejectV1, "praxis_review_request_changes": reviewcontract.ResolutionRequestChangesV1, "praxis_review_insufficient_evidence": reviewcontract.ResolutionInsufficientEvidenceV1}[actionID]
	if resolution == "" {
		return ObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Slack action is not a Review decision action")
	}
	digest := core.DigestBytes(raw)
	eventID := "slack:" + strconv.FormatInt(seconds, 10) + ":" + string(digest)
	return ObservationV1{ObservationBaseV1: platformcontract.ObservationBaseV1{ContractVersion: platformcontract.ContractVersionV1, TenantID: tenant, SourceEventID: eventID, SourceSequence: uint64(seconds), ActorHandle: actor, EventKind: kind, PayloadDigest: digest, ObservedUnixNano: time.Unix(seconds, 0).UnixNano(), ExpiresUnixNano: time.Unix(seconds, 0).Add(MaxSkewV1).UnixNano()}, Binding: binding, TeamID: teamID, ActionID: actionID, ProposedResolution: resolution}, nil
}

func PrepareDeliveryV1(intent platformcontract.DeliveryIntentV1) (platformcontract.DeliveryIntentV1, error) {
	intent.Platform = platformcontract.SlackV1
	return platformcontract.SealDeliveryIntentV1(intent)
}
