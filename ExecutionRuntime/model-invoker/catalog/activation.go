package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const MaxActivationPlanRoutes = 256

// ActivationAction is the only mutation an activation plan may request. An
// activation plan cannot replace route facts, adapters, endpoints, models, or
// credentials.
type ActivationAction string

const (
	ActivateHostBlockedRoute ActivationAction = "activate_host_blocked_route"
	DisableRoute             ActivationAction = "disable_route"
)

// ActivationDecisionCode is a stable, machine-readable activation outcome.
type ActivationDecisionCode string

const (
	ActivationCodeReady                   ActivationDecisionCode = "ready"
	ActivationCodeActivated               ActivationDecisionCode = "activated"
	ActivationCodeDisabled                ActivationDecisionCode = "disabled"
	ActivationCodeAlreadyDisabled         ActivationDecisionCode = "already_disabled"
	ActivationCodeNotApplied              ActivationDecisionCode = "not_applied_atomic_failure"
	ActivationCodeInvalidPlan             ActivationDecisionCode = "invalid_plan"
	ActivationCodeInvalidAction           ActivationDecisionCode = "invalid_action"
	ActivationCodeRouteIDNotExact         ActivationDecisionCode = "route_id_not_exact"
	ActivationCodeRouteNotFound           ActivationDecisionCode = "route_not_found"
	ActivationCodeDuplicateRouteAction    ActivationDecisionCode = "duplicate_route_action"
	ActivationCodeEvidenceDigestRequired  ActivationDecisionCode = "evidence_digest_required"
	ActivationCodeEvidenceDigestMismatch  ActivationDecisionCode = "evidence_digest_mismatch"
	ActivationCodeAdapterIDRequired       ActivationDecisionCode = "adapter_id_required"
	ActivationCodeAdapterIDMismatch       ActivationDecisionCode = "adapter_id_mismatch"
	ActivationCodeRouteNotHostBlocked     ActivationDecisionCode = "route_not_host_blocked"
	ActivationCodeOfferingNotSubscription ActivationDecisionCode = "offering_not_subscription"
	ActivationCodeTermsBlocked            ActivationDecisionCode = "terms_blocked"
	ActivationCodeEvidenceUnavailable     ActivationDecisionCode = "evidence_unavailable"
	ActivationCodeEvidenceExpired         ActivationDecisionCode = "evidence_expired"
	ActivationCodeOfficialClientOnly      ActivationDecisionCode = "official_client_only"
	ActivationCodeImplementationNotReady  ActivationDecisionCode = "implementation_not_ready"
	ActivationCodeCatalogValidationFailed ActivationDecisionCode = "catalog_validation_failed"
)

// RouteActivation pins one exact RouteID to both its source-backed evidence and
// its runtime adapter. ExpectedAdapterID is separate because implementation
// state is deliberately excluded from Evidence.Digest.
type RouteActivation struct {
	RouteID                upstream.RouteID `json:"route_id"`
	Action                 ActivationAction `json:"action"`
	ExpectedEvidenceDigest string           `json:"expected_evidence_digest"`
	ExpectedAdapterID      string           `json:"expected_adapter_id"`
}

// ActivationPlan is a host-owned, exact RouteID overlay for one immutable
// catalog snapshot.
type ActivationPlan struct {
	ID       string            `json:"id"`
	Revision string            `json:"revision"`
	Routes   []RouteActivation `json:"routes"`
}

// Clone returns a plan with independent route storage. Callers must still not
// mutate the source plan concurrently with Clone, following normal Go value
// ownership rules.
func (plan ActivationPlan) Clone() ActivationPlan {
	clone := plan
	clone.Routes = append([]RouteActivation(nil), plan.Routes...)
	return clone
}

// ActivationDecision records only public catalog facts. It never contains
// resolved secret material or entitlement state.
type ActivationDecision struct {
	RouteID           upstream.RouteID          `json:"route_id,omitempty"`
	Action            ActivationAction          `json:"action,omitempty"`
	BeforeCallable    bool                      `json:"before_callable"`
	AfterCallable     bool                      `json:"after_callable"`
	BeforeRequirement HostActivationRequirement `json:"before_requirement,omitempty"`
	AfterRequirement  HostActivationRequirement `json:"after_requirement,omitempty"`
	EvidenceDigest    string                    `json:"evidence_digest,omitempty"`
	AdapterID         string                    `json:"adapter_id,omitempty"`
	Code              ActivationDecisionCode    `json:"code"`
}

// ActivationReport is canonicalized by RouteID and action before AuditDigest
// is computed, so reordering an otherwise identical plan cannot change the
// audit identity.
type ActivationReport struct {
	PlanID          string               `json:"plan_id"`
	Revision        string               `json:"revision"`
	SchemaVersion   string               `json:"schema_version"`
	PlanInputDigest string               `json:"plan_input_digest"`
	Applied         bool                 `json:"applied"`
	Decisions       []ActivationDecision `json:"decisions"`
	AuditDigest     string               `json:"audit_digest"`
}

// ActivationError reports one stable rejection code. The complete canonical
// decision set remains available in ActivationReport.
type ActivationError struct {
	Code    ActivationDecisionCode
	RouteID upstream.RouteID
	Err     error
}

func (e *ActivationError) Error() string {
	if e == nil {
		return ""
	}
	if e.RouteID != "" {
		return fmt.Sprintf("catalog activation rejected for route %q: %s", e.RouteID, e.Code)
	}
	return fmt.Sprintf("catalog activation rejected: %s", e.Code)
}

func (e *ActivationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ApplyActivationPlan creates a new immutable catalog snapshot. It never
// mutates base and never returns a partially applied catalog.
func ApplyActivationPlan(base *Catalog, plan ActivationPlan, now time.Time) (*Catalog, ActivationReport, error) {
	plan = plan.Clone()
	report := ActivationReport{
		PlanID: safeActivationReportID(plan.ID), Revision: safeActivationReportID(plan.Revision),
		PlanInputDigest: canonicalActivationPlanInputDigest(plan),
	}
	if base != nil {
		report.SchemaVersion = base.SchemaVersion()
	}
	if base == nil || now.IsZero() || !safeActivationID(plan.ID) || !safeActivationID(plan.Revision) || len(plan.Routes) == 0 || len(plan.Routes) > MaxActivationPlanRoutes {
		report.Decisions = planDecisions(plan.Routes, ActivationCodeInvalidPlan)
		finalizeActivationReport(&report)
		return nil, report, &ActivationError{Code: ActivationCodeInvalidPlan}
	}

	document := base.Document()
	entries := make(map[upstream.RouteID]Entry, len(document.Entries))
	indices := make(map[upstream.RouteID]int, len(document.Entries))
	for index, entry := range document.Entries {
		entries[entry.ID] = entry
		indices[entry.ID] = index
	}

	seen := make(map[upstream.RouteID]struct{}, len(plan.Routes))
	report.Decisions = make([]ActivationDecision, 0, len(plan.Routes))
	for _, change := range plan.Routes {
		decision := ActivationDecision{
			Action: safeActivationReportAction(change.Action), Code: ActivationCodeReady,
		}
		entry, found := entries[change.RouteID]
		if found {
			decision.RouteID = entry.ID
			decision.BeforeCallable = entry.Implementation.Callable
			decision.AfterCallable = entry.Implementation.Callable
			decision.BeforeRequirement = entry.Implementation.HostActivationRequirement
			decision.AfterRequirement = entry.Implementation.HostActivationRequirement
			decision.EvidenceDigest = entry.Evidence.Digest
			decision.AdapterID = entry.Implementation.AdapterID
		}

		switch {
		case !exactActivationRouteID(change.RouteID):
			decision.Code = ActivationCodeRouteIDNotExact
		case duplicateActivationRoute(seen, change.RouteID):
			decision.Code = ActivationCodeDuplicateRouteAction
		case change.Action != ActivateHostBlockedRoute && change.Action != DisableRoute:
			decision.Code = ActivationCodeInvalidAction
		case !found:
			decision.Code = ActivationCodeRouteNotFound
		case strings.TrimSpace(change.ExpectedEvidenceDigest) == "":
			decision.Code = ActivationCodeEvidenceDigestRequired
		case !exactActivationEvidenceDigest(change.ExpectedEvidenceDigest):
			decision.Code = ActivationCodeEvidenceDigestMismatch
		case change.ExpectedEvidenceDigest != entry.Evidence.Digest:
			decision.Code = ActivationCodeEvidenceDigestMismatch
		case strings.TrimSpace(change.ExpectedAdapterID) == "":
			decision.Code = ActivationCodeAdapterIDRequired
		case !metadataIDPattern.MatchString(change.ExpectedAdapterID):
			decision.Code = ActivationCodeAdapterIDMismatch
		case change.ExpectedAdapterID != entry.Implementation.AdapterID:
			decision.Code = ActivationCodeAdapterIDMismatch
		case change.Action == ActivateHostBlockedRoute:
			decision.Code = validateHostActivation(entry, now)
		}
		report.Decisions = append(report.Decisions, decision)
	}

	sortActivationDecisions(report.Decisions)
	if code, routeID, failed := firstActivationFailure(report.Decisions); failed {
		for index := range report.Decisions {
			if report.Decisions[index].Code == ActivationCodeReady {
				report.Decisions[index].Code = ActivationCodeNotApplied
			}
		}
		finalizeActivationReport(&report)
		return nil, report, &ActivationError{Code: code, RouteID: routeID}
	}

	for _, change := range plan.Routes {
		index := indices[change.RouteID]
		entry := document.Entries[index]
		switch change.Action {
		case ActivateHostBlockedRoute:
			entry.Implementation.Callable = true
			entry.Implementation.HostActivationRequirement = ""
		case DisableRoute:
			entry.Implementation.Callable = false
			if subscriptionActivationOffering(entry.Route.Offering.Kind) &&
				entry.Implementation.AdapterID != "" &&
				entry.Implementation.Status.rank() >= ImplementationImplementedOffline.rank() {
				entry.Implementation.HostActivationRequirement = HostActivationTrustedSubscriptionAuthorizationResolver
			}
		}
		document.Entries[index] = entry
	}

	activated, err := New(document, now)
	if err != nil {
		for index := range report.Decisions {
			report.Decisions[index].Code = ActivationCodeCatalogValidationFailed
			report.Decisions[index].AfterCallable = report.Decisions[index].BeforeCallable
			report.Decisions[index].AfterRequirement = report.Decisions[index].BeforeRequirement
		}
		finalizeActivationReport(&report)
		return nil, report, &ActivationError{Code: ActivationCodeCatalogValidationFailed, Err: err}
	}

	for index := range report.Decisions {
		entry, _ := activated.Get(report.Decisions[index].RouteID)
		report.Decisions[index].AfterCallable = entry.Implementation.Callable
		report.Decisions[index].AfterRequirement = entry.Implementation.HostActivationRequirement
		if report.Decisions[index].Action == ActivateHostBlockedRoute {
			report.Decisions[index].Code = ActivationCodeActivated
		} else if report.Decisions[index].BeforeCallable || report.Decisions[index].BeforeRequirement != report.Decisions[index].AfterRequirement {
			report.Decisions[index].Code = ActivationCodeDisabled
		} else {
			report.Decisions[index].Code = ActivationCodeAlreadyDisabled
		}
	}
	report.Applied = true
	finalizeActivationReport(&report)
	return activated, report, nil
}

func validateHostActivation(entry Entry, now time.Time) ActivationDecisionCode {
	if entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != HostActivationTrustedSubscriptionAuthorizationResolver {
		return ActivationCodeRouteNotHostBlocked
	}
	if !subscriptionActivationOffering(entry.Route.Offering.Kind) {
		return ActivationCodeOfferingNotSubscription
	}
	if entry.Evidence.Status == EvidenceTermsBlocked {
		return ActivationCodeTermsBlocked
	}
	if entry.Evidence.Status != EvidenceFresh {
		return ActivationCodeEvidenceUnavailable
	}
	if now.Before(entry.Evidence.CheckedAt) || !now.Before(entry.Evidence.ValidUntil) {
		return ActivationCodeEvidenceExpired
	}
	if entry.Route.Offering.Entitlement.AllowedUsage == upstream.AllowedUsageOfficialClientOnly {
		return ActivationCodeOfficialClientOnly
	}
	if entry.Implementation.Status.rank() < ImplementationImplementedOffline.rank() || entry.Implementation.AdapterID == "" {
		return ActivationCodeImplementationNotReady
	}
	return ActivationCodeReady
}

func subscriptionActivationOffering(kind upstream.OfferingKind) bool {
	return kind == upstream.OfferingTokenPlan || kind == upstream.OfferingCodingPlan
}

func exactActivationRouteID(routeID upstream.RouteID) bool {
	value := string(routeID)
	return value != "" && value == strings.TrimSpace(value) && metadataIDPattern.MatchString(value) && !strings.ContainsAny(value, "*?[]{}")
}

func duplicateActivationRoute(seen map[upstream.RouteID]struct{}, routeID upstream.RouteID) bool {
	if _, exists := seen[routeID]; exists {
		return true
	}
	seen[routeID] = struct{}{}
	return false
}

func safeActivationID(value string) bool {
	return value == strings.TrimSpace(value) && metadataIDPattern.MatchString(value)
}

func safeActivationReportID(value string) string {
	if safeActivationID(value) {
		return value
	}
	return ""
}

func safeActivationReportAction(action ActivationAction) ActivationAction {
	if action == ActivateHostBlockedRoute || action == DisableRoute {
		return action
	}
	return ""
}

func exactActivationEvidenceDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	hexValue := strings.TrimPrefix(value, "sha256:")
	if hexValue != strings.ToLower(hexValue) {
		return false
	}
	decoded, err := hex.DecodeString(hexValue)
	return err == nil && len(decoded) == sha256.Size
}

func firstActivationFailure(decisions []ActivationDecision) (ActivationDecisionCode, upstream.RouteID, bool) {
	for _, decision := range decisions {
		if decision.Code != ActivationCodeReady {
			return decision.Code, decision.RouteID, true
		}
	}
	return "", "", false
}

func planDecisions(routes []RouteActivation, code ActivationDecisionCode) []ActivationDecision {
	decisions := make([]ActivationDecision, 0, len(routes))
	for range routes {
		decisions = append(decisions, ActivationDecision{Code: code})
	}
	sortActivationDecisions(decisions)
	return decisions
}

func sortActivationDecisions(decisions []ActivationDecision) {
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].RouteID != decisions[j].RouteID {
			return decisions[i].RouteID < decisions[j].RouteID
		}
		if decisions[i].Action != decisions[j].Action {
			return decisions[i].Action < decisions[j].Action
		}
		return decisions[i].Code < decisions[j].Code
	})
}

// canonicalActivationPlanInputDigest commits the audit trail to every supplied
// pin without reflecting caller-provided text into ActivationReport. Route
// order is canonical because an ActivationPlan rejects duplicate RouteIDs and
// applies its exact actions as one transaction.
func canonicalActivationPlanInputDigest(plan ActivationPlan) string {
	routes := append([]RouteActivation(nil), plan.Routes...)
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].RouteID != routes[j].RouteID {
			return routes[i].RouteID < routes[j].RouteID
		}
		if routes[i].Action != routes[j].Action {
			return routes[i].Action < routes[j].Action
		}
		if routes[i].ExpectedEvidenceDigest != routes[j].ExpectedEvidenceDigest {
			return routes[i].ExpectedEvidenceDigest < routes[j].ExpectedEvidenceDigest
		}
		return routes[i].ExpectedAdapterID < routes[j].ExpectedAdapterID
	})
	payload := struct {
		ID       string            `json:"id"`
		Revision string            `json:"revision"`
		Routes   []RouteActivation `json:"routes"`
	}{ID: plan.ID, Revision: plan.Revision, Routes: routes}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic("catalog: marshal activation plan input")
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func finalizeActivationReport(report *ActivationReport) {
	if report == nil {
		return
	}
	sortActivationDecisions(report.Decisions)
	payload := struct {
		PlanID          string               `json:"plan_id"`
		Revision        string               `json:"revision"`
		SchemaVersion   string               `json:"schema_version"`
		PlanInputDigest string               `json:"plan_input_digest"`
		Applied         bool                 `json:"applied"`
		Decisions       []ActivationDecision `json:"decisions"`
	}{
		PlanID: report.PlanID, Revision: report.Revision, SchemaVersion: report.SchemaVersion,
		PlanInputDigest: report.PlanInputDigest, Applied: report.Applied, Decisions: report.Decisions,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic("catalog: marshal activation audit report")
	}
	digest := sha256.Sum256(encoded)
	report.AuditDigest = fmt.Sprintf("sha256:%x", digest[:])
}
