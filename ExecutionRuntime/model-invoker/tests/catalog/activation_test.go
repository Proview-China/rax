package catalog_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestApplyActivationPlanActivatesExactRoutesAtomicallyAndCanonically(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	kimi := pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
	miniMax := pinnedActivation(t, base, "minimax.token-plan.global.messages", catalog.ActivateHostBlockedRoute)

	plan := catalog.ActivationPlan{ID: "catalog.activation.test", Revision: "r1", Routes: []catalog.RouteActivation{miniMax, kimi}}
	activated, report, err := catalog.ApplyActivationPlan(base, plan, testNow)
	if err != nil {
		t.Fatalf("ApplyActivationPlan() error = %v", err)
	}
	if activated == nil || !report.Applied || len(report.Decisions) != 2 || !validAuditDigest(report.AuditDigest) {
		t.Fatalf("activation result/report = %#v / %#v", activated, report)
	}
	if report.Decisions[0].RouteID != kimi.RouteID || report.Decisions[1].RouteID != miniMax.RouteID {
		t.Fatalf("decisions are not canonical: %#v", report.Decisions)
	}
	for _, decision := range report.Decisions {
		if decision.Code != catalog.ActivationCodeActivated || decision.BeforeCallable || !decision.AfterCallable ||
			decision.BeforeRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver || decision.AfterRequirement != "" {
			t.Errorf("activation decision = %#v", decision)
		}
		entry, ok := activated.Get(decision.RouteID)
		if !ok || !entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != "" {
			t.Errorf("activated entry %q = %#v", decision.RouteID, entry.Implementation)
		}
	}
	for _, routeID := range []upstream.RouteID{kimi.RouteID, miniMax.RouteID} {
		entry, _ := base.Get(routeID)
		if entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
			t.Fatalf("base catalog was mutated for %q", routeID)
		}
	}

	reordered := plan
	reordered.Routes = []catalog.RouteActivation{kimi, miniMax}
	_, reorderedReport, err := catalog.ApplyActivationPlan(base, reordered, testNow)
	if err != nil {
		t.Fatalf("reordered ApplyActivationPlan() error = %v", err)
	}
	if !reflect.DeepEqual(report, reorderedReport) {
		t.Fatalf("canonical reports differ:\nfirst  = %#v\nsecond = %#v", report, reorderedReport)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "KIMI_CODE_API_KEY") || strings.Contains(string(payload), "MINIMAX_TOKEN_PLAN_API_KEY") {
		t.Fatalf("activation report exposed credential references: %s", payload)
	}
}

func TestApplyActivationPlanRejectsMixedPlanWithoutPartialCatalog(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	kimi := pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
	miniMax := pinnedActivation(t, base, "minimax.token-plan.global.messages", catalog.ActivateHostBlockedRoute)
	miniMax.ExpectedAdapterID = "wrong-adapter"

	activated, report, err := catalog.ApplyActivationPlan(base, catalog.ActivationPlan{
		ID: "catalog.activation.atomic", Revision: "r1", Routes: []catalog.RouteActivation{kimi, miniMax},
	}, testNow)
	if activated != nil || err == nil || report.Applied {
		t.Fatalf("mixed activation = %#v, report = %#v, error = %v", activated, report, err)
	}
	var activationError *catalog.ActivationError
	if !errors.As(err, &activationError) || activationError.Code != catalog.ActivationCodeAdapterIDMismatch || activationError.RouteID != miniMax.RouteID {
		t.Fatalf("activation error = %#v", activationError)
	}
	wantCodes := map[upstream.RouteID]catalog.ActivationDecisionCode{
		kimi.RouteID:    catalog.ActivationCodeNotApplied,
		miniMax.RouteID: catalog.ActivationCodeAdapterIDMismatch,
	}
	for _, decision := range report.Decisions {
		if decision.Code != wantCodes[decision.RouteID] || decision.AfterCallable != decision.BeforeCallable || decision.AfterRequirement != decision.BeforeRequirement {
			t.Errorf("atomic failure decision = %#v", decision)
		}
	}
	entry, _ := base.Get(kimi.RouteID)
	if entry.Implementation.Callable {
		t.Fatal("base catalog was partially activated")
	}
}

func TestApplyActivationPlanReturnsStablePreflightRejectionCodes(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	kimi := pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
	openAI := pinnedActivation(t, base, "openai.direct.payg.responses", catalog.ActivateHostBlockedRoute)

	tests := []struct {
		name   string
		routes []catalog.RouteActivation
		code   catalog.ActivationDecisionCode
	}{
		{name: "wildcard", routes: []catalog.RouteActivation{{RouteID: "kimi.*", Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: kimi.ExpectedEvidenceDigest, ExpectedAdapterID: kimi.ExpectedAdapterID}}, code: catalog.ActivationCodeRouteIDNotExact},
		{name: "unknown exact route", routes: []catalog.RouteActivation{{RouteID: "kimi.missing.route", Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: kimi.ExpectedEvidenceDigest, ExpectedAdapterID: kimi.ExpectedAdapterID}}, code: catalog.ActivationCodeRouteNotFound},
		{name: "invalid action", routes: []catalog.RouteActivation{{RouteID: kimi.RouteID, Action: "force", ExpectedEvidenceDigest: kimi.ExpectedEvidenceDigest, ExpectedAdapterID: kimi.ExpectedAdapterID}}, code: catalog.ActivationCodeInvalidAction},
		{name: "missing digest", routes: []catalog.RouteActivation{{RouteID: kimi.RouteID, Action: kimi.Action, ExpectedAdapterID: kimi.ExpectedAdapterID}}, code: catalog.ActivationCodeEvidenceDigestRequired},
		{name: "wrong digest", routes: []catalog.RouteActivation{{RouteID: kimi.RouteID, Action: kimi.Action, ExpectedEvidenceDigest: "sha256:wrong", ExpectedAdapterID: kimi.ExpectedAdapterID}}, code: catalog.ActivationCodeEvidenceDigestMismatch},
		{name: "missing adapter", routes: []catalog.RouteActivation{{RouteID: kimi.RouteID, Action: kimi.Action, ExpectedEvidenceDigest: kimi.ExpectedEvidenceDigest}}, code: catalog.ActivationCodeAdapterIDRequired},
		{name: "wrong adapter", routes: []catalog.RouteActivation{{RouteID: kimi.RouteID, Action: kimi.Action, ExpectedEvidenceDigest: kimi.ExpectedEvidenceDigest, ExpectedAdapterID: "wrong"}}, code: catalog.ActivationCodeAdapterIDMismatch},
		{name: "callable route is not host blocked", routes: []catalog.RouteActivation{openAI}, code: catalog.ActivationCodeRouteNotHostBlocked},
		{name: "duplicate route action", routes: []catalog.RouteActivation{kimi, kimi}, code: catalog.ActivationCodeDuplicateRouteAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, report, err := catalog.ApplyActivationPlan(base, catalog.ActivationPlan{ID: "catalog.activation.reject", Revision: "r1", Routes: test.routes}, testNow)
			if result != nil || err == nil || report.Applied || !validAuditDigest(report.AuditDigest) {
				t.Fatalf("result/report/error = %#v / %#v / %v", result, report, err)
			}
			var activationError *catalog.ActivationError
			if !errors.As(err, &activationError) || activationError.Code != test.code {
				t.Fatalf("ActivationError = %#v, want %q", activationError, test.code)
			}
			found := false
			for _, decision := range report.Decisions {
				found = found || decision.Code == test.code
			}
			if !found {
				t.Fatalf("report decisions = %#v, want code %q", report.Decisions, test.code)
			}
		})
	}
}

func TestApplyActivationPlanRejectsTermsAndOfficialClientOnlyRoutes(t *testing.T) {
	t.Parallel()
	termsBlocked := mutatedActivationCatalog(t, func(entry *catalog.Entry) bool {
		if entry.ID != "kimi.code-membership.global.chat_completions" {
			return false
		}
		entry.Evidence.Status = catalog.EvidenceTermsBlocked
		return true
	})
	officialClientOnly := mutatedActivationCatalog(t, func(entry *catalog.Entry) bool {
		if entry.Route.Offering.ID != "kimi.code-membership" {
			return false
		}
		entry.Route.Offering.Entitlement.AllowedUsage = upstream.AllowedUsageOfficialClientOnly
		return true
	})

	for _, test := range []struct {
		name string
		base *catalog.Catalog
		code catalog.ActivationDecisionCode
	}{
		{name: "terms blocked", base: termsBlocked, code: catalog.ActivationCodeTermsBlocked},
		{name: "official client only", base: officialClientOnly, code: catalog.ActivationCodeOfficialClientOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			change := pinnedActivation(t, test.base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
			result, report, err := catalog.ApplyActivationPlan(test.base, catalog.ActivationPlan{ID: "catalog.activation.policy", Revision: "r1", Routes: []catalog.RouteActivation{change}}, testNow)
			if result != nil || err == nil || len(report.Decisions) != 1 || report.Decisions[0].Code != test.code {
				t.Fatalf("result/report/error = %#v / %#v / %v", result, report, err)
			}
		})
	}
}

func TestApplyActivationPlanRejectsUnavailableOrExpiredEvidence(t *testing.T) {
	t.Parallel()
	unverified := mutatedActivationCatalog(t, func(entry *catalog.Entry) bool {
		if entry.ID != "kimi.code-membership.global.chat_completions" {
			return false
		}
		entry.Evidence.Status = catalog.EvidenceUnverified
		return true
	})
	base := activationCatalog(t)
	baseEntry, _ := base.Get("kimi.code-membership.global.chat_completions")

	for _, test := range []struct {
		name string
		base *catalog.Catalog
		now  time.Time
		code catalog.ActivationDecisionCode
	}{
		{name: "unverified", base: unverified, now: testNow, code: catalog.ActivationCodeEvidenceUnavailable},
		{name: "expired", base: base, now: baseEntry.Evidence.ValidUntil, code: catalog.ActivationCodeEvidenceExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			change := pinnedActivation(t, test.base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
			result, report, err := catalog.ApplyActivationPlan(test.base, catalog.ActivationPlan{ID: "catalog.activation.evidence", Revision: "r1", Routes: []catalog.RouteActivation{change}}, test.now)
			if result != nil || err == nil || report.Applied || len(report.Decisions) != 1 || report.Decisions[0].Code != test.code {
				t.Fatalf("result/report/error = %#v / %#v / %v", result, report, err)
			}
		})
	}
}

func TestApplyActivationPlanRejectsInvalidTopLevelPlan(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	change := pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
	for _, test := range []struct {
		name string
		base *catalog.Catalog
		plan catalog.ActivationPlan
		now  time.Time
	}{
		{name: "nil catalog", plan: catalog.ActivationPlan{ID: "catalog.activation.invalid", Revision: "r1", Routes: []catalog.RouteActivation{change}}, now: testNow},
		{name: "zero time", base: base, plan: catalog.ActivationPlan{ID: "catalog.activation.invalid", Revision: "r1", Routes: []catalog.RouteActivation{change}}},
		{name: "missing id", base: base, plan: catalog.ActivationPlan{Revision: "r1", Routes: []catalog.RouteActivation{change}}, now: testNow},
		{name: "missing revision", base: base, plan: catalog.ActivationPlan{ID: "catalog.activation.invalid", Routes: []catalog.RouteActivation{change}}, now: testNow},
		{name: "empty routes", base: base, plan: catalog.ActivationPlan{ID: "catalog.activation.invalid", Revision: "r1"}, now: testNow},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, report, err := catalog.ApplyActivationPlan(test.base, test.plan, test.now)
			var activationError *catalog.ActivationError
			if result != nil || err == nil || report.Applied || !errors.As(err, &activationError) || activationError.Code != catalog.ActivationCodeInvalidPlan || !validAuditDigest(report.AuditDigest) {
				t.Fatalf("result/report/error = %#v / %#v / %#v", result, report, activationError)
			}
		})
	}
}

func TestDisableRouteRestoresSubscriptionRequirementAndPreservesDirectBoundary(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	kimiActivation := pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)
	active, _, err := catalog.ApplyActivationPlan(base, catalog.ActivationPlan{ID: "catalog.activation.enable", Revision: "r1", Routes: []catalog.RouteActivation{kimiActivation}}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	kimiDisable := pinnedActivation(t, active, kimiActivation.RouteID, catalog.DisableRoute)
	disabled, report, err := catalog.ApplyActivationPlan(active, catalog.ActivationPlan{ID: "catalog.activation.disable", Revision: "r1", Routes: []catalog.RouteActivation{kimiDisable}}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := disabled.Get(kimiDisable.RouteID)
	if entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver || report.Decisions[0].Code != catalog.ActivationCodeDisabled {
		t.Fatalf("disabled subscription = %#v, report = %#v", entry.Implementation, report)
	}

	openAIDisable := pinnedActivation(t, base, "openai.direct.payg.responses", catalog.DisableRoute)
	directDisabled, directReport, err := catalog.ApplyActivationPlan(base, catalog.ActivationPlan{ID: "catalog.activation.disable-direct", Revision: "r1", Routes: []catalog.RouteActivation{openAIDisable}}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ = directDisabled.Get(openAIDisable.RouteID)
	if entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != "" || directReport.Decisions[0].Code != catalog.ActivationCodeDisabled {
		t.Fatalf("disabled direct route = %#v, report = %#v", entry.Implementation, directReport)
	}

	alreadyDisabled := pinnedActivation(t, base, kimiActivation.RouteID, catalog.DisableRoute)
	unchanged, unchangedReport, err := catalog.ApplyActivationPlan(base, catalog.ActivationPlan{ID: "catalog.activation.disable-idempotent", Revision: "r1", Routes: []catalog.RouteActivation{alreadyDisabled}}, testNow)
	if err != nil || unchanged == nil || unchangedReport.Decisions[0].Code != catalog.ActivationCodeAlreadyDisabled {
		t.Fatalf("idempotent disable = %#v, report = %#v, error = %v", unchanged, unchangedReport, err)
	}
}

func TestActivationPlanCloneOwnsIndependentRouteStorage(t *testing.T) {
	base := activationCatalog(t)
	original := catalog.ActivationPlan{
		ID: "catalog.activation.clone", Revision: "r1",
		Routes: []catalog.RouteActivation{pinnedActivation(t, base, "kimi.code-membership.global.chat_completions", catalog.ActivateHostBlockedRoute)},
	}
	clone := original.Clone()
	original.Routes[0].RouteID = "mutated.original"
	if clone.Routes[0].RouteID != "kimi.code-membership.global.chat_completions" {
		t.Fatalf("clone shares original route storage: %#v", clone.Routes)
	}
	clone.Routes[0].Action = catalog.DisableRoute
	if original.Routes[0].Action != catalog.ActivateHostBlockedRoute {
		t.Fatalf("original shares clone route storage: %#v", original.Routes)
	}
}

func TestActivationReportNeverReflectsUntrustedPinOrIdentifierText(t *testing.T) {
	base := activationCatalog(t)
	entry, _ := base.Get("kimi.code-membership.global.chat_completions")
	secrets := []string{"DO-NOT-LOG-ROUTE-SECRET", "do-not-log-route-secret", "DO-NOT-LOG-DIGEST-SECRET", "DO-NOT-LOG-ADAPTER-SECRET", "DO-NOT-LOG-PLAN-SECRET"}
	tests := []catalog.ActivationPlan{
		{
			ID: "catalog.activation.safe-report", Revision: "r1",
			Routes: []catalog.RouteActivation{{RouteID: upstream.RouteID(secrets[0]), Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID}},
		},
		{
			ID: "catalog.activation.safe-report", Revision: "r1",
			Routes: []catalog.RouteActivation{{RouteID: upstream.RouteID(secrets[1]), Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID}},
		},
		{
			ID: "catalog.activation.safe-report", Revision: "r1",
			Routes: []catalog.RouteActivation{{RouteID: entry.ID, Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: secrets[2], ExpectedAdapterID: entry.Implementation.AdapterID}},
		},
		{
			ID: "catalog.activation.safe-report", Revision: "r1",
			Routes: []catalog.RouteActivation{{RouteID: entry.ID, Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: secrets[3]}},
		},
		{
			ID: secrets[4], Revision: "r1",
			Routes: []catalog.RouteActivation{{RouteID: entry.ID, Action: catalog.ActivateHostBlockedRoute, ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID}},
		},
	}
	for index, plan := range tests {
		_, report, err := catalog.ApplyActivationPlan(base, plan, testNow)
		if err == nil || report.Applied {
			t.Fatalf("case %d unexpectedly applied: %#v", index, report)
		}
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, secret := range secrets {
			if strings.Contains(string(payload), secret) || strings.Contains(err.Error(), secret) {
				t.Fatalf("case %d reflected untrusted text: report=%s err=%v", index, payload, err)
			}
		}
	}
}

func TestActivationPlanInputDigestDistinguishesPinsWithoutReflectingThem(t *testing.T) {
	t.Parallel()
	base := activationCatalog(t)
	entry, _ := base.Get("kimi.code-membership.global.chat_completions")
	plan := catalog.ActivationPlan{
		ID: "catalog.activation.input-digest", Revision: "r1",
		Routes: []catalog.RouteActivation{{
			RouteID: entry.ID, Action: catalog.ActivateHostBlockedRoute,
			ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: "wrong-adapter-a",
		}},
	}
	_, first, firstErr := catalog.ApplyActivationPlan(base, plan, testNow)
	plan.Routes[0].ExpectedAdapterID = "wrong-adapter-b"
	_, second, secondErr := catalog.ApplyActivationPlan(base, plan, testNow)
	if firstErr == nil || secondErr == nil {
		t.Fatalf("mismatched adapter pins unexpectedly applied: %v / %v", firstErr, secondErr)
	}
	if !validAuditDigest(first.PlanInputDigest) || !validAuditDigest(second.PlanInputDigest) || first.PlanInputDigest == second.PlanInputDigest {
		t.Fatalf("plan input digests = %q / %q", first.PlanInputDigest, second.PlanInputDigest)
	}
	if first.AuditDigest == second.AuditDigest {
		t.Fatalf("audit digests did not commit to distinct plan inputs: %q", first.AuditDigest)
	}
	for _, report := range []catalog.ActivationReport{first, second} {
		payload, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "wrong-adapter-a") || strings.Contains(string(payload), "wrong-adapter-b") {
			t.Fatalf("report reflected a rejected pin: %s", payload)
		}
	}
}

func activationCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	value, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func pinnedActivation(t *testing.T, snapshot *catalog.Catalog, routeID upstream.RouteID, action catalog.ActivationAction) catalog.RouteActivation {
	t.Helper()
	entry, ok := snapshot.Get(routeID)
	if !ok {
		t.Fatalf("route %q not found", routeID)
	}
	return catalog.RouteActivation{
		RouteID: routeID, Action: action,
		ExpectedEvidenceDigest: entry.Evidence.Digest,
		ExpectedAdapterID:      entry.Implementation.AdapterID,
	}
}

func mutatedActivationCatalog(t *testing.T, mutate func(*catalog.Entry) bool) *catalog.Catalog {
	t.Helper()
	document := catalog.DefaultDocument()
	matched := false
	for index := range document.Entries {
		if !mutate(&document.Entries[index]) {
			continue
		}
		matched = true
		digest, err := catalog.ComputeEvidenceDigest(document.Entries[index])
		if err != nil {
			t.Fatal(err)
		}
		document.Entries[index].Evidence.Digest = digest
	}
	if !matched {
		t.Fatal("mutation did not match a catalog entry")
	}
	value, err := catalog.New(document, testNow)
	if err != nil {
		t.Fatalf("mutated catalog: %v", err)
	}
	return value
}

func validAuditDigest(value string) bool {
	return len(value) == len("sha256:")+64 && strings.HasPrefix(value, "sha256:")
}
