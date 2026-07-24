package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestCompareRecipesOfflineOperationExactCodecAndNoAliasV1(t *testing.T) {
	request := compareRequestFixtureV1(t)
	sealed, err := SealCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeCompareRecipesRequestV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCompareRecipesRequestV1(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sealed, decoded) {
		t.Fatal("compare request codec drift")
	}
	response, err := CompareRecipesV1(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Comparison.Changes) != 2 || response.Comparison.Changes[0].FieldPath != "rules/instruction" || response.Comparison.Changes[1].FieldPath != "semantic_version" {
		t.Fatalf("unexpected canonical changes: %#v", response.Comparison.Changes)
	}
	encoded, err := EncodeCompareRecipesResponseV1(context.Background(), response)
	if err != nil || !bytes.Contains(encoded, []byte(`"comparison"`)) {
		t.Fatalf("encode comparison: %v %q", err, encoded)
	}

	sealed.BaseRecipe.Rules[0].MaxTokens++
	again, err := CompareRecipesV1(context.Background(), decoded)
	if err != nil || !reflect.DeepEqual(response, again) {
		t.Fatalf("caller alias changed result: %v", err)
	}
	if response.Comparison.Changes[0].BeforeDigest == nil {
		t.Fatal("expected before digest")
	}
	*response.Comparison.Changes[0].BeforeDigest = testkit.D("caller-mutation")
	again, err = CompareRecipesV1(context.Background(), decoded)
	if err != nil || *again.Comparison.Changes[0].BeforeDigest == *response.Comparison.Changes[0].BeforeDigest {
		t.Fatalf("response digest pointer aliased: %v", err)
	}
}

func TestCompareRecipesStrictPresenceLimitsCancelAndTamperV1(t *testing.T) {
	request := compareRequestFixtureV1(t)
	payload, err := EncodeCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(payload, []byte(`"max_tokens":100`), []byte(`"max_tokens":100,"max_tokens":100`), 1)
	if _, err := DecodeCompareRecipesRequestV1(context.Background(), duplicate); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("recursive duplicate key must fail: %v", err)
	}
	var nullDocument map[string]any
	if err := json.Unmarshal(payload, &nullDocument); err != nil {
		t.Fatal(err)
	}
	nullDocument["base_recipe"] = nil
	nullBase, err := json.Marshal(nullDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCompareRecipesRequestV1(context.Background(), nullBase); err == nil {
		t.Fatal("null base recipe must fail closed")
	}

	tooSmall := request
	tooSmall.Meta.RequestDigest = ""
	tooSmall.Meta.Limits.MaxRecipes = 1
	if _, err := SealCompareRecipesRequestV1(context.Background(), tooSmall); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("two-recipe limit must fail: %v", err)
	}

	sealed, err := SealCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	tampered := sealed
	tampered.CandidateRecipe.SemanticVersion = "9.9.9"
	if response, err := CompareRecipesV1(context.Background(), tampered); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(response, CompareRecipesResponseV1{}) {
		t.Fatalf("digest tamper must return zero response: %#v %v", response, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if response, err := CompareRecipesV1(ctx, sealed); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(response, CompareRecipesResponseV1{}) {
		t.Fatalf("cancel must return zero response: %#v %v", response, err)
	}

	response, err := CompareRecipesV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	response.Comparison.ComparisonDigest = testkit.D("tampered")
	if encoded, err := EncodeCompareRecipesResponseV1(context.Background(), response); !errors.Is(err, contract.ErrConflict) || encoded != nil {
		t.Fatalf("tampered response must not encode: %q %v", encoded, err)
	}
}

func TestCompareRecipesConcurrentDeterministicV1(t *testing.T) {
	request := compareRequestFixtureV1(t)
	sealed, err := SealCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want, err := CompareRecipesV1(context.Background(), sealed)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	for index := 0; index < 64; index++ {
		go func() {
			got, err := CompareRecipesV1(context.Background(), sealed)
			if err == nil && !reflect.DeepEqual(want, got) {
				err = errors.New("comparison drift")
			}
			errs <- err
		}()
	}
	for index := 0; index < 64; index++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompareRecipesIdenticalAndLifetimeFailClosedV1(t *testing.T) {
	request := compareRequestFixtureV1(t)
	request.CandidateRecipe = cloneRecipeV1(request.BaseRecipe)
	sealed, err := SealCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := CompareRecipesV1(context.Background(), sealed)
	if err != nil || response.Comparison.Changes == nil || len(response.Comparison.Changes) != 0 {
		t.Fatalf("identical recipes must produce present empty changes: %#v %v", response.Comparison.Changes, err)
	}
	encoded, err := EncodeCompareRecipesResponseV1(context.Background(), response)
	if err != nil || !bytes.Contains(encoded, []byte(`"changes":[]`)) {
		t.Fatalf("empty change presence drift: %q %v", encoded, err)
	}

	request = compareRequestFixtureV1(t)
	request.ExpiresUnixNano = request.BaseRecipe.ExpiresUnixNano + 1
	sealed, err = SealCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := CompareRecipesV1(context.Background(), sealed); !errors.Is(err, contract.ErrExpired) || !reflect.DeepEqual(response, CompareRecipesResponseV1{}) {
		t.Fatalf("comparison must not extend recipe lifetime: %#v %v", response, err)
	}
}

func compareRequestFixtureV1(t *testing.T) CompareRecipesRequestV1 {
	t.Helper()
	meta := requestMetaV1(OfflineCompareRecipesV1, "compare-recipes-1")
	meta.Limits.MaxRecipes = 2
	base := testkit.Recipe()
	candidate := testkit.Recipe()
	candidate.SemanticVersion = "1.1.0"
	candidate.Rules[0].MaxTokens = 90
	return CompareRecipesRequestV1{Meta: meta, BaseRecipe: base, CandidateRecipe: candidate, CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000_000}
}
