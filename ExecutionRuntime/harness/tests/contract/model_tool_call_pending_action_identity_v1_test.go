package contract_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestIdentityAndFactIDsUseIndependentCanonicalDomains(t *testing.T) {
	identity, fact, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	if !strings.HasPrefix(identity.ID, contract.ModelToolCallPendingActionIdentityIDPrefixV1) || !strings.HasPrefix(fact.FactID, contract.SettledTurnDomainResultFactIDPrefixV3) || identity.ID == fact.FactID {
		t.Fatalf("IDs are not domain-separated: %q / %q", identity.ID, fact.FactID)
	}
	sourceDigest, err := identity.SourceKey.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity, _ := contract.DeriveModelToolCallPendingActionIdentityIDV1(sourceDigest)
	wantFact, _ := contract.DeriveSettledTurnDomainResultFactIDV3(sourceDigest)
	if identity.ID != wantIdentity || fact.FactID != wantFact {
		t.Fatal("sealed IDs do not match their public derivation helpers")
	}
}

func TestIdentityAndFactRefsRejectForgedOrDomainMixedIDs(t *testing.T) {
	identity, fact, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	ref, err := fact.RefV3()
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*contract.SettledTurnDomainResultFactRefV3){
		"identity-id-is-fact-id": func(value *contract.SettledTurnDomainResultFactRefV3) {
			value.IdentityRef.ID = value.FactID
		},
		"fact-id-is-identity-id": func(value *contract.SettledTurnDomainResultFactRefV3) {
			value.FactID = value.IdentityRef.ID
		},
		"identity-id-forged": func(value *contract.SettledTurnDomainResultFactRefV3) {
			value.IdentityRef.ID = contract.ModelToolCallPendingActionIdentityIDPrefixV1 + string(testkit.Digest("forged-identity"))
		},
		"fact-id-forged": func(value *contract.SettledTurnDomainResultFactRefV3) {
			value.FactID = contract.SettledTurnDomainResultFactIDPrefixV3 + string(testkit.Digest("forged-fact"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			forged := ref
			mutate(&forged)
			if err := forged.Validate(); err == nil {
				t.Fatal("forged or domain-mixed IDs passed exact ref validation")
			}
		})
	}

	identityRef, err := identity.RefV1(fact.ContentDigest)
	if err != nil {
		t.Fatal(err)
	}
	identityRef.ID = fact.FactID
	if err := identityRef.Validate(); err == nil {
		t.Fatal("IdentityRef accepted the FactID namespace")
	}
}

func TestSettledTurnContentDigestHasExactlyFourFieldsAndEnvelopeSeparation(t *testing.T) {
	_, fact, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	base, err := fact.ContentDigestV3()
	if err != nil || base != fact.ContentDigest {
		t.Fatalf("base content digest invalid: %q / %v", base, err)
	}

	contentMutations := map[string]func(*contract.SettledTurnDomainResultFactV3){
		"candidate": func(value *contract.SettledTurnDomainResultFactV3) {
			value.Candidate.Digest = testkit.Digest("changed-candidate")
		},
		"model-projection": func(value *contract.SettledTurnDomainResultFactV3) {
			value.ModelProjection.Digest = testkit.Digest("changed-model-projection")
		},
		"pending-action": func(value *contract.SettledTurnDomainResultFactV3) {
			value.PendingAction.RequestDigest = testkit.Digest("changed-pending-action")
		},
		"identity": func(value *contract.SettledTurnDomainResultFactV3) {
			value.Identity.Digest = testkit.Digest("changed-identity")
		},
	}
	for name, mutate := range contentMutations {
		t.Run(name, func(t *testing.T) {
			changed := fact.Clone()
			mutate(&changed)
			digest, err := changed.ContentDigestV3()
			if err != nil || digest == base {
				t.Fatalf("four-field content mutation did not change digest: %q / %v", digest, err)
			}
		})
	}

	for name, mutate := range map[string]func(*contract.SettledTurnDomainResultFactV3){
		"settlement-owner": func(value *contract.SettledTurnDomainResultFactV3) {
			value.SettlementOwner.ArtifactDigest = testkit.Digest("changed-owner")
		},
		"schema": func(value *contract.SettledTurnDomainResultFactV3) {
			value.Schema.ContentDigest = testkit.Digest("changed-schema")
		},
		"created": func(value *contract.SettledTurnDomainResultFactV3) {
			value.CreatedUnixNano++
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := fact.Clone()
			mutate(&changed)
			contentDigest, contentErr := changed.ContentDigestV3()
			factDigest, factErr := changed.DigestV3()
			if contentErr != nil || factErr != nil || contentDigest != base || factDigest == fact.FactDigest {
				t.Fatalf("envelope separation failed: content=%q fact=%q errors=%v/%v", contentDigest, factDigest, contentErr, factErr)
			}
		})
	}

	body, err := json.Marshal(contract.SettledTurnDomainResultContentV3{Candidate: fact.Candidate, ModelProjection: fact.ModelProjection, PendingAction: fact.PendingAction, Identity: fact.Identity})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "identity_ref") {
		t.Fatal("IdentityRef flowed back into the four-field content body")
	}
	before := fact.ContentDigest
	if _, err := fact.RefV3(); err != nil || fact.ContentDigest != before {
		t.Fatalf("deriving IdentityRef changed content identity: %v", err)
	}
}

func TestIdentitySealsExactCanonicalModelBytesAndFullSourceKey(t *testing.T) {
	identity, fact, projection, pending := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	if len(projection.Observation.Calls) != 1 || string(projection.Observation.Calls[0].CanonicalArguments) != `{"input":"governed"}` {
		t.Fatalf("unexpected canonical arguments: %s", projection.Observation.Calls[0].CanonicalArguments)
	}
	if identity.CanonicalArgumentsDigest != core.DigestBytes(projection.Observation.Calls[0].CanonicalArguments) || identity.PendingActionRequestDigest != pending.RequestDigest {
		t.Fatal("identity does not bind exact canonical Model bytes and PendingAction")
	}
	if fact.Candidate != identity.SourceKey.Candidate || fact.ModelProjection != identity.SourceKey.ModelProjection || fact.SettlementOwner != identity.SourceKey.SettlementOwner {
		t.Fatal("fact lost full SourceKey lineage")
	}
	ref, err := fact.RefV3()
	if err != nil || ref.IdentityRef.SourceKeyDigest != ref.SourceKeyDigest {
		t.Fatalf("fact ref source lineage invalid: %#v / %v", ref, err)
	}
}

func TestIdentityChangesAcrossTenantScopeWithSameProviderCallID(t *testing.T) {
	first, _, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "tenant-a")
	second, _, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "tenant-b")
	if first.CallID != second.CallID || first.ID == second.ID || first.SourceKey.ExecutionScopeDigest == second.SourceKey.ExecutionScopeDigest {
		t.Fatal("cross-tenant same provider call ID aliased one Harness identity")
	}
}

func TestSettledTurnRepositoryLinearizesThreeIndexesAndRecoversLostReply(t *testing.T) {
	identity, fact, projection, pending := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	repository := fakes.NewSettledTurnDomainResultRepositoryV3()
	repository.LoseNextEnsureReply = true
	if _, err := repository.EnsureExact(context.Background(), fact); err == nil {
		t.Fatal("injected reply loss was not reported")
	}
	got, err := repository.EnsureExact(context.Background(), fact)
	if err != nil || got.FactDigest != fact.FactDigest || repository.HistoryLenV3() != 1 {
		t.Fatalf("exact reply-loss recovery failed: %#v / %v", got, err)
	}
	inputClone := fact.Clone()
	if _, err := repository.EnsureExact(context.Background(), inputClone); err != nil {
		t.Fatal(err)
	}
	inputClone.PendingAction.Payload.Inline[0] ^= 1
	got.PendingAction.Payload.Inline[0] ^= 1
	ref, _ := fact.RefV3()
	stored, err := repository.InspectExact(context.Background(), ref)
	if err != nil || stored.PendingAction.Payload.ContentDigest != identity.PayloadContentDigest || string(stored.PendingAction.Payload.Inline) != `{"input":"governed"}` {
		t.Fatal("repository returned aliased immutable content")
	}

	changedIdentity, err := contract.SealModelToolCallPendingActionIdentityV1(identity.SourceKey, projection, pending, identity.CreatedUnixNano+1, identity.NotAfterUnixNano+1)
	if err != nil {
		t.Fatal(err)
	}
	changedFact, err := contract.SealSettledTurnDomainResultFactV3(changedIdentity, pending, fact.Schema, changedIdentity.CreatedUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	if changedIdentity.ID != identity.ID || changedFact.FactID != fact.FactID || changedFact.FactDigest == fact.FactDigest {
		t.Fatal("conflict fixture did not preserve source keys while changing content")
	}
	if _, err := repository.EnsureExact(context.Background(), changedFact); err == nil {
		t.Fatal("same three-key identity with changed content was accepted")
	}
}

func TestSettledTurnRepositoryTypedNilIsUnavailable(t *testing.T) {
	_, fact, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	ref, err := fact.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	var repository *fakes.SettledTurnDomainResultRepositoryV3
	var port harnessports.SettledTurnDomainResultRepositoryV3 = repository
	if _, err := port.EnsureExact(context.Background(), fact); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil Ensure error = %v", err)
	}
	if _, err := port.InspectExact(context.Background(), ref); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil Inspect error = %v", err)
	}
}

func TestSettledTurnRepositoryConcurrentExactEnsureCreatesOneHistoryRecord(t *testing.T) {
	_, fact, _, _ := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	repository := fakes.NewSettledTurnDomainResultRepositoryV3()
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			got, err := repository.EnsureExact(context.Background(), fact)
			if err == nil && got.FactDigest != fact.FactDigest {
				err = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent Ensure returned another fact")
			}
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if repository.HistoryLenV3() != 1 {
		t.Fatalf("history length = %d", repository.HistoryLenV3())
	}
}

func TestSettledTurnRepositoryConcurrentDifferentContentOnOneSourceHasOneWinner(t *testing.T) {
	identity, fact, projection, pending := identityFactFixtureV1(t, time.Unix(1_750_000_000, 0), "")
	repository := fakes.NewSettledTurnDomainResultRepositoryV3()
	const workers = 64
	facts := make([]contract.SettledTurnDomainResultFactV3, workers)
	for index := range workers {
		changedIdentity, err := contract.SealModelToolCallPendingActionIdentityV1(identity.SourceKey, projection, pending, identity.CreatedUnixNano+int64(index), identity.NotAfterUnixNano+int64(index))
		if err != nil {
			t.Fatal(err)
		}
		facts[index], err = contract.SealSettledTurnDomainResultFactV3(changedIdentity, pending, fact.Schema, changedIdentity.CreatedUnixNano)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := repository.EnsureExact(context.Background(), facts[index])
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 || repository.HistoryLenV3() != 1 {
		t.Fatalf("different content winners=%d history=%d", successes, repository.HistoryLenV3())
	}
}

func identityFactFixtureV1(t *testing.T, now time.Time, tenant string) (contract.ModelToolCallPendingActionIdentityV1, contract.SettledTurnDomainResultFactV3, modelinvoker.ToolCallCandidateObservationProjectionV1, contract.PendingActionV2) {
	t.Helper()
	_, candidate := testkit.GovernedFactsV2(now)
	if tenant != "" {
		candidate.Run.Scope.Identity.TenantID = core.TenantID(tenant)
		candidate.Endpoint.Scope.Identity.TenantID = core.TenantID(tenant)
		candidate.Endpoint, _ = contract.NewEndpointRefV2(candidate.Endpoint.ID, candidate.Endpoint.Scope, candidate.Endpoint.Binding)
	}
	candidateRef, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := contract.NewPendingActionV2("action-g6a", "praxis.tool/execute", candidate.Input, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	call := modelinvoker.FunctionCall{ID: "provider-call-1", Name: "lookup", Arguments: json.RawMessage(`{"input":"governed"}`)}
	response := modelinvoker.Response{ID: "response-g6a", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(testkit.Digest("model-invocation"), response)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("model-execution-g6a", 1, "response-g6a", observation)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	source := contract.ModelToolCallPendingActionIdentitySourceKeyV1{ExecutionScopeDigest: scopeDigest, RunID: string(candidate.Run.RunID), SessionID: candidate.SessionRef, Turn: candidate.Turn, Candidate: candidateRef, ModelProjection: projection.Ref, CallOrdinal: contract.ModelToolCallOrdinalV1{EncodingVersion: contract.ModelToolCallOrdinalEncodingVersionV1, Present: true, Value: 0}, SettlementOwner: candidate.Provider}
	identity, err := contract.SealModelToolCallPendingActionIdentityV1(source, projection, pending, now.UnixNano(), now.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeports.SchemaRefV2{Namespace: contract.SettledTurnDomainResultSchemaNamespaceV3, Name: contract.SettledTurnDomainResultSchemaNameV3, Version: contract.SettledTurnDomainResultSchemaVersionV3, MediaType: "application/json", ContentDigest: testkit.Digest("settled-turn-v3-schema")}
	fact, err := contract.SealSettledTurnDomainResultFactV3(identity, pending, schema, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return identity, fact, projection, pending
}
