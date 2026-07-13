package effect

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func EvaluateIntent(intent union.IntentNode, effects []union.EffectRecord, verifications []union.VerificationRecord) union.IntentSatisfaction {
	superseded := make(map[union.EffectID]struct{})
	for _, effect := range effects {
		if !containsIntent(effect.IntentIDs, intent.ID) {
			continue
		}
		for _, effectID := range effect.SupersedesEffectIDs {
			superseded[effectID] = struct{}{}
		}
	}
	verificationByEffect := make(map[union.EffectID]union.VerificationStatus)
	for _, verification := range verifications {
		for _, effectID := range verification.EffectIDs {
			verificationByEffect[effectID] = strongestVerification(verificationByEffect[effectID], verification.Status)
		}
	}
	satisfaction := union.IntentSatisfaction{IntentID: intent.ID, Status: union.IntentUnsatisfied}
	var observed, verified, contradicted bool
	for _, effect := range effects {
		if _, stale := superseded[effect.ID]; stale {
			continue
		}
		if !containsIntent(effect.IntentIDs, intent.ID) {
			continue
		}
		if !effectMatchesIntent(intent, effect) {
			continue
		}
		observed = true
		satisfaction.EffectIDs = append(satisfaction.EffectIDs, effect.ID)
		status := strongestVerification(effect.VerificationStatus, verificationByEffect[effect.ID])
		switch status {
		case union.VerificationVerified:
			verified = true
		case union.VerificationContradicted:
			contradicted = true
		}
	}
	sort.Slice(satisfaction.EffectIDs, func(i, j int) bool { return satisfaction.EffectIDs[i] < satisfaction.EffectIDs[j] })
	switch {
	case contradicted:
		satisfaction.Status = union.IntentContradicted
	case verified:
		satisfaction.Status = union.IntentSatisfied
	case observed:
		satisfaction.Status = union.IntentPartiallySatisfied
	default:
		satisfaction.Status = union.IntentUnsatisfied
		satisfaction.MissingPostconditions = conditionKinds(intent.Postconditions)
	}
	return satisfaction
}

func effectMatchesIntent(intent union.IntentNode, observed union.EffectRecord) bool {
	wantKind := ""
	requireTarget := false
	switch intent.Kind {
	case union.IntentCreateFile:
		wantKind, requireTarget = "file_created", true
	case union.IntentModifyFile:
		wantKind, requireTarget = "file_changed", true
	case union.IntentRewriteFile:
		wantKind, requireTarget = "file_rewritten", true
	case union.IntentDeleteFile:
		wantKind, requireTarget = "file_deleted", true
	case union.IntentMoveFile:
		wantKind, requireTarget = "file_moved", true
	case union.IntentCreateDirectory:
		wantKind, requireTarget = "directory_created", true
	case union.IntentDeleteDirectory:
		wantKind, requireTarget = "directory_deleted", true
	case union.IntentProduceStructured:
		wantKind = "structured_output_produced"
	case union.IntentCallTool:
		wantKind, requireTarget = "tool_call_completed", true
	case union.IntentExecuteCode:
		wantKind = "code_execution_completed"
	case union.IntentComputerUse:
		wantKind = "computer_action_observed"
	default:
		return false
	}
	if observed.Kind != wantKind {
		return false
	}
	if requireTarget && observed.Target != intent.Target {
		return false
	}
	switch intent.Kind {
	case union.IntentCreateFile, union.IntentModifyFile, union.IntentRewriteFile, union.IntentDeleteFile,
		union.IntentCreateDirectory, union.IntentDeleteDirectory:
		return observed.Payload.WorkspaceChange != nil && observed.Payload.WorkspaceChange.Path == intent.Target
	case union.IntentMoveFile:
		destination, err := expectedMoveDestination(intent)
		return err == nil && observed.Payload.WorkspaceChange != nil &&
			observed.Payload.WorkspaceChange.Path == intent.Target && observed.Payload.WorkspaceChange.Destination == destination
	case union.IntentProduceStructured:
		return observed.Payload.StructuredOutput != nil
	case union.IntentCallTool:
		return observed.Payload.ToolCall != nil && observed.Payload.ToolCall.ToolID == intent.Target
	case union.IntentExecuteCode:
		return observed.Payload.CodeExecution != nil
	case union.IntentComputerUse:
		return observed.Payload.ComputerUse != nil
	default:
		return false
	}
}

func containsIntent(values []union.IntentID, target union.IntentID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func strongestVerification(left, right union.VerificationStatus) union.VerificationStatus {
	rank := func(status union.VerificationStatus) int {
		switch status {
		case union.VerificationContradicted:
			return 5
		case union.VerificationVerified:
			return 4
		case union.VerificationPartiallyVerified:
			return 3
		case union.VerificationUnverified:
			return 2
		case union.VerificationPending:
			return 1
		default:
			return 0
		}
	}
	if rank(right) > rank(left) {
		return right
	}
	return left
}

func conditionKinds(conditions []union.Condition) []string {
	result := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Kind != "" {
			result = append(result, condition.Kind)
		}
	}
	sort.Strings(result)
	return result
}
