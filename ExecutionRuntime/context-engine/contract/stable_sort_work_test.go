package contract

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestStableSortCandidatesV1AdversarialWorkBound(t *testing.T) {
	recipe := ContextRecipe{Rules: []FragmentRule{
		{Kind: FragmentInstruction}, {Kind: FragmentArtifactInline}, {Kind: FragmentConversation},
	}}
	base := make([]ContextCandidate, 512)
	for index := range base {
		base[index] = ContextCandidate{ID: fmt.Sprintf("candidate-%03d", index%17), SourceRef: fmt.Sprintf("source-%03d", index%29), Kind: recipe.Rules[index%len(recipe.Rules)].Kind}
	}
	fixtures := map[string][]ContextCandidate{
		"sorted":    append([]ContextCandidate(nil), base...),
		"reverse":   reverseCandidatesV1(base),
		"all_equal": repeatCandidateV1(base[0], len(base)),
		"shuffled":  shuffledCandidatesV1(base, 20260716),
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			comparisons := 0
			got := stableSortCandidatesObservedV1(fixture, recipe, func() { comparisons++ })
			if len(got) != 512 || comparisons == 0 || comparisons > 25_000 {
				t.Fatalf("unexpected stable-sort work: len=%d comparisons=%d", len(got), comparisons)
			}
			t.Logf("go stable sort fixture=%s comparisons=%d", name, comparisons)
		})
	}
}

func reverseCandidatesV1(values []ContextCandidate) []ContextCandidate {
	result := append([]ContextCandidate(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func repeatCandidateV1(value ContextCandidate, count int) []ContextCandidate {
	result := make([]ContextCandidate, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func shuffledCandidatesV1(values []ContextCandidate, seed int64) []ContextCandidate {
	result := append([]ContextCandidate(nil), values...)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(result), func(i, j int) { result[i], result[j] = result[j], result[i] })
	return result
}
