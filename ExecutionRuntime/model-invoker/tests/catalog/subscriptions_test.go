package catalog_test

import (
	"slices"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestSubscriptionCatalogRecordsSeparateApprovedCallableAndBlockedRoutes(t *testing.T) {
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
		if entry.Route.Offering.Entitlement.AllowsAutomaticPAYGSwitch {
			t.Errorf("subscription route %q has PAYG fallback", entry.ID)
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
		if entry.Route.Offering.ID == "zai.glm-coding-plan" {
			if entry.Implementation.Callable || entry.Implementation.Status != catalog.ImplementationResearchOnly || entry.Route.Offering.Entitlement.AllowedUsage != upstream.AllowedUsageOfficialClientOnly {
				t.Errorf("GLM official-client-only record = %#v", entry)
			}
			continue
		}
		if entry.Evidence.Status != catalog.EvidenceFresh ||
			entry.Route.Offering.Entitlement.AllowedUsage != upstream.AllowedUsageInteractiveCodingOnly ||
			!entry.Route.Offering.Entitlement.RequiresExplicitContext || !entry.Route.Offering.Entitlement.RequiresClientIdentity {
			t.Errorf("interactive route %q policy/status is incomplete", entry.ID)
		}
		if entry.Implementation.Callable || entry.Implementation.Status != catalog.ImplementationImplementedOffline || entry.Implementation.AdapterID == "" ||
			entry.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
			t.Errorf("host-blocked route %q lost its implemented candidate boundary", entry.ID)
		}
	}
	for offering, want := range wantRoutes {
		if got := gotRoutes[offering]; got != want {
			t.Errorf("offering %q route count = %d, want %d", offering, got, want)
		}
	}
}

func TestRestrictedSubscriptionRoutesUseExactCatalogModelSets(t *testing.T) {
	codingPlan := []string{
		"qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5",
		"qwen3.5-plus", "qwen3-max-2026-01-23", "qwen3-coder-next", "qwen3-coder-plus", "glm-4.7",
	}
	tokenPlanTeam := []string{
		"qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus", "qwen3.6-flash",
		"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3.2",
		"kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5",
		"glm-5.2", "glm-5.1", "glm-5", "MiniMax-M2.5",
	}
	wantByOffering := map[upstream.OfferingID][]string{
		"kimi.code-membership":    {"kimi-for-coding"},
		"minimax.token-plan":      {"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"},
		"mimo.token-plan":         {"mimo-v2.5", "mimo-v2.5-pro"},
		"alibaba.coding-plan":     codingPlan,
		"alibaba.token-plan-team": tokenPlanTeam,
	}
	seen := 0
	for _, entry := range catalog.DefaultDocument().Entries {
		want, ok := wantByOffering[entry.Route.Offering.ID]
		if !ok {
			continue
		}
		seen++
		if entry.ModelDiscovery.Method != catalog.ModelDiscoveryStaticCatalog || entry.ModelDiscovery.AliasPolicy != catalog.ModelAliasExactProviderID {
			t.Errorf("route %q does not enforce exact static model IDs", entry.ID)
		}
		got := make([]string, 0, len(entry.ModelDiscovery.Aliases))
		for _, alias := range entry.ModelDiscovery.Aliases {
			got = append(got, alias.ProviderModelRef)
		}
		if !slices.Equal(got, want) {
			t.Errorf("route %q models = %v, want %v", entry.ID, got, want)
		}
	}
	if seen != 16 {
		t.Fatalf("exact subscription model routes = %d, want 16", seen)
	}
}

func TestSubscriptionKeyFamiliesAndSavingsPlanBoundary(t *testing.T) {
	wantPrefixes := map[upstream.OfferingID][]string{
		"minimax.token-plan":      {"sk-cp-"},
		"mimo.token-plan":         {"tp-"},
		"alibaba.coding-plan":     {"sk-sp-"},
		"alibaba.token-plan-team": {"sk-sp-"},
	}
	seen := make(map[upstream.OfferingID]bool)
	for _, entry := range catalog.DefaultDocument().Entries {
		if entry.Route.Offering.ID == "xai.consumer-subscription" {
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

func TestSubscriptionCredentialHeadersMatchApprovedProtocolWireAuth(t *testing.T) {
	want := map[upstream.RouteID]struct {
		header string
		scheme string
	}{
		"kimi.code-membership.global.chat_completions": {header: "Authorization", scheme: "Bearer"},
		"kimi.code-membership.global.messages":         {header: "x-api-key"},
		"minimax.token-plan.global.chat_completions":   {header: "Authorization", scheme: "Bearer"},
		"minimax.token-plan.global.messages":           {header: "x-api-key"},
		"mimo.token-plan.cn.messages":                  {header: "Authorization", scheme: "Bearer"},
		"alibaba.coding-plan.cn.messages":              {header: "Authorization", scheme: "Bearer"},
		"alibaba.token-plan-team.cn-beijing.messages":  {header: "Authorization", scheme: "Bearer"},
	}
	seen := make(map[upstream.RouteID]bool, len(want))
	for _, entry := range catalog.DefaultDocument().Entries {
		expected, ok := want[entry.ID]
		if !ok {
			continue
		}
		seen[entry.ID] = true
		credential := entry.Route.Credential
		if credential.AuthHeader != expected.header || credential.AuthScheme != expected.scheme {
			t.Errorf("route %q auth = %q %q, want %q %q", entry.ID, credential.AuthHeader, credential.AuthScheme, expected.header, expected.scheme)
		}
	}
	for routeID := range want {
		if !seen[routeID] {
			t.Errorf("auth assertion route %q not found", routeID)
		}
	}
}
