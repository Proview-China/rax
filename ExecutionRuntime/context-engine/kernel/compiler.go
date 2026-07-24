package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ReferenceStore interface {
	Put([]byte) (contract.ContentRef, error)
	Get(contract.ContentRef) ([]byte, error)
}

type CompileRequest struct {
	AttemptID       string
	ManifestID      string
	FrameID         string
	GenerationID    string
	Generation      uint64
	Recipe          contract.ContextRecipe
	Execution       contract.ExecutionBinding
	Candidates      []contract.ContextCandidate
	ParentFrame     *contract.FactRef
	CreatedUnixNano int64
	ExpiresUnixNano int64
}

type CompileResult struct {
	Manifest contract.ContextManifest
	Frame    contract.ContextFrame
}

type evaluatedCandidate struct {
	candidate contract.ContextCandidate
	ref       contract.FactRef
	rule      contract.FragmentRule
	content   []byte
	required  bool
	decision  *contract.AdmissionDecision
}

type renderedFragment struct {
	Position        uint32                `json:"position"`
	Kind            contract.FragmentKind `json:"kind"`
	CandidateDigest contract.Digest       `json:"candidate_digest"`
	Content         []byte                `json:"content"`
}

func Compile(store ReferenceStore, request CompileRequest) (CompileResult, error) {
	if store == nil {
		return CompileResult{}, fmt.Errorf("%w: compile request", contract.ErrInvalid)
	}
	return CompileStagedV1(context.Background(), legacyReferenceStoreAdapterV1{store: store}, request, legacyCompileWorkLimitsV1())
}

func CompileStagedV1(ctx context.Context, store ContextAwareReferenceStoreV1, request CompileRequest, limits CompileWorkLimitsV1) (CompileResult, error) {
	if err := checkContextV1(ctx); err != nil {
		return CompileResult{}, err
	}
	if store == nil || limits.Validate() != nil {
		return CompileResult{}, fmt.Errorf("%w: compile limits", contract.ErrInvalid)
	}
	if len(request.Candidates) > int(limits.MaxCandidates) || request.Recipe.Budget.TotalTokens > limits.MaxTotalTokens {
		return CompileResult{}, fmt.Errorf("%w: compile limits", contract.ErrLimitExceeded)
	}
	if store == nil || request.Recipe.Validate() != nil || request.Execution.Validate() != nil || request.Generation == 0 || request.CreatedUnixNano <= 0 || request.ExpiresUnixNano <= request.CreatedUnixNano {
		return CompileResult{}, fmt.Errorf("%w: compile request", contract.ErrInvalid)
	}
	for _, id := range []string{request.AttemptID, request.ManifestID, request.FrameID, request.GenerationID} {
		if id == "" {
			return CompileResult{}, fmt.Errorf("%w: compile identity", contract.ErrInvalid)
		}
	}
	if request.CreatedUnixNano >= request.Recipe.ExpiresUnixNano || request.ExpiresUnixNano > request.Recipe.ExpiresUnixNano {
		return CompileResult{}, fmt.Errorf("%w: recipe lifetime", contract.ErrExpired)
	}
	if request.ParentFrame != nil && request.ParentFrame.Validate() != nil {
		return CompileResult{}, fmt.Errorf("%w: parent frame", contract.ErrInvalid)
	}

	if err := checkContextV1(ctx); err != nil {
		return CompileResult{}, err
	}
	sorted := contract.StableSortCandidates(request.Candidates, request.Recipe)
	if err := checkContextV1(ctx); err != nil {
		return CompileResult{}, err
	}
	evaluated := make([]evaluatedCandidate, 0, len(sorted))
	requiredKindSelected := make(map[contract.FragmentKind]bool)
	allRefs := make([]contract.FactRef, 0, len(sorted))

	for _, candidate := range sorted {
		if err := checkContextV1(ctx); err != nil {
			return CompileResult{}, err
		}
		digest, err := candidate.DigestValue()
		if err != nil {
			return CompileResult{}, err
		}
		ref := contract.FactRef{ID: candidate.ID, Revision: candidate.Revision, Digest: digest}
		allRefs = append(allRefs, ref)
		rule, hasRule := request.Recipe.Rule(candidate.Kind)
		required := candidate.Required
		if hasRule && rule.Required && !requiredKindSelected[candidate.Kind] {
			required = true
			requiredKindSelected[candidate.Kind] = true
		}
		item := evaluatedCandidate{candidate: candidate, ref: ref, rule: rule, required: required}
		if !hasRule {
			item.decision = decision(ref, contract.AdmissionExcluded, "recipe_rule_absent")
			if required {
				return CompileResult{}, fmt.Errorf("%w: required candidate has no recipe rule", contract.ErrConflict)
			}
			evaluated = append(evaluated, item)
			continue
		}
		if candidate.Execution != request.Execution {
			item.decision = decision(ref, contract.AdmissionRejected, "execution_binding_mismatch")
			if required {
				return CompileResult{}, fmt.Errorf("%w: candidate execution binding", contract.ErrUnauthorized)
			}
			evaluated = append(evaluated, item)
			continue
		}
		if request.CreatedUnixNano >= candidate.ExpiresUnixNano {
			item.decision = decision(ref, contract.AdmissionExcluded, "candidate_expired")
			if required {
				return CompileResult{}, fmt.Errorf("%w: required candidate", contract.ErrExpired)
			}
			evaluated = append(evaluated, item)
			continue
		}
		if candidate.Kind == contract.FragmentInstruction && candidate.Trust != contract.TrustAuthoritativeInstruction {
			item.decision = decision(ref, contract.AdmissionRejected, "untrusted_instruction")
			if required {
				return CompileResult{}, fmt.Errorf("%w: instruction trust", contract.ErrUnauthorized)
			}
			evaluated = append(evaluated, item)
			continue
		}
		content, err := store.GetContextV1(ctx, candidate.Content)
		if err != nil {
			item.decision = decision(ref, contract.AdmissionResidual, "content_unavailable")
			if required {
				return CompileResult{}, fmt.Errorf("%w: required content: %v", contract.ErrUnknown, err)
			}
			evaluated = append(evaluated, item)
			continue
		}
		item.content = content
		evaluated = append(evaluated, item)
	}

	for _, rule := range request.Recipe.Rules {
		if err := checkContextV1(ctx); err != nil {
			return CompileResult{}, err
		}
		if rule.Required && !requiredKindSelected[rule.Kind] {
			return CompileResult{}, fmt.Errorf("%w: required recipe kind %s", contract.ErrUnknown, rule.Kind)
		}
	}

	remainingRequired := map[contract.FrameRegion]uint64{}
	var remainingRequiredTotal uint64
	for _, item := range evaluated {
		if err := checkContextV1(ctx); err != nil {
			return CompileResult{}, err
		}
		if item.decision == nil && item.required {
			remainingRequired[item.rule.Region] += item.candidate.TokenEstimate
			remainingRequiredTotal += item.candidate.TokenEstimate
		}
	}
	if remainingRequired[contract.RegionStablePrefix] > request.Recipe.Budget.StablePrefixMax || remainingRequired[contract.RegionSemiStable] > request.Recipe.Budget.SemiStableMax || remainingRequired[contract.RegionDynamicTail] > request.Recipe.Budget.DynamicTailMax || remainingRequiredTotal > request.Recipe.Budget.TotalTokens {
		return CompileResult{}, fmt.Errorf("%w: required context exceeds budget", contract.ErrConflict)
	}

	usage := map[contract.FrameRegion]uint64{}
	var total uint64
	decisions := make([]contract.AdmissionDecision, 0, len(evaluated))
	fragments := make([]contract.ContextFragment, 0, len(evaluated))
	regionContent := map[contract.FrameRegion][]renderedFragment{
		contract.RegionStablePrefix: {},
		contract.RegionSemiStable:   {},
		contract.RegionDynamicTail:  {},
	}
	seenContent := make(map[string]struct{})
	for _, item := range evaluated {
		if err := checkContextV1(ctx); err != nil {
			return CompileResult{}, err
		}
		if item.decision != nil {
			decisions = append(decisions, *item.decision)
			continue
		}
		key := string(item.candidate.Kind) + ":" + string(item.candidate.Content.Digest)
		if _, ok := seenContent[key]; ok && !item.required {
			decisions = append(decisions, *decision(item.ref, contract.AdmissionExcluded, "duplicate_content"))
			continue
		}
		if item.required {
			remainingRequired[item.rule.Region] -= item.candidate.TokenEstimate
			remainingRequiredTotal -= item.candidate.TokenEstimate
		}
		regionLimit := regionBudget(request.Recipe.Budget, item.rule.Region)
		fits := usage[item.rule.Region]+item.candidate.TokenEstimate+remainingRequired[item.rule.Region] <= regionLimit && total+item.candidate.TokenEstimate+remainingRequiredTotal <= request.Recipe.Budget.TotalTokens && item.candidate.TokenEstimate <= item.rule.MaxTokens
		if !fits {
			if item.required || item.rule.Degradation == contract.DegradeReject {
				return CompileResult{}, fmt.Errorf("%w: candidate budget %s", contract.ErrConflict, item.candidate.ID)
			}
			decisions = append(decisions, *decision(item.ref, contract.AdmissionExcluded, "budget_exceeded"))
			continue
		}
		seenContent[key] = struct{}{}
		position := uint32(len(fragments) + 1)
		fragment := contract.ContextFragment{CandidateRef: item.ref, Kind: item.candidate.Kind, Region: item.rule.Region, Position: position, Content: item.candidate.Content, Tokens: item.candidate.TokenEstimate}
		fragments = append(fragments, fragment)
		decisions = append(decisions, contract.AdmissionDecision{CandidateRef: item.ref, Disposition: contract.AdmissionAdmitted, Reason: "policy_admitted", Region: item.rule.Region, Tokens: item.candidate.TokenEstimate})
		regionContent[item.rule.Region] = append(regionContent[item.rule.Region], renderedFragment{Position: position, Kind: item.candidate.Kind, CandidateDigest: item.ref.Digest, Content: item.content})
		usage[item.rule.Region] += item.candidate.TokenEstimate
		total += item.candidate.TokenEstimate
	}

	stableBytes, semiBytes, dynamicBytes, renderedBytes, err := renderRegionsContextV1(ctx, regionContent, limits.MaxGeneratedRawBytes)
	if err != nil {
		return CompileResult{}, err
	}
	generatedBytes := uint64(len(stableBytes)) + uint64(len(dynamicBytes)) + uint64(len(renderedBytes))
	generatedItems := uint32(3)
	if len(regionContent[contract.RegionSemiStable]) > 0 {
		generatedBytes += uint64(len(semiBytes))
		generatedItems++
	}
	if generatedItems > limits.MaxGeneratedContentItems || generatedBytes > limits.MaxGeneratedRawBytes || generatedBytes > limits.MaxOutputRawBytes {
		return CompileResult{}, fmt.Errorf("%w: generated output limits", contract.ErrLimitExceeded)
	}
	stableRef, err := store.PutContextV1(ctx, stableBytes)
	if err != nil {
		return CompileResult{}, err
	}
	var semiRef *contract.ContentRef
	if len(regionContent[contract.RegionSemiStable]) > 0 {
		ref, putErr := store.PutContextV1(ctx, semiBytes)
		if putErr != nil {
			return CompileResult{}, putErr
		}
		semiRef = &ref
	}
	dynamicRef, err := store.PutContextV1(ctx, dynamicBytes)
	if err != nil {
		return CompileResult{}, err
	}
	renderedRef, err := store.PutContextV1(ctx, renderedBytes)
	if err != nil {
		return CompileResult{}, err
	}
	sourceSetDigest, err := contract.DigestJSON(allRefs)
	if err != nil {
		return CompileResult{}, err
	}
	recipeDigest, err := request.Recipe.DigestValue()
	if err != nil {
		return CompileResult{}, err
	}
	manifest := contract.ContextManifest{
		ContractVersion: contract.Version, ID: request.ManifestID, Revision: 1, Execution: request.Execution,
		RecipeRef: contract.FactRef{ID: request.Recipe.ID, Revision: request.Recipe.Revision, Digest: recipeDigest}, GenerationID: request.GenerationID, ParentFrame: request.ParentFrame,
		Decisions: decisions, Fragments: fragments, StableTokens: usage[contract.RegionStablePrefix], SemiStableTokens: usage[contract.RegionSemiStable], DynamicTokens: usage[contract.RegionDynamicTail], TotalTokens: total,
		SourceSetDigest: sourceSetDigest, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano,
	}
	manifestDigest, err := manifest.DigestValue()
	if err != nil {
		return CompileResult{}, err
	}
	frame := contract.ContextFrame{
		ContractVersion: contract.Version, ID: request.FrameID, Revision: 1, Execution: request.Execution,
		ManifestRef: contract.FactRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifestDigest}, ParentFrame: request.ParentFrame,
		GenerationID: request.GenerationID, Generation: request.Generation, StablePrefix: stableRef, SemiStable: semiRef, DynamicTail: dynamicRef, Rendered: renderedRef,
		SourceSetDigest: sourceSetDigest, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano,
	}
	if err := frame.Validate(); err != nil {
		return CompileResult{}, err
	}
	if err := InspectFrameStagedV1(ctx, store, manifest, frame, InspectWorkLimitsV1{
		MaxFragments: limits.MaxCandidates, MaxContentItems: limits.MaxOutputContentItems,
		MaxContentItemBytes: limits.MaxOutputRawBytes, MaxRawBytes: limits.MaxOutputRawBytes,
		StreamChunkBytes: limits.StreamChunkBytes, CloneChunkBytes: limits.CloneChunkBytes,
	}); err != nil {
		return CompileResult{}, err
	}
	if err := checkContextV1(ctx); err != nil {
		return CompileResult{}, err
	}
	return CompileResult{Manifest: manifest, Frame: frame}, nil
}

func decision(ref contract.FactRef, disposition contract.AdmissionDisposition, reason string) *contract.AdmissionDecision {
	return &contract.AdmissionDecision{CandidateRef: ref, Disposition: disposition, Reason: reason}
}

func regionBudget(policy contract.BudgetPolicy, region contract.FrameRegion) uint64 {
	switch region {
	case contract.RegionStablePrefix:
		return policy.StablePrefixMax
	case contract.RegionSemiStable:
		return policy.SemiStableMax
	default:
		return policy.DynamicTailMax
	}
}

func renderRegions(regions map[contract.FrameRegion][]renderedFragment) ([]byte, []byte, []byte, []byte, error) {
	stable, err := json.Marshal(regions[contract.RegionStablePrefix])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: stable render", contract.ErrInvalid)
	}
	semi, err := json.Marshal(regions[contract.RegionSemiStable])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: semi-stable render", contract.ErrInvalid)
	}
	dynamic, err := json.Marshal(regions[contract.RegionDynamicTail])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: dynamic render", contract.ErrInvalid)
	}
	payload := struct {
		StablePrefix json.RawMessage `json:"stable_prefix"`
		SemiStable   json.RawMessage `json:"semi_stable"`
		DynamicTail  json.RawMessage `json:"dynamic_tail"`
	}{stable, semi, dynamic}
	rendered, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: frame render", contract.ErrInvalid)
	}
	return stable, semi, dynamic, rendered, nil
}

func IsUnknownOutcome(err error) bool {
	return errors.Is(err, contract.ErrUnknown)
}
