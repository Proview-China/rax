package kernel_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestCompareContextRecipesIdenticalAndFullDiffV1(t *testing.T) {
	base := testkit.Recipe()
	identical, err := kernel.CompareContextRecipesV1(context.Background(), base, base, testkit.Now, testkit.Now+1_000)
	if err != nil {
		t.Fatal(err)
	}
	if identical.Changes == nil || len(identical.Changes) != 0 || identical.BaseRecipeRef != identical.CandidateRef {
		t.Fatalf("unexpected identical comparison: %#v", identical)
	}
	if digest, err := identical.DigestValue(); err != nil || digest != identical.ComparisonDigest {
		t.Fatalf("comparison digest mismatch: %s %v", digest, err)
	}

	candidate := base
	candidate.ID = "recipe-2"
	candidate.SemanticVersion = "2.0.0"
	candidate.Revision = 2
	candidate.Owner.BindingDigest = testkit.D("new-owner")
	candidate.Rules = []contract.FragmentRule{
		{Kind: contract.FragmentArtifactInline, Region: contract.RegionSemiStable, MaxTokens: 100, Degradation: contract.DegradeExclude},
		{Kind: contract.FragmentInstruction, Region: contract.RegionStablePrefix, Required: true, MaxTokens: 90, Degradation: contract.DegradeReject},
		{Kind: contract.FragmentToolResult, Region: contract.RegionDynamicTail, MaxTokens: 80, Degradation: contract.DegradeExclude},
	}
	candidate.Budget = contract.BudgetPolicy{TotalTokens: 160, StablePrefixMax: 90, SemiStableMax: 100, DynamicTailMax: 80}
	candidate.RenderVersion = "render-v2"
	candidate.CreatedUnixNano++
	candidate.ExpiresUnixNano--

	comparison, err := kernel.CompareContextRecipesV1(context.Background(), base, candidate, testkit.Now, testkit.Now+1_000)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		"budget", "created_unix_nano", "expires_unix_nano", "owner", "recipe_id", "render_version", "revision",
		"rules/conversation", "rules/instruction", "rules/tool_result", "rules_order", "semantic_version",
	}
	gotPaths := make([]string, len(comparison.Changes))
	for index := range comparison.Changes {
		gotPaths[index] = comparison.Changes[index].FieldPath
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("unexpected diff paths\nwant=%v\ngot=%v", wantPaths, gotPaths)
	}
	if digest, err := comparison.DigestValue(); err != nil || digest != comparison.ComparisonDigest {
		t.Fatalf("comparison digest mismatch: %s %v", digest, err)
	}
	valueType := reflect.TypeOf(comparison)
	for _, forbidden := range []string{"Better", "Compatible", "Publish", "ReviewVerdict", "TaskSucceeded"} {
		if _, ok := valueType.FieldByName(forbidden); ok {
			t.Fatalf("structural comparison claims forbidden conclusion %s", forbidden)
		}
	}
}

func TestCompareContextRecipesDeterministicConcurrentV1(t *testing.T) {
	base := testkit.Recipe()
	candidate := base
	candidate.SemanticVersion = "1.1.0"
	candidate.Revision = 2
	const workers = 64
	results := make(chan contract.Digest, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			comparison, err := kernel.CompareContextRecipesV1(context.Background(), base, candidate, testkit.Now, testkit.Now+1_000)
			if err != nil {
				errors <- err
				return
			}
			results <- comparison.ComparisonDigest
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var expected contract.Digest
	for digest := range results {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatalf("concurrent comparison drift: %s != %s", digest, expected)
		}
	}
}

func TestCompareContextRecipesCanceledV1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := kernel.CompareContextRecipesV1(ctx, testkit.Recipe(), testkit.Recipe(), testkit.Now, testkit.Now+1)
	if err != context.Canceled || result.ComparisonDigest != "" {
		t.Fatalf("want canceled zero report, got %#v %v", result, err)
	}
}
