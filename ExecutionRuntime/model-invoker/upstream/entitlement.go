package upstream

import "time"

type EntitlementStatus string

const (
	EntitlementActive         EntitlementStatus = "active"
	EntitlementQuotaExhausted EntitlementStatus = "quota_exhausted"
	EntitlementExpired        EntitlementStatus = "expired"
	EntitlementSuspended      EntitlementStatus = "suspended"
)

// EntitlementState is an account-local snapshot. It contains no credential
// value and is useful only during its explicit observation window.
type EntitlementState struct {
	OfferingID        OfferingID          `json:"offering_id"`
	CredentialProfile CredentialProfileID `json:"credential_profile_id"`
	Status            EntitlementStatus   `json:"status"`
	ObservedAt        time.Time           `json:"observed_at"`
	ValidUntil        time.Time           `json:"valid_until"`
	ExpiresAt         time.Time           `json:"expires_at,omitempty"`
	RemainingQuota    *int64              `json:"remaining_quota,omitempty"`
	QuotaResetAt      time.Time           `json:"quota_reset_at,omitempty"`
}

const (
	PolicyEntitlementStateRequired PolicyReasonCode = "entitlement_state_required"
	PolicyEntitlementStateInvalid  PolicyReasonCode = "entitlement_state_invalid"
	PolicyEntitlementBinding       PolicyReasonCode = "entitlement_binding_mismatch"
	PolicyEntitlementStateStale    PolicyReasonCode = "entitlement_state_stale"
	PolicyEntitlementExpired       PolicyReasonCode = "entitlement_expired"
	PolicyEntitlementQuota         PolicyReasonCode = "entitlement_quota_exhausted"
	PolicyEntitlementSuspended     PolicyReasonCode = "entitlement_suspended"
	PolicyEntitlementCredential    PolicyReasonCode = "entitlement_credential_rejected"
	PolicyEntitlementBilling       PolicyReasonCode = "entitlement_billing_required"
	PolicyEntitlementAccessDenied  PolicyReasonCode = "entitlement_access_denied"
)

// Authorize applies the static usage policy and, for subscription offerings,
// a fresh account-local entitlement snapshot. It never selects or authorizes
// a pay-as-you-go fallback.
func (route UpstreamRoute) Authorize(invocation InvocationContext, state *EntitlementState, now time.Time) PolicyDecision {
	decision := route.Offering.Entitlement.Decide(invocation)
	decision.AllowsAutomaticPAYGSwitch = false
	if !decision.Allowed || !route.Offering.Kind.subscription() {
		return decision
	}

	var reasons []PolicyReason
	add := func(code PolicyReasonCode, field, message string) {
		reasons = append(reasons, PolicyReason{Code: code, Field: field, Message: message})
	}
	if state == nil {
		add(PolicyEntitlementStateRequired, "entitlement_state", "subscription offering requires a current entitlement state")
		return deniedPolicyDecision(reasons)
	}
	if now.IsZero() || state.ObservedAt.IsZero() || state.ValidUntil.IsZero() ||
		state.ObservedAt.After(now) || !state.ValidUntil.After(state.ObservedAt) {
		add(PolicyEntitlementStateInvalid, "entitlement_state", "entitlement observation window is invalid")
	}
	if state.OfferingID != route.Offering.ID {
		add(PolicyEntitlementBinding, "entitlement_state.offering_id", "entitlement state does not belong to the selected offering")
	}
	if state.CredentialProfile != route.Credential.ID {
		add(PolicyEntitlementBinding, "entitlement_state.credential_profile_id", "entitlement state does not belong to the selected credential profile")
	}
	if !now.IsZero() && !state.ValidUntil.IsZero() && !now.Before(state.ValidUntil) {
		add(PolicyEntitlementStateStale, "entitlement_state.valid_until", "entitlement state is stale")
	}
	if !state.ExpiresAt.IsZero() && !state.ObservedAt.IsZero() && state.ExpiresAt.Before(state.ObservedAt) {
		add(PolicyEntitlementStateInvalid, "entitlement_state.expires_at", "subscription expiry predates the observation")
	} else if !state.ExpiresAt.IsZero() && !now.IsZero() && !now.Before(state.ExpiresAt) {
		add(PolicyEntitlementExpired, "entitlement_state.expires_at", "subscription has expired")
	}
	if state.RemainingQuota != nil && *state.RemainingQuota < 0 {
		add(PolicyEntitlementStateInvalid, "entitlement_state.remaining_quota", "remaining quota cannot be negative")
	} else if state.RemainingQuota != nil && *state.RemainingQuota == 0 {
		add(PolicyEntitlementQuota, "entitlement_state.remaining_quota", "subscription quota is exhausted")
	}
	if !state.QuotaResetAt.IsZero() && !state.ObservedAt.IsZero() && state.QuotaResetAt.Before(state.ObservedAt) {
		add(PolicyEntitlementStateInvalid, "entitlement_state.quota_reset_at", "quota reset predates the observation")
	}
	switch state.Status {
	case EntitlementActive:
	case EntitlementQuotaExhausted:
		add(PolicyEntitlementQuota, "entitlement_state.status", "subscription quota is exhausted")
	case EntitlementExpired:
		add(PolicyEntitlementExpired, "entitlement_state.status", "subscription has expired")
	case EntitlementSuspended:
		add(PolicyEntitlementSuspended, "entitlement_state.status", "subscription is suspended")
	default:
		add(PolicyEntitlementStateInvalid, "entitlement_state.status", "entitlement status is invalid")
	}
	if len(reasons) > 0 {
		return deniedPolicyDecision(reasons)
	}
	return PolicyDecision{Allowed: true, Code: PolicyAllowed, AllowsAutomaticPAYGSwitch: false}
}

func deniedPolicyDecision(reasons []PolicyReason) PolicyDecision {
	return PolicyDecision{
		Allowed: false, Code: reasons[0].Code,
		Reasons:                   append([]PolicyReason(nil), reasons...),
		AllowsAutomaticPAYGSwitch: false,
	}
}

// DenyHTTPFailure converts subscription HTTP terminal statuses into stable
// control-plane reasons. It intentionally ignores provider body text and never
// authorizes a pay-as-you-go fallback.
func (route UpstreamRoute) DenyHTTPFailure(status int) PolicyDecision {
	code := PolicyEntitlementStateInvalid
	field := "http_status"
	message := "subscription request failed"
	if route.Offering.Kind.subscription() {
		switch status {
		case 401:
			code, message = PolicyEntitlementCredential, "subscription credential was rejected"
		case 402:
			code, message = PolicyEntitlementBilling, "subscription requires billing action"
		case 403:
			code, message = PolicyEntitlementAccessDenied, "subscription access was denied"
		case 429:
			code, message = PolicyEntitlementQuota, "subscription quota or request window is exhausted"
		}
	}
	return deniedPolicyDecision([]PolicyReason{{Code: code, Field: field, Message: message}})
}

func (kind OfferingKind) subscription() bool {
	return kind == OfferingTokenPlan || kind == OfferingCodingPlan
}
