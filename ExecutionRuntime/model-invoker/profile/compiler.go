package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type Compiler struct {
	registry *Registry
	now      time.Time
}

func NewCompiler(registry *Registry, now time.Time) (*Compiler, error) {
	if registry == nil || now.IsZero() {
		return nil, &Error{Code: ErrorInvalidProfile, Operation: "new_compiler", Message: "registry and validation time are required"}
	}
	return &Compiler{registry: registry, now: now}, nil
}

type CompileInput struct {
	Request                union.UnifiedExecutionRequest
	ActualManifest         InjectionManifest
	PolicyLayers           []PolicyLayer
	PaperOnly              bool
	RequiredNativeFeatures []string
	AutomaticModelFallback bool
}

type Compilation struct {
	Plan               union.PreparedExecutionPlan
	Profile            SemanticRouteProfile
	EffectivePolicy    RuntimePolicy
	ManifestEvaluation ManifestEvaluation
	MappingReport      MappingReportV2
	Residuals          []CapabilityResidual
}

func (compiler *Compiler) Compile(input CompileInput) (Compilation, error) {
	if compiler == nil || compiler.registry == nil {
		return Compilation{}, &Error{Code: ErrorInvalidProfile, Operation: "compile", Message: "compiler is not initialized"}
	}
	if err := input.Request.Validate(); err != nil {
		return Compilation{}, &Error{Code: ErrorProfileIncompatible, Operation: "validate_request", Message: err.Error()}
	}
	selector, expectedVersion, err := selectorFromUnion(input.Request.ProfileSelector)
	if err != nil {
		return Compilation{}, err
	}
	selected, err := compiler.registry.Resolve(selector)
	if err != nil {
		return Compilation{}, err
	}
	if expectedVersion != "" && string(selected.Version) != expectedVersion {
		return Compilation{}, &Error{Code: ErrorResolutionFailed, Operation: "resolve", Message: "exact profile version does not match"}
	}
	executionKind, err := resolveExecutionKind(input.Request.ExecutionKind, selected.Selection.ExecutionSurface)
	if err != nil {
		return Compilation{}, err
	}
	effectivePolicy, err := MergeRuntimePolicy(selected.DefaultPolicy, input.PolicyLayers...)
	if err != nil {
		return Compilation{}, &Error{Code: ErrorPolicyRejected, Operation: "merge_policy", Message: err.Error()}
	}
	if err := validatePolicyIdentity(effectivePolicy.Identity, selected.Selection); err != nil {
		return Compilation{}, err
	}
	if err := validateRequestLimits(input.Request, effectivePolicy); err != nil {
		return Compilation{}, err
	}
	if input.AutomaticModelFallback {
		return Compilation{}, &Error{
			Code: ErrorProfileIncompatible, Operation: "compile", Path: "fallback.model",
			Message: "automatic model fallback requires a new Profile resolution and Route fingerprint",
		}
	}
	if err := validateNativeFeatures(selected.HarnessCapability, input.RequiredNativeFeatures, effectivePolicy); err != nil {
		return Compilation{}, err
	}

	manifestEvaluation := ManifestEvaluation{Allowed: true}
	var actualManifestDigest string
	if input.PaperOnly {
		if input.ActualManifest.ProbeStatus != ManifestProbeNotRun || len(input.ActualManifest.Fields) != 0 {
			return Compilation{}, &Error{Code: ErrorProfileIncompatible, Operation: "compile", Message: "paper-only compilation requires an unrun actual manifest"}
		}
		actualManifestDigest = DigestString("actual-manifest:not-run")
	} else {
		manifestEvaluation, err = CompareManifests(
			selected.HarnessCapability.ExpectedManifest,
			input.ActualManifest,
			selected.ContextMode,
			effectivePolicy.AllowedP2ManifestPaths.Values,
		)
		if err != nil {
			return Compilation{}, err
		}
		if !manifestEvaluation.Allowed {
			return Compilation{}, &Error{Code: ErrorManifestDrift, Operation: "compile", Message: "actual Harness manifest violates the selected context mode"}
		}
		actualManifestDigest, err = input.ActualManifest.Digest(true)
		if err != nil {
			return Compilation{}, err
		}
	}

	graph, err := normalizeIntentGraph(input.Request.IntentGraph)
	if err != nil {
		return Compilation{}, err
	}
	mechanisms, decisions, residuals, err := compileMechanisms(graph, selected, effectivePolicy, input.Request.DegradationPolicy)
	if err != nil {
		return Compilation{}, err
	}
	for _, manifestResidual := range manifestEvaluation.Residuals {
		residuals = append(residuals, CapabilityResidual{
			Path: manifestResidual.Path, Kind: "manifest_drift", Severity: string(manifestResidual.Level),
			Impact: manifestResidual.Impact, Mitigation: manifestResidual.Mitigation,
		})
	}
	if input.PaperOnly {
		residuals = append(residuals, CapabilityResidual{
			Path: "context.actual_manifest", Kind: "probe_not_run", Severity: "audit",
			Impact:     "paper compilation does not prove the live Harness surface",
			Mitigation: "run preflight and recompile before Provider or model contact",
		})
	}

	mappingV2 := MappingReportV2{Decisions: decisions}
	if err := mappingV2.finalize(); err != nil {
		return Compilation{}, &Error{Code: ErrorProfileIncompatible, Operation: "mapping_report", Message: err.Error()}
	}
	expectedSummary, expectedManifestDigest, err := projectExpectedManifest(selected)
	if err != nil {
		return Compilation{}, err
	}
	selectionDigest, err := selected.Selection.Digest()
	if err != nil {
		return Compilation{}, err
	}
	profileDigest, err := selected.Digest(compiler.now)
	if err != nil {
		return Compilation{}, err
	}
	policyDigest, err := effectivePolicy.Digest()
	if err != nil {
		return Compilation{}, err
	}
	routeFingerprint, err := digestJSON(struct {
		SelectionDigest      string
		HarnessStack         []HarnessComponent
		ProfileDigest        string
		ExpectedManifest     string
		ActualManifest       string
		ToolRegistry         string
		Environment          string
		RuntimePolicy        string
		SemanticCodecVersion string
		ContextMode          ContextMode
	}{
		SelectionDigest: selectionDigest, HarnessStack: selected.Selection.HarnessStack,
		ProfileDigest: profileDigest, ExpectedManifest: expectedManifestDigest, ActualManifest: actualManifestDigest,
		ToolRegistry: expectedManifestDigest, Environment: selected.HarnessCapability.SanitizedEnvironmentDigest,
		RuntimePolicy: policyDigest, SemanticCodecVersion: selected.SemanticCodecVersion, ContextMode: selected.ContextMode,
	})
	if err != nil {
		return Compilation{}, err
	}
	requestDigest, err := input.Request.Digest()
	if err != nil {
		return Compilation{}, err
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion:  union.SemanticVersionV1,
		ExecutionID:      input.Request.ExecutionID,
		Profile:          union.VersionedIdentity{ID: string(selected.ID), Version: string(selected.Version)},
		Route:            union.VersionedIdentity{ID: string(selected.Selection.BaseRouteID), Version: selected.Selection.ModelRevision},
		ProfileKeyDigest: selectionDigest,
		ExecutionKind:    executionKind,
		IntentGraph:      graph,
		Mechanisms:       mechanisms,
		ExpectedManifest: expectedSummary,
		Residuals:        projectUnionResiduals(residuals),
		MappingReport:    projectUnionMapping(mappingV2),
		RouteFingerprint: routeFingerprint,
		Metadata: map[string]string{
			"request_digest":         requestDigest,
			"runtime_policy_digest":  policyDigest,
			"actual_manifest_digest": actualManifestDigest,
			"mapping_v2_digest":      mappingV2.Digest,
		},
	}
	planDigest, err := plan.ComputeDigest()
	if err != nil {
		return Compilation{}, &Error{Code: ErrorProfileIncompatible, Operation: "finalize_plan", Message: err.Error()}
	}
	plan.Digest = planDigest
	return Compilation{
		Plan: plan, Profile: selected.Clone(), EffectivePolicy: effectivePolicy,
		ManifestEvaluation: manifestEvaluation, MappingReport: mappingV2, Residuals: residuals,
	}, nil
}

func selectorFromUnion(selector union.ProfileSelector) (ProfileSelector, string, error) {
	if selector.Exact != nil {
		return ProfileSelector{ID: ProfileID(selector.Exact.ID)}, selector.Exact.Version, nil
	}
	constraints := SelectionConstraints{}
	for key, value := range selector.Constraints {
		switch key {
		case "base_route_id":
			constraints.BaseRouteID = upstream.RouteID(value)
		case "provider":
			constraints.Provider = upstream.ProviderID(value)
		case "model_id":
			constraints.ModelID = value
		case "model_revision":
			constraints.ModelRevision = value
		case "deployment":
			constraints.Deployment = upstream.DeploymentID(value)
		case "region":
			constraints.Region = value
		case "endpoint_identity":
			constraints.EndpointIdentity = upstream.EndpointID(value)
		case "protocol":
			constraints.Protocol = upstream.ProtocolID(value)
		case "protocol_schema_version":
			constraints.ProtocolSchemaVersion = value
		case "offering":
			constraints.Offering = upstream.OfferingID(value)
		case "auth_route":
			constraints.AuthRoute = value
		case "execution_surface":
			constraints.ExecutionSurface = ExecutionSurface(value)
		case "harness_stack_digest":
			constraints.HarnessStackDigest = value
		default:
			return ProfileSelector{}, "", &Error{
				Code: ErrorResolutionFailed, Operation: "resolve",
				Message: fmt.Sprintf("unknown profile selection constraint %q", key),
			}
		}
	}
	return ProfileSelector{Constraints: constraints}, "", nil
}

func resolveExecutionKind(requested union.ExecutionKind, surface ExecutionSurface) (union.ExecutionKind, error) {
	actual := union.ExecutionKindAgent
	if surface == ExecutionSurfaceDirectAPI {
		actual = union.ExecutionKindModel
	}
	if requested == union.ExecutionKindAuto {
		return actual, nil
	}
	if requested != actual {
		return "", &Error{Code: ErrorProfileIncompatible, Operation: "resolve_execution_kind", Message: "requested execution kind does not match the exact Profile"}
	}
	return actual, nil
}

func validatePolicyIdentity(identity IdentityLock, selection ProfileSelectionKey) error {
	if identity.BaseRouteID != selection.BaseRouteID || identity.Provider != selection.Provider ||
		identity.Offering != selection.Offering || identity.ModelID != selection.ModelID {
		return &Error{Code: ErrorPolicyRejected, Operation: "validate_identity", Message: "RuntimePolicy identity is not bound to the selected Profile"}
	}
	return nil
}

func validateRequestLimits(request union.UnifiedExecutionRequest, policy RuntimePolicy) error {
	checks := []struct {
		path      string
		requested int64
		limit     int64
	}{
		{"budget.max_wall_time", int64(request.Budget.MaxWallTime), int64(policy.MaxWallTime)},
		{"budget.max_tool_actions", int64(request.Budget.MaxToolActions), int64(policy.MaxActions)},
		{"tool_policy.max_actions", int64(request.ToolPolicy.MaxActions), int64(policy.MaxActions)},
		{"execution_policy.max_concurrency", int64(request.ExecutionPolicy.MaxConcurrency), int64(policy.MaxConcurrency)},
		{"tool_policy.parallelism", int64(request.ToolPolicy.Parallelism), int64(policy.MaxConcurrency)},
	}
	for _, check := range checks {
		if check.limit > 0 && check.requested > check.limit {
			return &Error{
				Code: ErrorPolicyRejected, Operation: "validate_request_limits", Path: check.path,
				Message: "request exceeds the effective RuntimePolicy limit",
			}
		}
	}
	if policy.Network.Mode == NetworkDenied && request.ExecutionPolicy.NetworkPolicy != "" &&
		request.ExecutionPolicy.NetworkPolicy != string(NetworkDenied) {
		return &Error{
			Code: ErrorPolicyRejected, Operation: "validate_request_limits", Path: "execution_policy.network_policy",
			Message: "request network policy exceeds the effective RuntimePolicy",
		}
	}
	return nil
}

func validateNativeFeatures(harness HarnessCapabilityProfile, required []string, policy RuntimePolicy) error {
	requiredSet := make(map[string]struct{}, len(required))
	for _, feature := range required {
		if !validStableName(feature) {
			return &Error{Code: ErrorProfileIncompatible, Operation: "validate_features", Message: "native feature name is invalid"}
		}
		requiredSet[feature] = struct{}{}
		if containsString(harness.ForbiddenNativeFeatures, feature) {
			return &Error{Code: ErrorProfileIncompatible, Operation: "validate_features", Path: feature, Message: "native feature is forbidden by this Profile"}
		}
		if !containsString(harness.SupportedNativeFeatures, feature) {
			return &Error{Code: ErrorProfileIncompatible, Operation: "validate_features", Path: feature, Message: "native feature is unavailable in this Profile"}
		}
	}
	for _, conflict := range harness.FeatureConflicts {
		_, left := requiredSet[conflict.Left]
		_, right := requiredSet[conflict.Right]
		if left && right {
			return &Error{Code: ErrorProfileIncompatible, Operation: "validate_features", Message: fmt.Sprintf("native features %q and %q are mutually exclusive", conflict.Left, conflict.Right)}
		}
	}
	if _, legacy := requiredSet["legacy_wire"]; legacy && !policy.AllowLegacyCompatibility {
		return &Error{Code: ErrorProfileIncompatible, Operation: "validate_features", Path: "legacy_wire", Message: "legacy compatibility is denied by RuntimePolicy"}
	}
	return nil
}

type rankedMechanism struct {
	capability MechanismCapability
	score      int
	preference int
}

func compileMechanisms(
	graph union.IntentGraph,
	routeProfile SemanticRouteProfile,
	policy RuntimePolicy,
	degradation union.DegradationPolicy,
) ([]union.MechanismPlan, []MappingDecisionV2, []CapabilityResidual, error) {
	var plans []union.MechanismPlan
	var decisions []MappingDecisionV2
	var residuals []CapabilityResidual
	for _, intent := range graph.Nodes {
		if err := validateIntentAgainstPolicy(intent, policy); err != nil {
			return nil, nil, nil, err
		}
		candidates := feasibleMechanisms(intent, routeProfile, policy, degradation)
		if len(candidates) == 0 {
			return nil, nil, nil, &Error{
				Code: ErrorCapabilityRejected, Operation: "compile_intent", Path: string(intent.ID),
				Message: fmt.Sprintf("no policy-allowed and verifiable mechanism exists for intent %q", intent.Kind),
			}
		}
		sortRankedMechanisms(candidates)
		// These are final policy caps, where zero means no fallbacks. minPositive
		// is only appropriate while merging sparse layers, where zero means the
		// layer did not specify a tighter value.
		fallbackLimit := policy.MaxFallbacks
		if policy.RetryFallback.MaxFallbacks < fallbackLimit {
			fallbackLimit = policy.RetryFallback.MaxFallbacks
		}
		selected := []rankedMechanism{candidates[0]}
		for _, candidate := range candidates[1:] {
			if len(selected)-1 >= fallbackLimit {
				break
			}
			if candidate.capability.FallbackSafe &&
				(!policy.RetryFallback.AllowedFallbackMechanismIDs.Specified ||
					containsString(policy.RetryFallback.AllowedFallbackMechanismIDs.Values, candidate.capability.ID)) {
				selected = append(selected, candidate)
			}
		}
		ids := make([]union.MechanismPlanID, len(selected))
		for index, selectedMechanism := range selected {
			ids[index] = union.MechanismPlanID("mp." + string(intent.ID) + "." + selectedMechanism.capability.ID)
		}
		for index, selectedMechanism := range selected {
			capability := selectedMechanism.capability
			plan := union.MechanismPlan{
				ID: ids[index], IntentID: intent.ID, Kind: capability.Kind, Origin: capability.Origin,
				Owner: capability.Owner, SelectionAuthority: capability.SelectionAuthority,
				CapabilityRef: capability.ID, PreferredRank: selectedMechanism.preference,
				HardConstraints:    append([]string(nil), capability.HardConstraints...),
				ExpectedEffects:    append([]string(nil), capability.ExpectedEffects...),
				VerificationPlanID: union.VerificationID("verify." + string(intent.ID)),
				SemanticFidelity:   capability.Fidelity,
			}
			if index == 0 && len(ids) > 1 {
				plan.FallbackPlanIDs = append([]union.MechanismPlanID(nil), ids[1:]...)
			}
			plans = append(plans, plan)
		}
		primary := selected[0].capability
		action := mappingActionForFidelity(primary.Fidelity, routeProfile.Selection.ExecutionSurface)
		decisions = append(decisions, MappingDecisionV2{
			SourcePath: "intent_graph." + string(intent.ID), TargetPath: "mechanisms." + primary.ID,
			Action: action, Origin: primary.Origin,
			Detail:   "Intent compiled after capability, policy, observability, and verifier hard filters",
			Evidence: routeProfile.HarnessCapability.ProbeDigest,
		})
		if primary.Fidelity == union.SemanticFidelityDegraded {
			residuals = append(residuals, CapabilityResidual{
				Path: "intent_graph." + string(intent.ID), Capability: primary.ID, Kind: "degraded_fidelity", Severity: "warning",
				Impact: "selected mechanism does not preserve exact semantics", Mitigation: "retain explicit degradation and verification",
			})
		}
	}
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].IntentID != plans[j].IntentID {
			return plans[i].IntentID < plans[j].IntentID
		}
		return plans[i].ID < plans[j].ID
	})
	return plans, decisions, residuals, nil
}

func feasibleMechanisms(intent union.IntentNode, profile SemanticRouteProfile, policy RuntimePolicy, degradation union.DegradationPolicy) []rankedMechanism {
	preferences := behaviorPreferences(profile.ModelBehavior, intent.Kind)
	var result []rankedMechanism
	for _, mechanism := range profile.HarnessCapability.AvailableMechanisms {
		if !containsIntentKind(mechanism.IntentKinds, intent.Kind) || mechanism.Origin == union.CapabilityOriginUnavailable ||
			!mechanism.ModelAddressable || !mechanism.EffectObservable || !mechanism.VerifierAvailable {
			continue
		}
		if policy.Approval.Strength != ApprovalNone && !mechanism.ApprovalSupported {
			continue
		}
		if policy.AllowedMechanismIDs.Specified && !containsString(policy.AllowedMechanismIDs.Values, mechanism.ID) {
			continue
		}
		if containsString(policy.DeniedMechanismIDs, mechanism.ID) || intersectsStrings(policy.DeniedCapabilities, mechanism.Capabilities) {
			continue
		}
		if !fidelityAccepted(intent.AcceptedFidelity, mechanism.Fidelity, degradation) {
			continue
		}
		preference, preferred := preferences[mechanism.ID]
		if !preferred {
			preference = 1000
		}
		score := mechanism.Score.Total() + policy.MechanismPreferenceWeight[mechanism.ID]
		if preferred {
			score += 100 - preference
		}
		result = append(result, rankedMechanism{capability: mechanism, score: score, preference: preference})
	}
	return result
}

func sortRankedMechanisms(values []rankedMechanism) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if fidelityRank(left.capability.Fidelity) != fidelityRank(right.capability.Fidelity) {
			return fidelityRank(left.capability.Fidelity) > fidelityRank(right.capability.Fidelity)
		}
		if left.capability.VerifierAvailable != right.capability.VerifierAvailable {
			return left.capability.VerifierAvailable
		}
		if left.capability.Score.OperationalRisk != right.capability.Score.OperationalRisk {
			return left.capability.Score.OperationalRisk < right.capability.Score.OperationalRisk
		}
		if left.preference != right.preference {
			return left.preference < right.preference
		}
		return left.capability.ID < right.capability.ID
	})
}

func validateIntentAgainstPolicy(intent union.IntentNode, policy RuntimePolicy) error {
	if intent.Kind == union.IntentComputerUse && !policy.Computer.Enabled {
		return &Error{Code: ErrorCapabilityRejected, Operation: "compile_intent", Path: string(intent.ID), Message: "Computer Use is unavailable under RuntimePolicy"}
	}
	if policy.AllowedIntentKinds.Specified && !containsIntentKind(policy.AllowedIntentKinds.Values, intent.Kind) {
		return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: string(intent.ID), Message: "intent kind is not in the RuntimePolicy allow set"}
	}
	if containsIntentKind(policy.DeniedIntentKinds, intent.Kind) {
		return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: string(intent.ID), Message: "intent kind is denied by RuntimePolicy"}
	}
	switch intent.Kind {
	case union.IntentCreateFile, union.IntentModifyFile, union.IntentRewriteFile, union.IntentDeleteFile,
		union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory:
		if !pathAllowed(intent.Target, policy.Filesystem.WritablePaths) {
			return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: intent.Target, Message: "filesystem target is outside the writable allow set"}
		}
		if pathDenied(intent.Target, policy.Filesystem.DeniedPaths) {
			return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: intent.Target, Message: "filesystem target is explicitly denied"}
		}
		if (intent.Kind == union.IntentDeleteFile || intent.Kind == union.IntentDeleteDirectory) && !policy.Filesystem.AllowDelete {
			return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: intent.Target, Message: "delete is denied"}
		}
		if intent.Kind == union.IntentMoveFile && !policy.Filesystem.AllowMove {
			return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: intent.Target, Message: "move is denied"}
		}
		if intent.Kind == union.IntentMoveFile {
			destination, err := moveDestination(intent.Specification)
			if err != nil {
				return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: string(intent.ID), Message: err.Error()}
			}
			if !pathAllowed(destination, policy.Filesystem.WritablePaths) || pathDenied(destination, policy.Filesystem.DeniedPaths) {
				return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: destination, Message: "move destination is not writable under RuntimePolicy"}
			}
		}
	case union.IntentExecuteCode:
		if err := validateExecuteCodeSpec(intent.Specification, policy); err != nil {
			return &Error{Code: ErrorPolicyRejected, Operation: "compile_intent", Path: string(intent.ID), Message: err.Error()}
		}
	}
	return nil
}

func moveDestination(raw json.RawMessage) (string, error) {
	var specification struct {
		Destination string `json:"destination"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &specification) != nil || !filepath.IsAbs(specification.Destination) ||
		filepath.Clean(specification.Destination) != specification.Destination {
		return "", fmt.Errorf("MoveFile requires an absolute clean destination specification")
	}
	return specification.Destination, nil
}

type executeCodeSpecification struct {
	Argv      []string `json:"argv"`
	CWD       string   `json:"cwd"`
	Network   bool     `json:"network"`
	TimeoutMS int64    `json:"timeout_ms"`
}

func validateExecuteCodeSpec(raw json.RawMessage, policy RuntimePolicy) error {
	var specification executeCodeSpecification
	if len(raw) == 0 {
		return fmt.Errorf("ExecuteCode requires a valid exact argv specification")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&specification); err != nil || len(specification.Argv) == 0 {
		return fmt.Errorf("ExecuteCode requires a valid exact argv specification")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("ExecuteCode requires a valid exact argv specification")
	}
	if !argvAllowed(specification.Argv, policy.Process.AllowedArgv) || containsArgv(policy.Process.DeniedArgv, specification.Argv) {
		return fmt.Errorf("ExecuteCode argv is not allowed")
	}
	if !pathAllowed(specification.CWD, policy.Process.AllowedCWDs) {
		return fmt.Errorf("ExecuteCode cwd is not allowed")
	}
	if specification.Network && (policy.Process.NetworkAccess == NetworkDenied || policy.Network.Mode == NetworkDenied) {
		return fmt.Errorf("ExecuteCode network is denied")
	}
	if specification.TimeoutMS < 0 || specification.TimeoutMS > int64((1<<63-1)/int64(time.Millisecond)) {
		return fmt.Errorf("ExecuteCode timeout is invalid")
	}
	timeout := time.Duration(specification.TimeoutMS) * time.Millisecond
	if policy.Process.MaxTimeout > 0 && timeout > policy.Process.MaxTimeout {
		return fmt.Errorf("ExecuteCode timeout exceeds RuntimePolicy")
	}
	return nil
}

func normalizeIntentGraph(graph union.IntentGraph) (union.IntentGraph, error) {
	if err := graph.Validate(); err != nil {
		return union.IntentGraph{}, &Error{Code: ErrorProfileIncompatible, Operation: "normalize_intents", Message: err.Error()}
	}
	nodes := make(map[union.IntentID]union.IntentNode, len(graph.Nodes))
	dependents := make(map[union.IntentID][]union.IntentID)
	degree := make(map[union.IntentID]int, len(graph.Nodes))
	for _, node := range graph.Nodes {
		clone := node
		clone.DependsOn = append([]union.IntentID(nil), node.DependsOn...)
		sort.Slice(clone.DependsOn, func(i, j int) bool { return clone.DependsOn[i] < clone.DependsOn[j] })
		clone.AcceptedFidelity = append([]union.SemanticFidelity(nil), node.AcceptedFidelity...)
		sort.Slice(clone.AcceptedFidelity, func(i, j int) bool { return clone.AcceptedFidelity[i] < clone.AcceptedFidelity[j] })
		nodes[node.ID] = clone
		degree[node.ID] = len(node.DependsOn)
		for _, dependency := range node.DependsOn {
			dependents[dependency] = append(dependents[dependency], node.ID)
		}
	}
	var ready []union.IntentID
	for id, count := range degree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	normalized := union.IntentGraph{Nodes: make([]union.IntentNode, 0, len(nodes))}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		normalized.Nodes = append(normalized.Nodes, nodes[id])
		for _, dependent := range dependents[id] {
			degree[dependent]--
			if degree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(normalized.Nodes) != len(nodes) {
		return union.IntentGraph{}, &Error{Code: ErrorProfileIncompatible, Operation: "normalize_intents", Message: "intent graph is cyclic"}
	}
	return normalized, nil
}

func projectExpectedManifest(profile SemanticRouteProfile) (union.ContextManifestSummary, string, error) {
	expected := profile.HarnessCapability.ExpectedManifest.Clone()
	expected.normalize()
	components := make([]union.ManifestComponent, 0, len(expected.Fields)+len(profile.Selection.HarnessStack))
	for _, field := range expected.Fields {
		components = append(components, union.ManifestComponent{
			Kind: "injection_field", Name: field.Path, State: string(field.State),
			Digest: DigestString(field.Value), Opaque: field.State == ManifestFieldOpaque,
		})
	}
	for _, component := range profile.Selection.HarnessStack {
		components = append(components, union.ManifestComponent{
			Kind: "harness_component", Name: component.Component, Version: component.Version,
			State: "expected", Digest: component.BinaryDigest,
		})
	}
	sort.Slice(components, func(i, j int) bool {
		if components[i].Kind != components[j].Kind {
			return components[i].Kind < components[j].Kind
		}
		return components[i].Name < components[j].Name
	})
	summary := union.ContextManifestSummary{
		ID: "expected." + string(profile.ID), Version: string(profile.Version), Mode: string(profile.ContextMode),
		Components: components, OpaqueFields: append([]string(nil), profile.HarnessCapability.OpaqueFields...),
	}
	sort.Strings(summary.OpaqueFields)
	digest, err := summary.ComputeDigest()
	if err != nil {
		return union.ContextManifestSummary{}, "", err
	}
	summary.Digest = digest
	return summary, digest, nil
}

func projectUnionMapping(report MappingReportV2) union.MappingReport {
	decisions := make([]union.MappingDecision, len(report.Decisions))
	for index, decision := range report.Decisions {
		decisions[index] = union.MappingDecision{
			Path: decision.SourcePath, Fidelity: unionFidelityForMapping(decision.Action),
			Origin: decision.Origin, Detail: decision.Detail,
		}
	}
	return union.MappingReport{Decisions: decisions, Digest: report.Digest}
}

func projectUnionResiduals(residuals []CapabilityResidual) []union.Residual {
	result := make([]union.Residual, len(residuals))
	for index, residual := range residuals {
		result[index] = union.Residual{
			Path: residual.Path, Capability: residual.Capability, Kind: residual.Kind,
			Severity: residual.Severity, Impact: residual.Impact, Mitigation: residual.Mitigation,
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func behaviorPreferences(profile ModelBehaviorProfile, intentKind union.IntentKind) map[string]int {
	result := make(map[string]int)
	for _, preference := range profile.MechanismPreferences {
		if preference.IntentKind != intentKind {
			continue
		}
		current, exists := result[preference.MechanismID]
		if !exists || preference.Rank < current {
			result[preference.MechanismID] = preference.Rank
		}
	}
	return result
}

func fidelityAccepted(accepted []union.SemanticFidelity, candidate union.SemanticFidelity, degradation union.DegradationPolicy) bool {
	if candidate == union.SemanticFidelityUnavailable {
		return false
	}
	if len(accepted) == 0 {
		return candidate == union.SemanticFidelityExact
	}
	for _, value := range accepted {
		if value == candidate {
			return true
		}
	}
	if candidate == union.SemanticFidelityDegraded && degradation.Default == union.DegradationDefaultAllowReported {
		return containsSemanticFidelity(degradation.AllowedFidelities, candidate)
	}
	return false
}

func pathAllowed(path string, constraint PathSetConstraint) bool {
	if !constraint.Specified || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, allowed := range constraint.Values {
		if path == allowed || strings.HasPrefix(path, strings.TrimRight(allowed, "/")+"/") {
			return true
		}
	}
	return false
}

func pathDenied(path string, denied []string) bool {
	for _, root := range denied {
		if pathContains(root, path) {
			return true
		}
	}
	return false
}

func argvAllowed(argv []string, constraint ArgvSetConstraint) bool {
	return constraint.Specified && containsArgv(constraint.Values, argv)
}

func containsArgv(values [][]string, target []string) bool {
	targetKey := strings.Join(target, "\x00")
	for _, value := range values {
		if strings.Join(value, "\x00") == targetKey {
			return true
		}
	}
	return false
}

func containsIntentKind(values []union.IntentKind, target union.IntentKind) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsSemanticFidelity(values []union.SemanticFidelity, target union.SemanticFidelity) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intersectsStrings(left, right []string) bool {
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := set[value]; exists {
			return true
		}
	}
	return false
}

func fidelityRank(value union.SemanticFidelity) int {
	switch value {
	case union.SemanticFidelityExact:
		return 3
	case union.SemanticFidelityTransformed:
		return 2
	case union.SemanticFidelityDegraded:
		return 1
	default:
		return 0
	}
}
