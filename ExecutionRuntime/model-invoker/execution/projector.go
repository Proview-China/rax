package execution

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type ProjectionInput struct {
	Invocation      Invocation
	Events          []union.UnifiedExecutionEvent
	ContextManifest union.ContextManifestSummary
	Residuals       []union.Residual
}

type Projector struct{}

func (Projector) Project(input ProjectionInput) (union.UnifiedExecutionResult, error) {
	if err := input.Invocation.Validate(); err != nil {
		return union.UnifiedExecutionResult{}, err
	}
	if err := input.ContextManifest.Validate(); err != nil {
		return union.UnifiedExecutionResult{}, fmt.Errorf("%w: context manifest: %v", ErrProjectionInvariant, err)
	}
	ledger, err := ReplayForPlan(input.Invocation.Plan, input.Events)
	if err != nil {
		return union.UnifiedExecutionResult{}, fmt.Errorf("%w: replay: %v", ErrProjectionInvariant, err)
	}
	state := ledger.State()
	if !state.Terminal {
		return union.UnifiedExecutionResult{}, fmt.Errorf("%w: terminal event is missing", ErrProjectionInvariant)
	}

	result := union.UnifiedExecutionResult{
		SemanticVersion:       input.Invocation.Request.SemanticVersion,
		ExecutionID:           input.Invocation.Request.ExecutionID,
		TerminalEventID:       state.TerminalEventID,
		Status:                state.TerminalStatus,
		MappingReport:         input.Invocation.Plan.MappingReport,
		ContextManifest:       input.ContextManifest,
		PendingBackgroundWork: state.PendingBackgroundWork,
	}
	result.SessionID = input.Invocation.Request.SessionIntent.SessionID
	result.TurnID = input.Invocation.Request.SessionIntent.TurnID
	result.Residuals = append(result.Residuals, input.Invocation.Plan.Residuals...)
	result.Residuals = append(result.Residuals, input.Residuals...)

	attemptIndex := make(map[union.MechanismAttemptID]int)
	actionIndex := make(map[union.ItemID]int)
	effectIDs := make(map[union.EffectID]int)
	verificationIDs := make(map[union.VerificationID]struct{})
	for _, event := range input.Events {
		if event.Lifecycle != nil && event.Lifecycle.Status != "" {
			result.StopReason = event.Lifecycle.StopReason
			result.PendingBackgroundWork = event.Lifecycle.PendingBackgroundWork
		}
		if event.Mechanism != nil && event.Mechanism.Attempt != nil {
			attempt := *event.Mechanism.Attempt
			if index, exists := attemptIndex[attempt.ID]; exists {
				result.MechanismTrace[index] = attempt
			} else {
				attemptIndex[attempt.ID] = len(result.MechanismTrace)
				result.MechanismTrace = append(result.MechanismTrace, attempt)
			}
		}
		if event.Model != nil && len(event.Model.Content) != 0 && modelContentIsFinalOutput(*event.Model) {
			appendModelContent(&result, *event.Model)
		}
		if event.Model != nil && len(event.Model.Usage) != 0 {
			result.UsageMetrics = append(result.UsageMetrics, event.Model.Usage...)
		}
		if event.Item != nil {
			item := event.Item.Item
			if index, exists := actionIndex[item.ID]; exists {
				result.Actions[index] = item
			} else {
				actionIndex[item.ID] = len(result.Actions)
				result.Actions = append(result.Actions, item)
			}
		}
		if event.Effect != nil && event.Effect.Effect != nil {
			observed := *event.Effect.Effect
			if _, duplicate := effectIDs[observed.ID]; duplicate {
				return union.UnifiedExecutionResult{}, fmt.Errorf("%w: duplicate Effect identity", ErrProjectionInvariant)
			}
			effectIDs[observed.ID] = len(result.Effects)
			result.Effects = append(result.Effects, observed)
		}
		if event.Effect != nil && event.Effect.Verification != nil {
			verification := *event.Effect.Verification
			if _, duplicate := verificationIDs[verification.ID]; duplicate {
				return union.UnifiedExecutionResult{}, fmt.Errorf("%w: duplicate verification identity", ErrProjectionInvariant)
			}
			verificationIDs[verification.ID] = struct{}{}
			result.Verifications = append(result.Verifications, verification)
			for _, effectID := range verification.EffectIDs {
				index, exists := effectIDs[effectID]
				if !exists {
					return union.UnifiedExecutionResult{}, fmt.Errorf("%w: verification references an Effect not yet projected", ErrProjectionInvariant)
				}
				observed := &result.Effects[index]
				if !containsVerificationID(observed.VerificationRefs, verification.ID) {
					observed.VerificationRefs = append(observed.VerificationRefs, verification.ID)
				}
				observed.VerificationStatus = mergeVerificationStatus(observed.VerificationStatus, verification.Status)
			}
		}
		if event.Diagnostic != nil && event.Diagnostic.Residual != nil {
			result.Residuals = append(result.Residuals, *event.Diagnostic.Residual)
		}
	}
	result.WorkspaceChanges = effectiveWorkspaceChanges(result.Effects)

	for _, intent := range input.Invocation.Plan.IntentGraph.Nodes {
		result.IntentSatisfaction = append(result.IntentSatisfaction, effect.EvaluateIntent(intent, result.Effects, result.Verifications))
	}
	sort.Slice(result.IntentSatisfaction, func(left, right int) bool {
		return result.IntentSatisfaction[left].IntentID < result.IntentSatisfaction[right].IntentID
	})
	result.VerificationStatus = aggregateVerification(input.Invocation.Plan.IntentGraph, result.IntentSatisfaction)
	if result.Status == union.ExecutionStatusSucceeded && result.VerificationStatus != union.VerificationVerified {
		return union.UnifiedExecutionResult{}, fmt.Errorf("%w: succeeded result requires all required intents verified", ErrProjectionInvariant)
	}
	if result.Status == union.ExecutionStatusIndeterminate && result.VerificationStatus == union.VerificationVerified {
		result.VerificationStatus = union.VerificationPartiallyVerified
	}
	if state.RouteTerminalCandidate.Status == union.ExecutionStatusFailed || state.RouteTerminalCandidate.Status == union.ExecutionStatusIndeterminate {
		result.Error = &union.UnifiedError{
			Kind: "route_terminal", Phase: "adapter", Code: state.RouteTerminalCandidate.StopReason,
		}
	} else if result.Status == union.ExecutionStatusFailed || result.Status == union.ExecutionStatusIndeterminate {
		result.Error = &union.UnifiedError{Kind: "execution_terminal", Phase: "projection"}
	}
	digest, err := result.ComputeDigest()
	if err != nil {
		return union.UnifiedExecutionResult{}, fmt.Errorf("%w: result digest: %v", ErrProjectionInvariant, err)
	}
	result.Digest = digest
	return result, nil
}

func containsVerificationID(values []union.VerificationID, target union.VerificationID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mergeVerificationStatus(current, next union.VerificationStatus) union.VerificationStatus {
	if current == union.VerificationContradicted || next == union.VerificationContradicted {
		return union.VerificationContradicted
	}
	if current == "" || current == union.VerificationPending || current == union.VerificationUnverified || current == union.VerificationNotApplicable {
		return next
	}
	if next == union.VerificationPending || next == union.VerificationUnverified || next == union.VerificationNotApplicable {
		return current
	}
	if current == next {
		return current
	}
	return union.VerificationPartiallyVerified
}

func effectiveWorkspaceChanges(effects []union.EffectRecord) []union.WorkspaceChange {
	superseded := make(map[union.EffectID]struct{})
	for _, observed := range effects {
		for _, stale := range observed.SupersedesEffectIDs {
			superseded[stale] = struct{}{}
		}
	}
	changes := make([]union.WorkspaceChange, 0)
	for _, observed := range effects {
		if _, stale := superseded[observed.ID]; stale || observed.Payload.WorkspaceChange == nil {
			continue
		}
		changes = append(changes, *observed.Payload.WorkspaceChange)
	}
	return changes
}

func modelContentIsFinalOutput(event union.ModelEvent) bool {
	disclosure := strings.ToLower(event.DisclosureClass)
	kind := strings.ToLower(event.Kind)
	return !strings.Contains(disclosure, "reasoning") && !strings.Contains(disclosure, "thought") &&
		!strings.Contains(kind, "reasoning") && !strings.Contains(kind, "thought")
}

func appendModelContent(result *union.UnifiedExecutionResult, event union.ModelEvent) {
	isIncremental := strings.Contains(event.Kind, "delta") || strings.Contains(event.Kind, "chunk")
	for _, part := range event.Content {
		part.JSON = append([]byte(nil), part.JSON...)
		part.Metadata = cloneStringMap(part.Metadata)
		if isIncremental && part.Kind == "text" && len(result.FinalContent) != 0 && result.FinalContent[len(result.FinalContent)-1].Kind == "text" {
			result.FinalContent[len(result.FinalContent)-1].Text += part.Text
			continue
		}
		result.FinalContent = append(result.FinalContent, part)
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func aggregateVerification(graph union.IntentGraph, satisfaction []union.IntentSatisfaction) union.VerificationStatus {
	byIntent := make(map[union.IntentID]union.IntentSatisfactionStatus, len(satisfaction))
	for _, item := range satisfaction {
		byIntent[item.IntentID] = item.Status
	}
	allRequiredVerified := true
	anyObserved := false
	for _, intent := range graph.Nodes {
		status := byIntent[intent.ID]
		if status == union.IntentContradicted {
			return union.VerificationContradicted
		}
		if status == union.IntentSatisfied || status == union.IntentPartiallySatisfied {
			anyObserved = true
		}
		if intent.Required && status != union.IntentSatisfied {
			allRequiredVerified = false
		}
	}
	if allRequiredVerified {
		return union.VerificationVerified
	}
	if anyObserved {
		return union.VerificationPartiallyVerified
	}
	return union.VerificationUnverified
}
