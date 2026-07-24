// Package conformance provides reusable owner-semantic fixtures and repository
// checks. Passing them does not certify production durability, availability,
// cross-process CAS, operational SLA, or trusted extension resolution.
package conformance

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type TestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func DigestV1(value string) core.Digest {
	return core.DigestBytes([]byte("agent-definition-test:" + value))
}

func RefV1(value string) contract.ObjectRefV1 {
	return contract.ObjectRefV1{ID: value, Revision: 1, Digest: DigestV1(value)}
}

func CatalogV1() contract.ValidationCatalogV1 {
	kinds := contract.RequiredCoreKindsV1()
	capabilities := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		capabilities = append(capabilities, "praxis/"+strings.ReplaceAll(strings.TrimPrefix(kind, "praxis/"), "/", "-"))
	}
	return contract.ValidationCatalogV1{Kinds: kinds, Capabilities: capabilities, RegisteredExtensionKeys: []string{"example/required", "example/optional"}}
}

func SourceV1(now time.Time) contract.AgentDefinitionSourceV1 {
	catalog := CatalogV1()
	coreKinds := contract.RequiredCoreKindsV1()
	components := make([]contract.ComponentRequirementV1, 0, len(coreKinds))
	for index, kind := range coreKinds {
		components = append(components, contract.ComponentRequirementV1{
			ComponentID: fmt.Sprintf("agent/component-%02d", index+1), Kind: kind,
			SemanticVersion: contract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
			ContractName:    "praxis/component-contract", ContractVersion: contract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
			RequiredCapabilities: []string{catalog.Capabilities[index]}, Required: true, SupportMode: contract.SupportModeProductionV1,
			LocalityConstraint: contract.LocalityHostControlPlaneV1, ResidualPolicy: contract.ResidualPolicyV1{}, DependencyIDs: []string{},
		})
	}
	return contract.AgentDefinitionSourceV1{
		ContractVersion: contract.ContractVersionV1, DefinitionID: "agent/example", Revision: 1,
		IdentityRef: RefV1("identity/example"), ProfileSelectionRef: RefV1("profile/example"), Components: components,
		PolicyRefs: contract.PolicyRefsV1{Runtime: RefV1("policy/runtime"), Authority: RefV1("policy/authority"), Review: RefV1("policy/review"), Budget: RefV1("policy/budget"), Sandbox: RefV1("policy/sandbox"), Context: RefV1("policy/context"), Continuity: RefV1("policy/continuity"), ToolMCP: RefV1("policy/tool-mcp"), MemoryKnowledge: RefV1("policy/memory-knowledge")},
		SecretRefs: []contract.SecretRefV1{}, ProvenanceRef: RefV1("provenance/example"), ApprovalRef: RefV1("approval/example"),
		EffectiveWindow: contract.EffectiveWindowV1{NotBeforeUnixNano: now.Add(-time.Hour).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano()},
		Extensions:      []contract.ExtensionV1{}, ChangeReason: "initial production definition",
	}
}

func RunRepositoryConformanceV1(t TestingT, repository ports.DefinitionRepositoryV1, catalog contract.ValidationCatalogV1, now time.Time) {
	t.Helper()
	if isTypedNilV1(repository) {
		t.Fatalf("repository is typed nil")
		return
	}
	definition, err := contract.SealDefinitionV1(SourceV1(now), catalog, now.UnixNano())
	if err != nil {
		t.Fatalf("seal fixture: %v", err)
		return
	}
	request := ports.CreateDefinitionRequestV1{Definition: definition}
	lostReply := createAndLoseReplyV1(repository, request)
	if !core.HasCategory(lostReply, core.ErrorUnavailable) {
		t.Fatalf("lost-reply injection did not classify unavailable: %v", lostReply)
		return
	}
	recovered, err := repository.InspectDefinitionRevisionV1(context.Background(), definition.DefinitionID, definition.Revision)
	if err != nil || recovered.Digest != definition.Digest {
		t.Fatalf("lost-reply exact revision recovery: %#v %v", recovered, err)
		return
	}
	replayed, err := repository.CreateDefinitionV1(context.Background(), request)
	if err != nil || replayed.Definition.Digest != recovered.Digest {
		t.Fatalf("same-content replay: %#v %v", replayed, err)
		return
	}
	inspected, err := repository.InspectExactDefinitionV1(context.Background(), definition.RefV1())
	if err != nil || inspected.Digest != definition.Digest {
		t.Fatalf("exact inspect: %#v %v", inspected, err)
		return
	}
	replayed.Definition.Components[0].ComponentID = "attacker/mutated"
	cloneChecked, err := repository.InspectExactDefinitionV1(context.Background(), definition.RefV1())
	if err != nil || cloneChecked.Components[0].ComponentID == "attacker/mutated" {
		t.Fatalf("repository result aliases immutable history: %#v %v", cloneChecked, err)
		return
	}
	current, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, now.UnixNano())
	if err != nil || current.Definition != definition.RefV1() || current.State != contract.DefinitionCurrentActiveV1 {
		t.Fatalf("current inspect: %#v %v", current, err)
		return
	}

	changedSource := contract.CloneSourceV1(definition.AgentDefinitionSourceV1)
	changedSource.ChangeReason = "changed content at the same revision"
	changed, err := contract.SealDefinitionV1(changedSource, catalog, now.UnixNano())
	if err != nil {
		t.Fatalf("seal changed-content fixture: %v", err)
		return
	}
	if _, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: changed}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same revision changed content was not conflict: %v", err)
		return
	}

	revisionTwoSource := contract.CloneSourceV1(definition.AgentDefinitionSourceV1)
	revisionTwoSource.Revision = 2
	revisionTwoSource.ChangeReason = "conformance revision two"
	revisionTwo, err := contract.SealDefinitionV1(revisionTwoSource, catalog, now.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("seal revision two fixture: %v", err)
		return
	}
	if _, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: revisionTwo, ExpectedCurrentRevision: 0}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("revision CAS accepted stale expected revision: %v", err)
		return
	}
	revisionTwoResult, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: revisionTwo, ExpectedCurrentRevision: current.Revision})
	if err != nil || revisionTwoResult.Current.Definition != revisionTwo.RefV1() {
		t.Fatalf("revision CAS did not advance exact current: %#v %v", revisionTwoResult, err)
		return
	}

	expiredAt := time.Unix(0, revisionTwo.EffectiveWindow.NotAfterUnixNano).Add(time.Nanosecond)
	expired, err := repository.InspectCurrentDefinitionV1(context.Background(), revisionTwo.DefinitionID, expiredAt.UnixNano())
	if err != nil || expired.State != contract.DefinitionCurrentExpiredV1 {
		t.Fatalf("current expiry projection: %#v %v", expired, err)
		return
	}
	if _, err := repository.InspectCurrentDefinitionV1(context.Background(), revisionTwo.DefinitionID, now.Add(time.Minute).UnixNano()); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("current clock ABA was not rejected: %v", err)
		return
	}
	revokedAt := expiredAt.Add(time.Minute)
	revoked, err := repository.RevokeDefinitionV1(context.Background(), ports.RevokeDefinitionRequestV1{DefinitionID: revisionTwo.DefinitionID, ExpectedCurrentRevision: revisionTwoResult.Current.Revision, RevokedUnixNano: revokedAt.UnixNano(), Reason: "conformance revoke"})
	if err != nil || revoked.State != contract.DefinitionCurrentRevokedV1 {
		t.Fatalf("current revoke: %#v %v", revoked, err)
		return
	}
}

func createAndLoseReplyV1(repository ports.DefinitionRepositoryV1, request ports.CreateDefinitionRequestV1) error {
	if _, err := repository.CreateDefinitionV1(context.Background(), request); err != nil {
		return err
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "conformance discarded committed create reply")
}

func isTypedNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
