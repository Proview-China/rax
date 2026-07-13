package union

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ValidationError struct {
	Field   string
	Problem string
}

func (err *ValidationError) Error() string {
	if err.Field == "" {
		return "union validation: " + err.Problem
	}
	return fmt.Sprintf("union validation: %s %s", err.Field, err.Problem)
}

func validationError(field, problem string) error {
	return &ValidationError{Field: field, Problem: problem}
}

func cloneJSON[T any](value T) (T, error) {
	var clone T
	data, err := json.Marshal(value)
	if err != nil {
		return clone, fmt.Errorf("clone union value: %w", err)
	}
	if err := json.Unmarshal(data, &clone); err != nil {
		return clone, fmt.Errorf("clone union value: %w", err)
	}
	return clone, nil
}

func StableDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("digest union value: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (value UnifiedExecutionRequest) Clone() (UnifiedExecutionRequest, error) {
	return cloneJSON(value)
}

func (value PreparedExecutionPlan) Clone() (PreparedExecutionPlan, error) {
	return cloneJSON(value)
}

func (value UnifiedExecutionEvent) Clone() (UnifiedExecutionEvent, error) {
	return cloneJSON(value)
}

func (value ExecutionCommand) Clone() (ExecutionCommand, error) {
	return cloneJSON(value)
}

func (value UnifiedExecutionResult) Clone() (UnifiedExecutionResult, error) {
	return cloneJSON(value)
}

func (value ContextManifestSummary) Clone() (ContextManifestSummary, error) {
	return cloneJSON(value)
}

func (value EffectRecord) Clone() (EffectRecord, error) {
	return cloneJSON(value)
}

func (value UnifiedExecutionRequest) Digest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	return StableDigest(value)
}

func (value PreparedExecutionPlan) ComputeDigest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	value.Digest = ""
	return StableDigest(value)
}

func (value UnifiedExecutionEvent) Digest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	return StableDigest(value)
}

func (value ExecutionCommand) Digest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	return StableDigest(value)
}

func (value UnifiedExecutionResult) ComputeDigest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	value.Digest = ""
	return StableDigest(value)
}

func (value ContextManifestSummary) ComputeDigest() (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	value.Digest = ""
	return StableDigest(value)
}

func validateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return validationError(field, "must not be empty")
	}
	return nil
}

func validateRaw(field string, value json.RawMessage) error {
	if len(value) != 0 && (!utf8.Valid(value) || !json.Valid(value)) {
		return validationError(field, "must contain valid JSON")
	}
	return nil
}

var forbiddenMapKeySuffixes = []string{
	"accesstoken", "apikey", "authorization", "clientsecret", "credentials", "credential",
	"oauthtoken", "password", "refreshtoken", "secret",
}

// These patterns intentionally cover only high-confidence credential literals.
// User content is not scanned here; this guard is limited to semantic control
// maps and extension envelopes, where plaintext credentials are never valid.
var credentialLiteralPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer[[:space:]]+[a-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`(?i)sk-(?:ant-)?[a-z0-9_-]{16,}`),
	regexp.MustCompile(`(?i)gh[pousr]_[a-z0-9]{20,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{20,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----`),
}

func validateStringMap(field string, values map[string]string) error {
	for key, value := range values {
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return validationError(field, "must contain valid UTF-8 strings")
		}
		normalized := normalizeMapKey(key)
		if normalized == "" {
			return validationError(field, "must not contain an empty key")
		}
		if forbiddenMapKey(normalized) {
			return validationError(field, "must not contain credential material")
		}
		if containsCredentialLiteral(value) {
			return validationError(field, "must not contain credential material")
		}
	}
	return nil
}

func validateUTF8Strings(field string, value any) error {
	if !allStringsUTF8(reflect.ValueOf(value)) {
		return validationError(field, "must contain valid UTF-8 strings")
	}
	return nil
}

func allStringsUTF8(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		return true
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		return value.IsNil() || allStringsUTF8(value.Elem())
	case reflect.String:
		return utf8.ValidString(value.String())
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if !allStringsUTF8(value.Field(index)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return true
		}
		for index := 0; index < value.Len(); index++ {
			if !allStringsUTF8(value.Index(index)) {
				return false
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if !allStringsUTF8(iterator.Key()) || !allStringsUTF8(iterator.Value()) {
				return false
			}
		}
	}
	return true
}

func containsCredentialLiteral(value string) bool {
	for _, pattern := range credentialLiteralPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func normalizeMapKey(key string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(key)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func forbiddenMapKey(normalized string) bool {
	for _, suffix := range forbiddenMapKeySuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func validateExtensions(field string, values map[string]json.RawMessage) error {
	for key, value := range values {
		if err := validateStringMap(field, map[string]string{key: ""}); err != nil {
			return err
		}
		if err := validateRaw(field, value); err != nil {
			return err
		}
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(string(value)))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return validationError(field, "must contain valid JSON")
		}
		if containsCredentialJSON(decoded) {
			return validationError(field, "must not contain credential material")
		}
	}
	return nil
}

func containsCredentialJSON(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if forbiddenMapKey(normalizeMapKey(key)) || containsCredentialJSON(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsCredentialJSON(nested) {
				return true
			}
		}
	case string:
		return containsCredentialLiteral(typed)
	}
	return false
}

func validateCapabilityOrigin(field string, value CapabilityOrigin) error {
	switch value {
	case CapabilityOriginNative, CapabilityOriginProviderHosted, CapabilityOriginHarnessHosted,
		CapabilityOriginCallerHosted, CapabilityOriginEmulated, CapabilityOriginUnavailable:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateSemanticFidelity(field string, value SemanticFidelity) error {
	switch value {
	case SemanticFidelityExact, SemanticFidelityTransformed, SemanticFidelityDegraded, SemanticFidelityUnavailable:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateStructuredOutputMechanism(field string, value StructuredOutputMechanism) error {
	switch value {
	case StructuredStrictJSONSchema, StructuredHarnessSchema, StructuredToolSchema,
		StructuredJSONObject, StructuredEmulatedSchema, StructuredPromptedJSON:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateExecutionOwner(field string, value ExecutionOwner) error {
	switch value {
	case ExecutionOwnerModel, ExecutionOwnerProvider, ExecutionOwnerHarness, ExecutionOwnerPraxis, ExecutionOwnerExternal:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateEventOrigin(field string, value EventOrigin) error {
	switch value {
	case EventOriginPraxis, EventOriginModel, EventOriginProvider, EventOriginHarness, EventOriginExternal:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateExecutionKind(field string, value ExecutionKind) error {
	switch value {
	case ExecutionKindAuto, ExecutionKindModel, ExecutionKindAgent:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateVerificationStatus(field string, value VerificationStatus) error {
	switch value {
	case VerificationPending, VerificationVerified, VerificationPartiallyVerified,
		VerificationUnverified, VerificationContradicted, VerificationNotApplicable:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateExecutionStatus(field string, value ExecutionStatus) error {
	switch value {
	case ExecutionStatusSucceeded, ExecutionStatusPartial, ExecutionStatusFailed,
		ExecutionStatusCancelled, ExecutionStatusIndeterminate:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateAttemptStatus(field string, value AttemptStatus) error {
	switch value {
	case AttemptStatusPlanned, AttemptStatusSelected, AttemptStatusAwaitingApproval, AttemptStatusRunning,
		AttemptStatusCompleted, AttemptStatusFailed, AttemptStatusDeclined, AttemptStatusCancelled, AttemptStatusIndeterminate:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateItemStatus(field string, value ItemStatus) error {
	switch value {
	case ItemStatusPending, ItemStatusInProgress, ItemStatusCompleted, ItemStatusIncomplete,
		ItemStatusFailed, ItemStatusCancelled, ItemStatusIndeterminate:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateSideEffectState(field string, value SideEffectState) error {
	switch value {
	case SideEffectNone, SideEffectPossible, SideEffectObserved, SideEffectReconciled, SideEffectUnknown:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateIntentKind(field string, value IntentKind) error {
	switch value {
	case IntentCreateFile, IntentModifyFile, IntentRewriteFile, IntentDeleteFile, IntentMoveFile,
		IntentCreateDirectory, IntentDeleteDirectory, IntentProduceStructured, IntentCallTool,
		IntentExecuteCode, IntentComputerUse:
		return nil
	default:
		return validationError(field, "is not recognized")
	}
}

func validateUnique[T comparable](field string, values []T) error {
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			return validationError(field, "must not contain duplicates")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsValue[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (selector ProfileSelector) Validate() error {
	hasExact := selector.Exact != nil
	hasConstraints := len(selector.Constraints) != 0
	if hasExact == hasConstraints {
		return validationError("profile_selector", "must select exactly one mode")
	}
	if hasExact {
		return selector.Exact.Validate("profile_selector.exact")
	}
	if err := validateStringMap("profile_selector.constraints", selector.Constraints); err != nil {
		return err
	}
	for _, value := range selector.Constraints {
		if strings.TrimSpace(value) == "" {
			return validationError("profile_selector.constraints", "must not contain an empty value")
		}
	}
	return nil
}

func validateContentPart(field string, part ContentPart) error {
	switch part.Kind {
	case "text":
		if part.Text == "" || len(part.JSON) != 0 || part.Reference != "" {
			return validationError(field, "text content must contain only non-empty text")
		}
	case "json":
		if len(part.JSON) == 0 || part.Text != "" || part.Reference != "" {
			return validationError(field, "json content must contain exactly one JSON value")
		}
	case "image_ref", "audio_ref", "video_ref", "file_ref":
		if part.Reference == "" || part.MediaType == "" || part.Text != "" || len(part.JSON) != 0 {
			return validationError(field, "media reference content requires a reference and media type")
		}
	case "artifact_ref":
		if part.Reference == "" || part.Text != "" || len(part.JSON) != 0 {
			return validationError(field, "artifact reference content requires exactly one reference")
		}
	default:
		return validationError(field+".kind", "is not recognized by the v1 content union")
	}
	if err := validateRaw(field+".json", part.JSON); err != nil {
		return err
	}
	return validateStringMap(field+".metadata", part.Metadata)
}

func validateIntentNode(field string, node IntentNode) error {
	if err := validateRequired(field+".id", string(node.ID)); err != nil {
		return err
	}
	if err := validateIntentKind(field+".kind", node.Kind); err != nil {
		return err
	}
	if err := validateRequired(field+".target", node.Target); err != nil {
		return err
	}
	if err := validateRaw(field+".specification", node.Specification); err != nil {
		return err
	}
	if err := validateUnique(field+".depends_on", node.DependsOn); err != nil {
		return err
	}
	for _, condition := range append(append([]Condition(nil), node.Preconditions...), node.Postconditions...) {
		if err := validateRequired(field+".condition.kind", condition.Kind); err != nil {
			return err
		}
		if err := validateRaw(field+".condition.payload", condition.Payload); err != nil {
			return err
		}
	}
	for _, fidelity := range node.AcceptedFidelity {
		if err := validateSemanticFidelity(field+".accepted_fidelity", fidelity); err != nil {
			return err
		}
	}
	if err := validateUnique(field+".accepted_fidelity", node.AcceptedFidelity); err != nil {
		return err
	}
	return validateStringMap(field+".metadata", node.Metadata)
}

func (graph IntentGraph) Validate() error {
	if len(graph.Nodes) == 0 {
		return validationError("intent_graph.nodes", "must not be empty")
	}
	nodes := make(map[IntentID]IntentNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		if err := validateIntentNode(fmt.Sprintf("intent_graph.nodes[%d]", index), node); err != nil {
			return err
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return validationError("intent_graph.nodes", "must not contain duplicate ids")
		}
		nodes[node.ID] = node
	}
	for _, node := range graph.Nodes {
		for _, dependency := range node.DependsOn {
			if dependency == node.ID {
				return validationError("intent_graph.depends_on", "must not contain a self dependency")
			}
			if _, exists := nodes[dependency]; !exists {
				return validationError("intent_graph.depends_on", "must reference an existing intent")
			}
		}
	}
	state := make(map[IntentID]uint8, len(nodes))
	var visit func(IntentID) error
	visit = func(id IntentID) error {
		switch state[id] {
		case 1:
			return validationError("intent_graph", "must be acyclic")
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range nodes[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range nodes {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func (request UnifiedExecutionRequest) Validate() error {
	if err := validateUTF8Strings("request", request); err != nil {
		return err
	}
	if request.SemanticVersion != SemanticVersionV1 {
		return validationError("semantic_version", "is not supported")
	}
	if err := validateRequired("execution_id", string(request.ExecutionID)); err != nil {
		return err
	}
	if err := request.ProfileSelector.Validate(); err != nil {
		return err
	}
	if err := validateExecutionKind("execution_kind", request.ExecutionKind); err != nil {
		return err
	}
	if err := request.IntentGraph.Validate(); err != nil {
		return err
	}
	if request.DegradationPolicy.Default != DegradationDefaultReject && request.DegradationPolicy.Default != DegradationDefaultAllowReported {
		return validationError("degradation_policy.default", "is not recognized")
	}
	for _, fidelity := range request.DegradationPolicy.AllowedFidelities {
		if err := validateSemanticFidelity("degradation_policy.allowed_fidelities", fidelity); err != nil {
			return err
		}
	}
	if err := validateUnique("degradation_policy.allowed_paths", request.DegradationPolicy.AllowedPaths); err != nil {
		return err
	}
	if err := validateUnique("degradation_policy.allowed_fidelities", request.DegradationPolicy.AllowedFidelities); err != nil {
		return err
	}
	if err := validateUnique("degradation_policy.forbidden_actions", request.DegradationPolicy.ForbiddenActions); err != nil {
		return err
	}
	if request.Budget.MaxInputTokens < 0 || request.Budget.MaxOutputTokens < 0 || request.Budget.MaxWallTime < 0 ||
		request.Budget.MaxSteps < 0 || request.Budget.MaxToolActions < 0 {
		return validationError("budget", "must not contain negative limits")
	}
	if request.ExecutionPolicy.MaxConcurrency < 0 || request.ToolPolicy.Parallelism < 0 || request.ToolPolicy.MaxActions < 0 {
		return validationError("policy", "must not contain negative limits")
	}
	if err := validateRaw("output_contract.json_schema", request.OutputContract.JSONSchema); err != nil {
		return err
	}
	if err := validateUnique("tool_policy.allowed_tool_ids", request.ToolPolicy.AllowedToolIDs); err != nil {
		return err
	}
	inputIDs := make([]ItemID, 0, len(request.Input))
	for index, input := range request.Input {
		field := fmt.Sprintf("input[%d]", index)
		if err := validateRequired(field+".id", string(input.ID)); err != nil {
			return err
		}
		switch input.Kind {
		case "message":
			if input.Role != "user" && input.Role != "assistant" && input.Role != "tool" {
				return validationError(field+".role", "is not recognized for a message")
			}
			if len(input.Content) == 0 {
				return validationError(field+".content", "must not be empty for a message")
			}
		case "tool_result":
			if input.ActionID == "" {
				return validationError(field+".action_id", "is required for a tool result")
			}
		case "artifact_reference":
			if len(input.Content) == 0 {
				return validationError(field+".content", "must contain an artifact reference")
			}
		case "native_extension":
			if err := validateNativeInputExtension(field+".payload", input.Payload); err != nil {
				return err
			}
		default:
			return validationError(field+".kind", "is not recognized by the v1 input union")
		}
		if err := validateRaw(field+".payload", input.Payload); err != nil {
			return err
		}
		for partIndex, part := range input.Content {
			if err := validateContentPart(fmt.Sprintf("%s.content[%d]", field, partIndex), part); err != nil {
				return err
			}
		}
		inputIDs = append(inputIDs, input.ID)
	}
	if err := validateUnique("input.id", inputIDs); err != nil {
		return err
	}
	instructionIDs := make([]string, 0, len(request.Instructions))
	for index, instruction := range request.Instructions {
		field := fmt.Sprintf("instructions[%d]", index)
		if err := validateRequired(field+".id", instruction.ID); err != nil {
			return err
		}
		switch instruction.Authority {
		case "runtime_policy", "developer", "task":
		default:
			return validationError(field+".authority", "is not recognized by the v1 instruction union")
		}
		switch instruction.Scope {
		case "execution", "session", "turn", "action":
		default:
			return validationError(field+".scope", "is not recognized by the v1 instruction union")
		}
		switch instruction.ConflictPolicy {
		case "reject", "higher_authority_wins", "append":
		default:
			return validationError(field+".conflict_policy", "is not recognized by the v1 instruction union")
		}
		if len(instruction.Content) == 0 {
			return validationError(field+".content", "must not be empty")
		}
		for partIndex, part := range instruction.Content {
			if err := validateContentPart(fmt.Sprintf("%s.content[%d]", field, partIndex), part); err != nil {
				return err
			}
		}
		instructionIDs = append(instructionIDs, instruction.ID)
	}
	if err := validateUnique("instructions.id", instructionIDs); err != nil {
		return err
	}
	contextIDs := make([]string, 0, len(request.Context))
	for index, reference := range request.Context {
		field := fmt.Sprintf("context[%d]", index)
		for _, required := range []struct {
			name, value string
		}{
			{"id", reference.ID}, {"kind", reference.Kind}, {"reference", reference.Reference},
			{"access", reference.Access}, {"visibility", reference.Visibility},
		} {
			if err := validateRequired(field+"."+required.name, required.value); err != nil {
				return err
			}
		}
		switch reference.Kind {
		case "conversation", "workspace", "file", "artifact", "memory", "resource":
		default:
			return validationError(field+".kind", "is not recognized by the v1 context union")
		}
		switch reference.Access {
		case "read", "write", "execute":
		default:
			return validationError(field+".access", "is not recognized by the v1 context union")
		}
		switch reference.Visibility {
		case "model", "harness", "praxis_only":
		default:
			return validationError(field+".visibility", "is not recognized by the v1 context union")
		}
		if containsCredentialLiteral(reference.Reference) || containsCredentialLiteral(reference.Snapshot) {
			return validationError(field, "must not contain credential material")
		}
		contextIDs = append(contextIDs, reference.ID)
	}
	if err := validateUnique("context.id", contextIDs); err != nil {
		return err
	}
	toolIDs := make([]string, 0, len(request.Tools))
	toolNames := make([]string, 0, len(request.Tools))
	for index, tool := range request.Tools {
		field := fmt.Sprintf("tools[%d]", index)
		if err := validateRequired(field+".id", tool.ID); err != nil {
			return err
		}
		if err := validateRequired(field+".name", tool.Name); err != nil {
			return err
		}
		if err := validateRequired(field+".kind", tool.Kind); err != nil {
			return err
		}
		if err := validateExecutionOwner(field+".execution_owner", tool.ExecutionOwner); err != nil {
			return err
		}
		if tool.Timeout < 0 {
			return validationError(field+".timeout", "must not be negative")
		}
		for rawField, raw := range map[string]json.RawMessage{
			"input_schema": tool.InputSchema, "output_schema": tool.OutputSchema, "extension": tool.Extension,
		} {
			if err := validateRaw(field+"."+rawField, raw); err != nil {
				return err
			}
		}
		toolIDs = append(toolIDs, tool.ID)
		toolNames = append(toolNames, tool.Name)
	}
	if err := validateUnique("tools.id", toolIDs); err != nil {
		return err
	}
	if err := validateUnique("tools.name", toolNames); err != nil {
		return err
	}
	toolIDSet := make(map[string]struct{}, len(toolIDs))
	for _, toolID := range toolIDs {
		toolIDSet[toolID] = struct{}{}
	}
	for _, allowedToolID := range request.ToolPolicy.AllowedToolIDs {
		if _, exists := toolIDSet[allowedToolID]; !exists {
			return validationError("tool_policy.allowed_tool_ids", "must reference a declared tool")
		}
	}
	if err := validateStringMap("metadata", request.Metadata); err != nil {
		return err
	}
	return validateExtensions("extensions", request.Extensions)
}

func validateNativeInputExtension(field string, raw json.RawMessage) error {
	if err := validateRaw(field, raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return validationError(field, "is required for a native extension")
	}
	var envelope map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil || envelope == nil {
		return validationError(field, "must be an object envelope")
	}
	namespace, _ := envelope["namespace"].(string)
	if strings.TrimSpace(namespace) == "" || !strings.ContainsAny(namespace, "./") {
		return validationError(field+".namespace", "must be a namespaced extension identifier")
	}
	if containsCredentialJSON(envelope) {
		return validationError(field, "must not contain credential material")
	}
	return nil
}

func validateMechanismPlan(field string, plan MechanismPlan) error {
	if err := validateRequired(field+".id", string(plan.ID)); err != nil {
		return err
	}
	if err := validateRequired(field+".intent_id", string(plan.IntentID)); err != nil {
		return err
	}
	if err := validateRequired(field+".kind", plan.Kind); err != nil {
		return err
	}
	if err := validateCapabilityOrigin(field+".origin", plan.Origin); err != nil {
		return err
	}
	if err := validateExecutionOwner(field+".owner", plan.Owner); err != nil {
		return err
	}
	switch plan.SelectionAuthority {
	case SelectionAuthorityRuntime, SelectionAuthorityModelWithinSet, SelectionAuthorityHarness, SelectionAuthorityProvider:
	default:
		return validationError(field+".selection_authority", "is not recognized")
	}
	if err := validateSemanticFidelity(field+".semantic_fidelity", plan.SemanticFidelity); err != nil {
		return err
	}
	if plan.PreferredRank < 0 {
		return validationError(field+".preferred_rank", "must not be negative")
	}
	if err := validateUnique(field+".fallback_plan_ids", plan.FallbackPlanIDs); err != nil {
		return err
	}
	for _, fallback := range plan.FallbackPlanIDs {
		if fallback == plan.ID {
			return validationError(field+".fallback_plan_ids", "must not reference the same mechanism")
		}
	}
	return nil
}

func (manifest ContextManifestSummary) Validate() error {
	if err := validateRequired("context_manifest.id", manifest.ID); err != nil {
		return err
	}
	if err := validateRequired("context_manifest.version", manifest.Version); err != nil {
		return err
	}
	if err := validateRequired("context_manifest.mode", manifest.Mode); err != nil {
		return err
	}
	componentKeys := make([]string, 0, len(manifest.Components))
	for index, component := range manifest.Components {
		field := fmt.Sprintf("context_manifest.components[%d]", index)
		if err := validateRequired(field+".kind", component.Kind); err != nil {
			return err
		}
		if err := validateRequired(field+".name", component.Name); err != nil {
			return err
		}
		if err := validateRequired(field+".state", component.State); err != nil {
			return err
		}
		if component.Owner != "" {
			if err := validateExecutionOwner(field+".owner", component.Owner); err != nil {
				return err
			}
		}
		componentKeys = append(componentKeys, component.Kind+"\x00"+component.Name)
	}
	if err := validateUnique("context_manifest.components", componentKeys); err != nil {
		return err
	}
	toolIDs := make([]string, 0, len(manifest.Tools.Entries))
	for index, tool := range manifest.Tools.Entries {
		field := fmt.Sprintf("context_manifest.tools.entries[%d]", index)
		if err := validateRequired(field+".id", tool.ID); err != nil {
			return err
		}
		if err := validateRequired(field+".permission_mode", tool.PermissionMode); err != nil {
			return err
		}
		if err := validateExecutionOwner(field+".owner", tool.Owner); err != nil {
			return err
		}
		if tool.FallbackOwner != "" {
			if err := validateExecutionOwner(field+".fallback_owner", tool.FallbackOwner); err != nil {
				return err
			}
		}
		if tool.Registered && !tool.Discovered {
			return validationError(field, "registered requires discovered")
		}
		if tool.ModelVisible && !tool.Registered {
			return validationError(field, "model visibility requires registration")
		}
		if tool.Executable && !tool.Registered {
			return validationError(field, "execution requires registration")
		}
		if tool.AutoApproved && !tool.Executable {
			return validationError(field, "automatic approval requires an executable tool")
		}
		switch tool.Probe.Status {
		case ToolProbeNotRun:
			if tool.Probe.EvidenceDigest != "" || !tool.Probe.ObservedAt.IsZero() {
				return validationError(field+".probe", "not_run must not include evidence")
			}
		case ToolProbeReported, ToolProbeObserved:
			if err := validateRequired(field+".probe.evidence_digest", tool.Probe.EvidenceDigest); err != nil {
				return err
			}
		default:
			return validationError(field+".probe.status", "is not recognized")
		}
		toolIDs = append(toolIDs, tool.ID)
	}
	if err := validateUnique("context_manifest.tools.entries.id", toolIDs); err != nil {
		return err
	}
	for _, path := range manifest.OpaqueFields {
		if err := validateRequired("context_manifest.opaque_fields", path); err != nil {
			return err
		}
	}
	return validateUnique("context_manifest.opaque_fields", manifest.OpaqueFields)
}

func validateResidual(field string, residual Residual) error {
	if err := validateRequired(field+".path", residual.Path); err != nil {
		return err
	}
	if err := validateRequired(field+".kind", residual.Kind); err != nil {
		return err
	}
	if err := validateRequired(field+".severity", residual.Severity); err != nil {
		return err
	}
	return validateRequired(field+".impact", residual.Impact)
}

func validateMappingReport(report MappingReport) error {
	paths := make([]string, 0, len(report.Decisions))
	for index, decision := range report.Decisions {
		field := fmt.Sprintf("mapping_report.decisions[%d]", index)
		if err := validateRequired(field+".path", decision.Path); err != nil {
			return err
		}
		if err := validateSemanticFidelity(field+".fidelity", decision.Fidelity); err != nil {
			return err
		}
		if err := validateCapabilityOrigin(field+".origin", decision.Origin); err != nil {
			return err
		}
		paths = append(paths, decision.Path)
	}
	return validateUnique("mapping_report.decisions.path", paths)
}

func (plan PreparedExecutionPlan) Validate() error {
	if err := validateUTF8Strings("plan", plan); err != nil {
		return err
	}
	if plan.SemanticVersion != SemanticVersionV1 {
		return validationError("semantic_version", "is not supported")
	}
	if err := validateRequired("execution_id", string(plan.ExecutionID)); err != nil {
		return err
	}
	if err := plan.Profile.Validate("profile"); err != nil {
		return err
	}
	if err := plan.Route.Validate("route"); err != nil {
		return err
	}
	if err := validateRequired("profile_key_digest", plan.ProfileKeyDigest); err != nil {
		return err
	}
	if plan.ExecutionKind != ExecutionKindModel && plan.ExecutionKind != ExecutionKindAgent {
		return validationError("execution_kind", "must be resolved before planning")
	}
	if err := plan.IntentGraph.Validate(); err != nil {
		return err
	}
	intentIDs := make(map[IntentID]struct{}, len(plan.IntentGraph.Nodes))
	for _, intent := range plan.IntentGraph.Nodes {
		intentIDs[intent.ID] = struct{}{}
	}
	mechanismIDs := make(map[MechanismPlanID]struct{}, len(plan.Mechanisms))
	mechanismsByID := make(map[MechanismPlanID]MechanismPlan, len(plan.Mechanisms))
	mechanismsByIntent := make(map[IntentID]int, len(plan.IntentGraph.Nodes))
	for index, mechanism := range plan.Mechanisms {
		if err := validateMechanismPlan(fmt.Sprintf("mechanisms[%d]", index), mechanism); err != nil {
			return err
		}
		if _, exists := intentIDs[mechanism.IntentID]; !exists {
			return validationError("mechanisms.intent_id", "must reference an existing intent")
		}
		if _, duplicate := mechanismIDs[mechanism.ID]; duplicate {
			return validationError("mechanisms.id", "must not contain duplicates")
		}
		mechanismIDs[mechanism.ID] = struct{}{}
		mechanismsByID[mechanism.ID] = mechanism
		mechanismsByIntent[mechanism.IntentID]++
	}
	for intentID := range intentIDs {
		if mechanismsByIntent[intentID] == 0 {
			return validationError("mechanisms.intent_id", "must plan at least one mechanism for every intent")
		}
	}
	for _, mechanism := range plan.Mechanisms {
		for _, fallback := range mechanism.FallbackPlanIDs {
			fallbackPlan, exists := mechanismsByID[fallback]
			if !exists {
				return validationError("mechanisms.fallback_plan_ids", "must reference an existing mechanism")
			}
			if fallbackPlan.IntentID != mechanism.IntentID {
				return validationError("mechanisms.fallback_plan_ids", "must remain within the same intent")
			}
		}
	}
	fallbackState := make(map[MechanismPlanID]uint8, len(mechanismsByID))
	var visitFallback func(MechanismPlanID) error
	visitFallback = func(id MechanismPlanID) error {
		switch fallbackState[id] {
		case 1:
			return validationError("mechanisms.fallback_plan_ids", "must be acyclic")
		case 2:
			return nil
		}
		fallbackState[id] = 1
		for _, fallback := range mechanismsByID[id].FallbackPlanIDs {
			if err := visitFallback(fallback); err != nil {
				return err
			}
		}
		fallbackState[id] = 2
		return nil
	}
	for id := range mechanismsByID {
		if err := visitFallback(id); err != nil {
			return err
		}
	}
	if err := plan.ExpectedManifest.Validate(); err != nil {
		return err
	}
	for index, residual := range plan.Residuals {
		if err := validateResidual(fmt.Sprintf("residuals[%d]", index), residual); err != nil {
			return err
		}
	}
	if err := validateMappingReport(plan.MappingReport); err != nil {
		return err
	}
	if err := validateRequired("route_fingerprint", plan.RouteFingerprint); err != nil {
		return err
	}
	return validateStringMap("metadata", plan.Metadata)
}

func validateMechanismAttempt(field string, attempt MechanismAttempt) error {
	if err := validateRequired(field+".id", string(attempt.ID)); err != nil {
		return err
	}
	if err := validateRequired(field+".mechanism_plan_id", string(attempt.MechanismPlanID)); err != nil {
		return err
	}
	if err := validateRequired(field+".actual_kind", attempt.ActualKind); err != nil {
		return err
	}
	if err := validateCapabilityOrigin(field+".actual_origin", attempt.ActualOrigin); err != nil {
		return err
	}
	if err := validateExecutionOwner(field+".actual_owner", attempt.ActualOwner); err != nil {
		return err
	}
	if err := validateAttemptStatus(field+".status", attempt.Status); err != nil {
		return err
	}
	if err := validateSideEffectState(field+".side_effect_state", attempt.SideEffectState); err != nil {
		return err
	}
	if attempt.NativeToolIdentity != nil {
		if err := attempt.NativeToolIdentity.Validate(); err != nil {
			return err
		}
	}
	if err := validateRaw(field+".sanitized_input", attempt.SanitizedInput); err != nil {
		return err
	}
	if !attempt.StartedAt.IsZero() && !attempt.EndedAt.IsZero() && attempt.EndedAt.Before(attempt.StartedAt) {
		return validationError(field+".ended_at", "must not precede started_at")
	}
	return nil
}

func validateEvidence(field string, evidence EvidenceRef) error {
	if err := validateRequired(field+".kind", evidence.Kind); err != nil {
		return err
	}
	if err := validateRequired(field+".source", evidence.Source); err != nil {
		return err
	}
	if err := validateRequired(field+".digest", evidence.Digest); err != nil {
		return err
	}
	if evidence.CapturedAt.IsZero() {
		return validationError(field+".captured_at", "must not be zero")
	}
	return validateRequired(field+".sensitivity", evidence.Sensitivity)
}

func validateFileState(field string, state FileStateSnapshot) error {
	if err := validateRequired(field+".path", state.Path); err != nil {
		return err
	}
	if state.Size < 0 {
		return validationError(field+".size", "must not be negative")
	}
	if !state.Exists {
		if state.Type != FileStateAbsent {
			return validationError(field+".type", "must be absent when the path does not exist")
		}
		if state.Hash != "" || state.Size != 0 || state.Mode != 0 || !state.ModifiedAt.IsZero() || state.Symlink != "" {
			return validationError(field, "must not contain file metadata when the path does not exist")
		}
		return nil
	}
	switch state.Type {
	case FileStateRegular, FileStateDirectory, FileStateSymlink, FileStateOther:
	default:
		return validationError(field+".type", "is not recognized")
	}
	if state.Type == FileStateSymlink && strings.TrimSpace(state.Symlink) == "" {
		return validationError(field+".symlink", "must identify the link target")
	}
	return nil
}

func validateWorkspaceChange(field string, change WorkspaceChange) error {
	if err := validateRequired(field+".kind", change.Kind); err != nil {
		return err
	}
	if err := validateRequired(field+".path", change.Path); err != nil {
		return err
	}
	if change.Destination != "" && change.Destination == change.Path {
		return validationError(field+".destination", "must differ from the source path")
	}
	if change.Before != nil {
		if err := validateFileState(field+".before", *change.Before); err != nil {
			return err
		}
	}
	if change.After != nil {
		if err := validateFileState(field+".after", *change.After); err != nil {
			return err
		}
	}
	if change.DestinationBefore != nil {
		if err := validateFileState(field+".destination_before", *change.DestinationBefore); err != nil {
			return err
		}
	}
	if change.DestinationAfter != nil {
		if err := validateFileState(field+".destination_after", *change.DestinationAfter); err != nil {
			return err
		}
	}
	if change.Destination == "" && (change.DestinationBefore != nil || change.DestinationAfter != nil) {
		return validationError(field+".destination", "is required when destination snapshots are present")
	}
	if change.Destination != "" {
		if change.DestinationBefore == nil || change.DestinationAfter == nil {
			return validationError(field, "must contain both destination snapshots")
		}
		if change.DestinationBefore.Path != change.Destination || change.DestinationAfter.Path != change.Destination {
			return validationError(field+".destination", "must match both destination snapshot paths")
		}
	}
	if change.Before != nil && change.Before.Path != change.Path {
		return validationError(field+".before.path", "must match the workspace change path")
	}
	if change.After != nil && change.After.Path != change.Path {
		return validationError(field+".after.path", "must match the workspace change path")
	}
	return nil
}

func (payload EffectPayload) Validate() error {
	arms := 0
	if payload.WorkspaceChange != nil {
		arms++
		if err := validateWorkspaceChange("effect.payload.workspace_change", *payload.WorkspaceChange); err != nil {
			return err
		}
	}
	if payload.StructuredOutput != nil {
		arms++
		if err := validateStructuredOutputMechanism("effect.payload.structured_output.mechanism", payload.StructuredOutput.Mechanism); err != nil {
			return err
		}
		if err := validateCapabilityOrigin("effect.payload.structured_output.origin", payload.StructuredOutput.Origin); err != nil {
			return err
		}
		if err := validateSemanticFidelity("effect.payload.structured_output.fidelity", payload.StructuredOutput.Fidelity); err != nil {
			return err
		}
		if err := validateRequired("effect.payload.structured_output.schema_digest", payload.StructuredOutput.SchemaDigest); err != nil {
			return err
		}
		if err := validateRaw("effect.payload.structured_output.parsed", payload.StructuredOutput.Parsed); err != nil {
			return err
		}
		if payload.StructuredOutput.JSONValid && len(payload.StructuredOutput.Parsed) == 0 {
			return validationError("effect.payload.structured_output.parsed", "is required when JSON is valid")
		}
		if payload.StructuredOutput.SchemaValid && !payload.StructuredOutput.JSONValid {
			return validationError("effect.payload.structured_output.schema_valid", "cannot be true when JSON is invalid")
		}
		if payload.StructuredOutput.SchemaValid && strings.TrimSpace(payload.StructuredOutput.FinalDigest) == "" {
			return validationError("effect.payload.structured_output.final_digest", "is required when schema validation succeeds")
		}
		if payload.StructuredOutput.RepairAttempts < 0 {
			return validationError("effect.payload.structured_output.repair_attempts", "must not be negative")
		}
	}
	if payload.CodeExecution != nil {
		arms++
		if err := validateRequired("effect.payload.code_execution.mechanism", payload.CodeExecution.Mechanism); err != nil {
			return err
		}
		if err := validateCapabilityOrigin("effect.payload.code_execution.origin", payload.CodeExecution.Origin); err != nil {
			return err
		}
		if len(payload.CodeExecution.Argv) == 0 {
			return validationError("effect.payload.code_execution.argv", "must not be empty")
		}
		if err := validateRequired("effect.payload.code_execution.runtime_identity", payload.CodeExecution.RuntimeIdentity); err != nil {
			return err
		}
		if payload.CodeExecution.Duration < 0 {
			return validationError("effect.payload.code_execution.duration", "must not be negative")
		}
		for index, evidence := range payload.CodeExecution.NetworkEvidence {
			if err := validateEvidence(fmt.Sprintf("effect.payload.code_execution.network_evidence[%d]", index), evidence); err != nil {
				return err
			}
		}
	}
	if payload.ToolCall != nil {
		arms++
		if err := validateRequired("effect.payload.tool_call.tool_id", payload.ToolCall.ToolID); err != nil {
			return err
		}
		if err := validateRequired("effect.payload.tool_call.action_id", string(payload.ToolCall.ActionID)); err != nil {
			return err
		}
		if err := validateRequired("effect.payload.tool_call.mechanism", payload.ToolCall.Mechanism); err != nil {
			return err
		}
		if err := validateCapabilityOrigin("effect.payload.tool_call.origin", payload.ToolCall.Origin); err != nil {
			return err
		}
		if err := validateExecutionOwner("effect.payload.tool_call.owner", payload.ToolCall.Owner); err != nil {
			return err
		}
		if !payload.ToolCall.Executed {
			return validationError("effect.payload.tool_call.executed", "must be true for an observed Effect")
		}
		if err := validateRequired("effect.payload.tool_call.input_digest", payload.ToolCall.InputDigest); err != nil {
			return err
		}
		if err := validateEventOrigin("effect.payload.tool_call.result_origin", payload.ToolCall.ResultOrigin); err != nil {
			return err
		}
		if payload.ToolCall.ResultOrigin == EventOriginModel {
			return validationError("effect.payload.tool_call.result_origin", "must identify an execution observer, not model prose")
		}
		if err := validateSideEffectState("effect.payload.tool_call.side_effect_state", payload.ToolCall.SideEffectState); err != nil {
			return err
		}
	}
	if payload.ComputerUse != nil {
		arms++
		if err := validateRequired("effect.payload.computer_use.mechanism", payload.ComputerUse.Mechanism); err != nil {
			return err
		}
		if err := validateCapabilityOrigin("effect.payload.computer_use.origin", payload.ComputerUse.Origin); err != nil {
			return err
		}
		if err := validateRequired("effect.payload.computer_use.action", payload.ComputerUse.Action); err != nil {
			return err
		}
		for index, evidence := range append(append([]EvidenceRef(nil), payload.ComputerUse.BeforeRefs...), payload.ComputerUse.AfterRefs...) {
			if err := validateEvidence(fmt.Sprintf("effect.payload.computer_use.evidence[%d]", index), evidence); err != nil {
				return err
			}
		}
	}
	if len(payload.Extension) != 0 {
		arms++
		if err := validateRaw("effect.payload.extension", payload.Extension); err != nil {
			return err
		}
	}
	if arms != 1 {
		return validationError("effect.payload", "must contain exactly one tagged payload")
	}
	return nil
}

func (effect EffectRecord) Validate() error {
	if err := validateUTF8Strings("effect", effect); err != nil {
		return err
	}
	if err := validateRequired("effect.id", string(effect.ID)); err != nil {
		return err
	}
	if len(effect.IntentIDs) == 0 {
		return validationError("effect.intent_ids", "must not be empty")
	}
	if err := validateUnique("effect.intent_ids", effect.IntentIDs); err != nil {
		return err
	}
	if err := validateRequired("effect.mechanism_attempt_id", string(effect.MechanismAttemptID)); err != nil {
		return err
	}
	if err := validateRequired("effect.kind", effect.Kind); err != nil {
		return err
	}
	if err := validateRequired("effect.target", effect.Target); err != nil {
		return err
	}
	if err := effect.Payload.Validate(); err != nil {
		return err
	}
	switch {
	case effect.Payload.WorkspaceChange != nil && effect.Target != effect.Payload.WorkspaceChange.Path:
		return validationError("effect.target", "must match the workspace change path")
	case effect.Payload.ToolCall != nil && effect.Target != effect.Payload.ToolCall.ToolID:
		return validationError("effect.target", "must match the executed tool identity")
	case effect.Payload.ComputerUse != nil && effect.Target != effect.Payload.ComputerUse.Target:
		return validationError("effect.target", "must match the computer target")
	}
	if err := validateRequired("effect.observation_source", effect.ObservationSource); err != nil {
		return err
	}
	if err := validateVerificationStatus("effect.verification_status", effect.VerificationStatus); err != nil {
		return err
	}
	if err := validateUnique("effect.verification_refs", effect.VerificationRefs); err != nil {
		return err
	}
	if (effect.VerificationStatus == VerificationVerified || effect.VerificationStatus == VerificationPartiallyVerified ||
		effect.VerificationStatus == VerificationContradicted) && len(effect.VerificationRefs) == 0 {
		return validationError("effect.verification_refs", "are required for a conclusive verification status")
	}
	if err := validateUnique("effect.supersedes_effect_ids", effect.SupersedesEffectIDs); err != nil {
		return err
	}
	for _, superseded := range effect.SupersedesEffectIDs {
		if superseded == effect.ID {
			return validationError("effect.supersedes_effect_ids", "must not reference the same Effect")
		}
	}
	for index, evidence := range effect.EvidenceRefs {
		if err := validateEvidence(fmt.Sprintf("effect.evidence_refs[%d]", index), evidence); err != nil {
			return err
		}
	}
	if effect.OccurredAt.IsZero() {
		return validationError("effect.occurred_at", "must not be zero")
	}
	return nil
}

func (verification VerificationRecord) Validate() error {
	if err := validateUTF8Strings("verification", verification); err != nil {
		return err
	}
	if err := validateRequired("verification.id", string(verification.ID)); err != nil {
		return err
	}
	if err := validateRequired("verification.kind", verification.Kind); err != nil {
		return err
	}
	if err := validateVerificationStatus("verification.status", verification.Status); err != nil {
		return err
	}
	if err := verification.Verifier.Validate("verification.verifier"); err != nil {
		return err
	}
	if err := validateUnique("verification.effect_ids", verification.EffectIDs); err != nil {
		return err
	}
	if err := validateUnique("verification.intent_ids", verification.IntentIDs); err != nil {
		return err
	}
	if len(verification.EffectIDs) == 0 && len(verification.IntentIDs) == 0 {
		return validationError("verification", "must reference at least one Effect or intent")
	}
	if verification.CompletedAt.IsZero() {
		return validationError("verification.completed_at", "must not be zero")
	}
	for index, evidence := range verification.EvidenceRefs {
		if err := validateEvidence(fmt.Sprintf("verification.evidence_refs[%d]", index), evidence); err != nil {
			return err
		}
	}
	return nil
}

func validateIntentSatisfaction(field string, satisfaction IntentSatisfaction) error {
	if err := validateRequired(field+".intent_id", string(satisfaction.IntentID)); err != nil {
		return err
	}
	switch satisfaction.Status {
	case IntentSatisfied, IntentPartiallySatisfied, IntentUnsatisfied, IntentContradicted:
	default:
		return validationError(field+".status", "is not recognized")
	}
	if err := validateUnique(field+".effect_ids", satisfaction.EffectIDs); err != nil {
		return err
	}
	if satisfaction.Status == IntentUnsatisfied && len(satisfaction.EffectIDs) != 0 {
		return validationError(field+".effect_ids", "must be empty when the intent is unsatisfied")
	}
	if satisfaction.Status != IntentUnsatisfied && len(satisfaction.EffectIDs) == 0 {
		return validationError(field+".effect_ids", "must not be empty for an observed satisfaction state")
	}
	return nil
}

func (header EventHeader) Validate() error {
	if err := validateRequired("event.header.event_id", string(header.EventID)); err != nil {
		return err
	}
	if header.SemanticVersion != SemanticVersionV1 {
		return validationError("event.header.semantic_version", "is not supported")
	}
	if err := validateRequired("event.header.execution_id", string(header.ExecutionID)); err != nil {
		return err
	}
	if header.Sequence == 0 {
		return validationError("event.header.sequence", "must be greater than zero")
	}
	if header.Timestamp.IsZero() {
		return validationError("event.header.timestamp", "must not be zero")
	}
	switch header.Origin {
	case EventOriginPraxis, EventOriginModel, EventOriginProvider, EventOriginHarness, EventOriginExternal:
	default:
		return validationError("event.header.origin", "is not recognized")
	}
	switch header.Family {
	case EventFamilyLifecycle, EventFamilyIntent, EventFamilyMechanism, EventFamilyModel,
		EventFamilyItem, EventFamilyEffect, EventFamilyControl, EventFamilyDiagnostic:
	default:
		return validationError("event.header.family", "is not recognized")
	}
	switch header.Visibility {
	case VisibilityModelVisible, VisibilityUserVisible, VisibilityProgressOnly, VisibilityAuditOnly, VisibilityPrivateRuntime:
	default:
		return validationError("event.header.visibility", "is not recognized")
	}
	switch header.SecurityClassification {
	case SecurityPublic, SecurityInternal, SecuritySensitive, SecurityRestricted:
	default:
		return validationError("event.header.security_classification", "is not recognized")
	}
	if header.ExecutionKind != ExecutionKindModel && header.ExecutionKind != ExecutionKindAgent {
		return validationError("event.header.execution_kind", "must be resolved")
	}
	if err := validateExecutionKind("event.header.execution_kind", header.ExecutionKind); err != nil {
		return err
	}
	if err := header.Profile.Validate("event.header.profile"); err != nil {
		return err
	}
	if err := header.Route.Validate("event.header.route"); err != nil {
		return err
	}
	if header.NativeIdentity != nil {
		return header.NativeIdentity.Validate()
	}
	return nil
}

func validateExecutionItem(field string, item ExecutionItem) error {
	if err := validateRequired(field+".id", string(item.ID)); err != nil {
		return err
	}
	if err := validateRequired(field+".kind", item.Kind); err != nil {
		return err
	}
	if err := validateItemStatus(field+".status", item.Status); err != nil {
		return err
	}
	if err := validateSideEffectState(field+".side_effect_state", item.SideEffectState); err != nil {
		return err
	}
	return validateRaw(field+".payload", item.Payload)
}

func validateUsageMetric(field string, metric UsageMetric) error {
	if err := validateRequired(field+".kind", metric.Kind); err != nil {
		return err
	}
	if err := validateRequired(field+".unit", metric.Unit); err != nil {
		return err
	}
	if err := validateRequired(field+".scope", metric.Scope); err != nil {
		return err
	}
	if err := validateRequired(field+".source", metric.Source); err != nil {
		return err
	}
	if err := validateRequired(field+".quality", metric.Quality); err != nil {
		return err
	}
	if metric.Value < 0 || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
		return validationError(field+".value", "must be finite and non-negative")
	}
	return nil
}

func (event UnifiedExecutionEvent) Validate() error {
	if err := validateUTF8Strings("event", event); err != nil {
		return err
	}
	if err := event.Header.Validate(); err != nil {
		return err
	}
	tagCount := 0
	family := EventFamily("")
	if event.Lifecycle != nil {
		tagCount++
		family = EventFamilyLifecycle
		if err := validateRequired("event.lifecycle.kind", event.Lifecycle.Kind); err != nil {
			return err
		}
		if event.Lifecycle.Status != "" {
			if err := validateExecutionStatus("event.lifecycle.status", event.Lifecycle.Status); err != nil {
				return err
			}
		}
		if event.Lifecycle.PendingBackgroundWork < 0 {
			return validationError("event.lifecycle.pending_background_work", "must not be negative")
		}
	}
	if event.Intent != nil {
		tagCount++
		family = EventFamilyIntent
		if err := validateRequired("event.intent.kind", event.Intent.Kind); err != nil {
			return err
		}
		if event.Intent.Satisfaction != nil {
			if err := validateIntentSatisfaction("event.intent.satisfaction", *event.Intent.Satisfaction); err != nil {
				return err
			}
		}
	}
	if event.Mechanism != nil {
		tagCount++
		family = EventFamilyMechanism
		if err := validateRequired("event.mechanism.kind", event.Mechanism.Kind); err != nil {
			return err
		}
		mechanismTags := 0
		if event.Mechanism.Plan != nil {
			mechanismTags++
			if err := validateMechanismPlan("event.mechanism.plan", *event.Mechanism.Plan); err != nil {
				return err
			}
			if event.Header.MechanismPlanID != event.Mechanism.Plan.ID || event.Header.IntentID != event.Mechanism.Plan.IntentID {
				return validationError("event.header", "must match the mechanism plan identities")
			}
		}
		if event.Mechanism.Attempt != nil {
			mechanismTags++
			if err := validateMechanismAttempt("event.mechanism.attempt", *event.Mechanism.Attempt); err != nil {
				return err
			}
			if event.Header.MechanismAttemptID != event.Mechanism.Attempt.ID || event.Header.MechanismPlanID != event.Mechanism.Attempt.MechanismPlanID {
				return validationError("event.header", "must match the mechanism attempt identities")
			}
		}
		if mechanismTags > 1 {
			return validationError("event.mechanism", "must not contain multiple tagged values")
		}
	}
	if event.Model != nil {
		tagCount++
		family = EventFamilyModel
		if err := validateRequired("event.model.kind", event.Model.Kind); err != nil {
			return err
		}
		for index, part := range event.Model.Content {
			if err := validateContentPart(fmt.Sprintf("event.model.content[%d]", index), part); err != nil {
				return err
			}
		}
		if err := validateRaw("event.model.payload", event.Model.Payload); err != nil {
			return err
		}
		if event.Model.ResultOrigin != "" {
			if err := validateEventOrigin("event.model.result_origin", event.Model.ResultOrigin); err != nil {
				return err
			}
		}
		if event.Model.ActionID != "" && event.Header.ActionID != event.Model.ActionID {
			return validationError("event.header.action_id", "must match the model event action identity")
		}
		if event.Model.ExecutionItemID != "" && event.Header.ItemID != event.Model.ExecutionItemID {
			return validationError("event.header.item_id", "must match the model event execution item identity")
		}
		for index, metric := range event.Model.Usage {
			if err := validateUsageMetric(fmt.Sprintf("event.model.usage[%d]", index), metric); err != nil {
				return err
			}
		}
		if event.Model.Executed == nil {
			if event.Model.ResultOrigin != "" || event.Model.SyntheticReason != "" || event.Model.ExecutionItemID != "" {
				return validationError("event.model.executed", "is required for a model result association")
			}
		} else {
			if event.Model.ActionID == "" || event.Model.ExecutionItemID == "" {
				return validationError("event.model", "executed results require action and execution item identities")
			}
			if *event.Model.Executed {
				if event.Model.ResultOrigin == "" {
					return validationError("event.model.result_origin", "is required for an executed result")
				}
				if event.Model.SyntheticReason != "" {
					return validationError("event.model.synthetic_reason", "must be empty for an executed result")
				}
			} else {
				if event.Model.SyntheticReason == "" {
					return validationError("event.model.synthetic_reason", "is required for a synthetic result")
				}
				if event.Model.ResultOrigin != "" {
					return validationError("event.model.result_origin", "must be empty for a synthetic result")
				}
			}
		}
	}
	if event.Item != nil {
		tagCount++
		family = EventFamilyItem
		if err := validateRequired("event.item.kind", event.Item.Kind); err != nil {
			return err
		}
		if err := validateExecutionItem("event.item.item", event.Item.Item); err != nil {
			return err
		}
		if event.Header.ItemID != event.Item.Item.ID {
			return validationError("event.header.item_id", "must match the execution item identity")
		}
		if event.Item.Item.ActionID != "" && event.Header.ActionID != event.Item.Item.ActionID {
			return validationError("event.header.action_id", "must match the execution item action identity")
		}
		if event.Item.Item.AttemptID != "" && event.Header.MechanismAttemptID != event.Item.Item.AttemptID {
			return validationError("event.header.mechanism_attempt_id", "must match the execution item attempt identity")
		}
		if err := validateRaw("event.item.delta", event.Item.Delta); err != nil {
			return err
		}
	}
	if event.Effect != nil {
		tagCount++
		family = EventFamilyEffect
		if err := validateRequired("event.effect.kind", event.Effect.Kind); err != nil {
			return err
		}
		effectTags := 0
		if event.Effect.Effect != nil {
			effectTags++
			if err := event.Effect.Effect.Validate(); err != nil {
				return err
			}
			if event.Header.EffectID != event.Effect.Effect.ID || event.Header.MechanismAttemptID != event.Effect.Effect.MechanismAttemptID {
				return validationError("event.header", "must match the Effect identities")
			}
			if !containsValue(event.Effect.Effect.IntentIDs, event.Header.IntentID) {
				return validationError("event.header.intent_id", "must match an Effect intent identity")
			}
			if event.Effect.Effect.Payload.ToolCall != nil && event.Header.ActionID != event.Effect.Effect.Payload.ToolCall.ActionID {
				return validationError("event.header.action_id", "must match the tool Effect action identity")
			}
		}
		if event.Effect.Verification != nil {
			effectTags++
			if err := event.Effect.Verification.Validate(); err != nil {
				return err
			}
			if event.Header.VerificationID != event.Effect.Verification.ID {
				return validationError("event.header.verification_id", "must match the verification identity")
			}
			if len(event.Effect.Verification.IntentIDs) != 0 && !containsValue(event.Effect.Verification.IntentIDs, event.Header.IntentID) {
				return validationError("event.header.intent_id", "must match a verification intent identity")
			}
			if len(event.Effect.Verification.EffectIDs) != 0 && !containsValue(event.Effect.Verification.EffectIDs, event.Header.EffectID) {
				return validationError("event.header.effect_id", "must match a verification Effect identity")
			}
		}
		if effectTags != 1 {
			return validationError("event.effect", "must contain exactly one tagged value")
		}
	}
	if event.Control != nil {
		tagCount++
		family = EventFamilyControl
		if err := validateRequired("event.control.kind", event.Control.Kind); err != nil {
			return err
		}
		if event.Control.PendingBackgroundWork < 0 {
			return validationError("event.control.pending_background_work", "must not be negative")
		}
		if err := validateRaw("event.control.payload", event.Control.Payload); err != nil {
			return err
		}
		if event.Control.ApprovalID != "" && event.Header.ApprovalID != event.Control.ApprovalID {
			return validationError("event.header.approval_id", "must match the control approval identity")
		}
		if event.Control.ActionID != "" && event.Header.ActionID != event.Control.ActionID {
			return validationError("event.header.action_id", "must match the control action identity")
		}
		if event.Control.MechanismAttemptID != "" && event.Header.MechanismAttemptID != event.Control.MechanismAttemptID {
			return validationError("event.header.mechanism_attempt_id", "must match the control attempt identity")
		}
	}
	if event.Diagnostic != nil {
		tagCount++
		family = EventFamilyDiagnostic
		if err := validateRequired("event.diagnostic.kind", event.Diagnostic.Kind); err != nil {
			return err
		}
		if event.Diagnostic.Residual != nil {
			if err := validateResidual("event.diagnostic.residual", *event.Diagnostic.Residual); err != nil {
				return err
			}
		}
		if event.Diagnostic.Manifest != nil {
			if err := event.Diagnostic.Manifest.Validate(); err != nil {
				return err
			}
		}
		if err := validateRaw("event.diagnostic.payload", event.Diagnostic.Payload); err != nil {
			return err
		}
	}
	if tagCount != 1 {
		return validationError("event", "must contain exactly one tagged event payload")
	}
	if event.Header.Family != family {
		return validationError("event.header.family", "must match the tagged event payload")
	}
	return nil
}

func (command ExecutionCommand) Validate() error {
	if err := validateUTF8Strings("command", command); err != nil {
		return err
	}
	if command.SemanticVersion != SemanticVersionV1 {
		return validationError("command.semantic_version", "is not supported")
	}
	if err := validateRequired("command.execution_id", string(command.ExecutionID)); err != nil {
		return err
	}
	switch command.Kind {
	case CommandApproveAction, CommandDenyAction, CommandProvideInput, CommandCancelExecution,
		CommandInterrupt, CommandContinue, CommandProvideToolResult:
	default:
		return validationError("command.kind", "is not recognized")
	}
	if err := validateRequired("command.expected_execution_status", command.ExpectedExecutionStatus); err != nil {
		return err
	}
	if err := validateRequired("command.idempotency_key", command.IdempotencyKey); err != nil {
		return err
	}
	if err := validateRaw("command.payload", command.Payload); err != nil {
		return err
	}
	switch command.Kind {
	case CommandApproveAction, CommandDenyAction:
		if err := validateRequired("command.approval_id", string(command.ApprovalID)); err != nil {
			return err
		}
		if err := validateRequired("command.action_id", string(command.ActionID)); err != nil {
			return err
		}
		if err := validateRequired("command.mechanism_attempt_id", string(command.MechanismAttemptID)); err != nil {
			return err
		}
		if err := validateRequired("command.input_digest", command.InputDigest); err != nil {
			return err
		}
		if command.ActionRevision == 0 {
			return validationError("command.action_revision", "must be greater than zero")
		}
	case CommandProvideToolResult:
		if err := validateRequired("command.action_id", string(command.ActionID)); err != nil {
			return err
		}
		if err := validateRequired("command.mechanism_attempt_id", string(command.MechanismAttemptID)); err != nil {
			return err
		}
	}
	return nil
}

func validateUnifiedError(value UnifiedError) error {
	if err := validateRequired("result.error.kind", value.Kind); err != nil {
		return err
	}
	return validateRequired("result.error.phase", value.Phase)
}

func (result UnifiedExecutionResult) Validate() error {
	if err := validateUTF8Strings("result", result); err != nil {
		return err
	}
	if result.SemanticVersion != SemanticVersionV1 {
		return validationError("result.semantic_version", "is not supported")
	}
	if err := validateRequired("result.execution_id", string(result.ExecutionID)); err != nil {
		return err
	}
	if err := validateRequired("result.terminal_event_id", string(result.TerminalEventID)); err != nil {
		return err
	}
	if err := validateExecutionStatus("result.status", result.Status); err != nil {
		return err
	}
	if err := validateVerificationStatus("result.verification_status", result.VerificationStatus); err != nil {
		return err
	}
	if result.PendingBackgroundWork < 0 {
		return validationError("result.pending_background_work", "must not be negative")
	}
	if result.Status == ExecutionStatusSucceeded && result.VerificationStatus != VerificationVerified {
		return validationError("result.verification_status", "must be verified when execution succeeded")
	}
	if result.Status == ExecutionStatusSucceeded && result.Error != nil {
		return validationError("result.error", "must be empty when execution succeeded")
	}
	if result.Status == ExecutionStatusSucceeded && len(result.IntentSatisfaction) == 0 {
		return validationError("result.intent_satisfaction", "must not be empty when execution succeeded")
	}
	satisfactionIDs := make([]IntentID, 0, len(result.IntentSatisfaction))
	for index, satisfaction := range result.IntentSatisfaction {
		if err := validateIntentSatisfaction(fmt.Sprintf("result.intent_satisfaction[%d]", index), satisfaction); err != nil {
			return err
		}
		satisfactionIDs = append(satisfactionIDs, satisfaction.IntentID)
		if result.Status == ExecutionStatusSucceeded && satisfaction.Status != IntentSatisfied {
			return validationError("result.intent_satisfaction", "must be satisfied when execution succeeded")
		}
	}
	if err := validateUnique("result.intent_satisfaction.intent_id", satisfactionIDs); err != nil {
		return err
	}
	satisfactionSet := make(map[IntentID]struct{}, len(satisfactionIDs))
	for _, intentID := range satisfactionIDs {
		satisfactionSet[intentID] = struct{}{}
	}
	attemptIDs := make([]MechanismAttemptID, 0, len(result.MechanismTrace))
	for index, attempt := range result.MechanismTrace {
		if err := validateMechanismAttempt(fmt.Sprintf("result.mechanism_trace[%d]", index), attempt); err != nil {
			return err
		}
		attemptIDs = append(attemptIDs, attempt.ID)
	}
	if err := validateUnique("result.mechanism_trace.id", attemptIDs); err != nil {
		return err
	}
	attemptSet := make(map[MechanismAttemptID]struct{}, len(attemptIDs))
	attemptIndex := make(map[MechanismAttemptID]int, len(attemptIDs))
	for index, attemptID := range attemptIDs {
		attemptSet[attemptID] = struct{}{}
		attemptIndex[attemptID] = index
	}
	for index, attempt := range result.MechanismTrace {
		if attempt.RetryOf != "" {
			previous, exists := attemptIndex[attempt.RetryOf]
			if !exists || previous >= index {
				return validationError("result.mechanism_trace.retry_of", "must reference an earlier result attempt")
			}
		}
		if attempt.SupersededBy != "" {
			later, exists := attemptIndex[attempt.SupersededBy]
			if !exists || later <= index {
				return validationError("result.mechanism_trace.superseded_by", "must reference a later result attempt")
			}
		}
	}
	effectIDs := make(map[EffectID]struct{}, len(result.Effects))
	effectsByID := make(map[EffectID]EffectRecord, len(result.Effects))
	for _, effect := range result.Effects {
		if err := effect.Validate(); err != nil {
			return err
		}
		if _, duplicate := effectIDs[effect.ID]; duplicate {
			return validationError("result.effects.id", "must not contain duplicates")
		}
		if _, exists := attemptSet[effect.MechanismAttemptID]; !exists {
			return validationError("result.effects.mechanism_attempt_id", "must reference a result mechanism attempt")
		}
		for _, intentID := range effect.IntentIDs {
			if _, exists := satisfactionSet[intentID]; !exists {
				return validationError("result.effects.intent_ids", "must reference a result intent satisfaction")
			}
		}
		for _, superseded := range effect.SupersedesEffectIDs {
			stale, exists := effectsByID[superseded]
			if !exists {
				return validationError("result.effects.supersedes_effect_ids", "must reference an earlier result Effect")
			}
			sharesIntent := false
			for _, intentID := range effect.IntentIDs {
				if containsValue(stale.IntentIDs, intentID) {
					sharesIntent = true
					break
				}
			}
			if !sharesIntent {
				return validationError("result.effects.supersedes_effect_ids", "must share an intent with the superseded Effect")
			}
		}
		effectIDs[effect.ID] = struct{}{}
		effectsByID[effect.ID] = effect
	}
	verificationIDs := make(map[VerificationID]struct{}, len(result.Verifications))
	verificationsByID := make(map[VerificationID]VerificationRecord, len(result.Verifications))
	for _, verification := range result.Verifications {
		if err := verification.Validate(); err != nil {
			return err
		}
		if _, duplicate := verificationIDs[verification.ID]; duplicate {
			return validationError("result.verifications.id", "must not contain duplicates")
		}
		verificationIDs[verification.ID] = struct{}{}
		verificationsByID[verification.ID] = verification
		for _, effectID := range verification.EffectIDs {
			if _, exists := effectIDs[effectID]; !exists {
				return validationError("result.verifications.effect_ids", "must reference a result Effect")
			}
		}
		for _, intentID := range verification.IntentIDs {
			if _, exists := satisfactionSet[intentID]; !exists {
				return validationError("result.verifications.intent_ids", "must reference a result intent satisfaction")
			}
		}
	}
	for _, satisfaction := range result.IntentSatisfaction {
		for _, effectID := range satisfaction.EffectIDs {
			observed, exists := effectsByID[effectID]
			if !exists {
				return validationError("result.intent_satisfaction.effect_ids", "must reference a result Effect")
			}
			if !containsValue(observed.IntentIDs, satisfaction.IntentID) {
				return validationError("result.intent_satisfaction.effect_ids", "must reference an Effect associated with the same intent")
			}
		}
	}
	for _, observed := range result.Effects {
		for _, verificationID := range observed.VerificationRefs {
			verification, exists := verificationsByID[verificationID]
			if !exists {
				return validationError("result.effects.verification_refs", "must reference a result verification")
			}
			if !containsValue(verification.EffectIDs, observed.ID) {
				return validationError("result.effects.verification_refs", "must reference a verification associated with the same Effect")
			}
		}
		for _, stale := range observed.SupersedesEffectIDs {
			if _, exists := effectIDs[stale]; !exists {
				return validationError("result.effects.supersedes_effect_ids", "must reference a result Effect")
			}
		}
	}
	for _, verification := range result.Verifications {
		for _, effectID := range verification.EffectIDs {
			if !containsValue(effectsByID[effectID].VerificationRefs, verification.ID) {
				return validationError("result.verifications.effect_ids", "must be reciprocally referenced by the associated Effect")
			}
		}
	}
	for index, part := range result.FinalContent {
		if err := validateContentPart(fmt.Sprintf("result.final_content[%d]", index), part); err != nil {
			return err
		}
	}
	actionIDs := make([]ItemID, 0, len(result.Actions))
	for index, item := range result.Actions {
		if err := validateExecutionItem(fmt.Sprintf("result.actions[%d]", index), item); err != nil {
			return err
		}
		actionIDs = append(actionIDs, item.ID)
	}
	if err := validateUnique("result.actions.id", actionIDs); err != nil {
		return err
	}
	for index, change := range result.WorkspaceChanges {
		if err := validateWorkspaceChange(fmt.Sprintf("result.workspace_changes[%d]", index), change); err != nil {
			return err
		}
	}
	for index, metric := range result.UsageMetrics {
		if err := validateUsageMetric(fmt.Sprintf("result.usage_metrics[%d]", index), metric); err != nil {
			return err
		}
	}
	if err := validateMappingReport(result.MappingReport); err != nil {
		return err
	}
	if err := result.ContextManifest.Validate(); err != nil {
		return err
	}
	for index, residual := range result.Residuals {
		if err := validateResidual(fmt.Sprintf("result.residuals[%d]", index), residual); err != nil {
			return err
		}
	}
	if result.Error != nil {
		if err := validateUnifiedError(*result.Error); err != nil {
			return err
		}
	}
	return nil
}
