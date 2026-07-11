package catalog_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestSubscriptionCatalogRecordsAreBoundAndNonCallable(t *testing.T) {
	document := catalog.DefaultDocument()
	wantRoutes := map[upstream.OfferingID]int{
		"zai.glm-coding-plan":       1,
		"kimi.code-membership":      2,
		"minimax.token-plan":        2,
		"mimo.token-plan":           6,
		"alibaba.coding-plan":       4,
		"alibaba.token-plan-team":   2,
		"xai.consumer-subscription": 1,
	}
	gotRoutes := make(map[upstream.OfferingID]int)
	for _, entry := range document.Entries {
		if _, isSubscription := wantRoutes[entry.Route.Offering.ID]; !isSubscription {
			continue
		}
		gotRoutes[entry.Route.Offering.ID]++
		if entry.Implementation.AdapterID != "" || entry.Route.Offering.Entitlement.AllowsAutomaticPAYGSwitch {
			t.Errorf("control record %q has adapter or PAYG fallback", entry.ID)
		}
		if entry.Boundaries.Production != catalog.ProductionProhibited || entry.Boundaries.Quota != catalog.QuotaSubscriptionWindow || entry.Boundaries.Expiry != catalog.ExpirySubscriptionPeriod {
			t.Errorf("control record %q boundaries = %#v", entry.ID, entry.Boundaries)
		}
		if entry.Route.Offering.ID == "xai.consumer-subscription" {
			if entry.Evidence.Status != catalog.EvidenceUnverified || entry.Implementation.Status != catalog.ImplementationResearchOnly ||
				entry.Route.Offering.Entitlement.AllowedUsage != upstream.AllowedUsageOfficialClientOnly {
				t.Errorf("xAI consumer record = %#v", entry)
			}
			continue
		}
		if entry.Evidence.Status != catalog.EvidenceFresh || entry.Implementation.Status != catalog.ImplementationPlanned ||
			entry.Route.Offering.Entitlement.AllowedUsage != upstream.AllowedUsageInteractiveCodingOnly ||
			!entry.Route.Offering.Entitlement.RequiresExplicitContext || !entry.Route.Offering.Entitlement.RequiresClientIdentity {
			t.Errorf("interactive control record %q policy/status is incomplete", entry.ID)
		}
	}
	for offering, want := range wantRoutes {
		if got := gotRoutes[offering]; got != want {
			t.Errorf("offering %q route count = %d, want %d", offering, got, want)
		}
	}
}

func TestSubscriptionKeyFamiliesAndSavingsPlanBoundary(t *testing.T) {
	wantPrefixes := map[upstream.OfferingID][]string{
		"minimax.token-plan":  {"sk-cp-"},
		"mimo.token-plan":     {"tp-"},
		"alibaba.coding-plan": {"sk-sp-"},
	}
	seen := make(map[upstream.OfferingID]bool)
	for _, entry := range catalog.DefaultDocument().Entries {
		if entry.Implementation.Callable || entry.Route.Offering.ID == "xai.consumer-subscription" {
			continue
		}
		if want, guarded := wantPrefixes[entry.Route.Offering.ID]; guarded {
			seen[entry.Route.Offering.ID] = true
			if len(entry.Route.Credential.KeyPrefixes) != len(want) || entry.Route.Credential.KeyPrefixes[0] != want[0] {
				t.Errorf("offering %q key prefixes = %v, want %v", entry.Route.Offering.ID, entry.Route.Credential.KeyPrefixes, want)
			}
		}
		if entry.Route.Offering.BillingPlan != nil {
			t.Errorf("subscription route %q incorrectly represents a savings plan", entry.ID)
		}
	}
	for offering := range wantPrefixes {
		if !seen[offering] {
			t.Errorf("key-prefix offering %q not found", offering)
		}
	}
}
