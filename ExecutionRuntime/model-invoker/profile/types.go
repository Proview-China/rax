package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const CandidateVersion = "praxis.model-invoker.profile/v1candidate"

type ProfileID string
type ProfileVersion string

type ExecutionSurface string

const (
	ExecutionSurfaceDirectAPI   ExecutionSurface = "direct_api"
	ExecutionSurfaceAppServer   ExecutionSurface = "app_server"
	ExecutionSurfaceAgentSDK    ExecutionSurface = "agent_sdk"
	ExecutionSurfaceOfficialCLI ExecutionSurface = "official_cli"
	ExecutionSurfaceOfficialSDK ExecutionSurface = "official_sdk"
)

type ContextMode string

const (
	ContextSemanticStable ContextMode = "semantic_stable"
	ContextVendorDefault  ContextMode = "vendor_default"
	ContextCustomExplicit ContextMode = "custom_explicit"
)

type HarnessComponent struct {
	Component            string `json:"component"`
	Version              string `json:"version"`
	ExecutablePath       string `json:"executable_path,omitempty"`
	BinaryDigest         string `json:"binary_digest"`
	ProtocolSchemaDigest string `json:"protocol_schema_digest"`
}

func (component HarnessComponent) Validate() error {
	if !validStableName(component.Component) {
		return fmt.Errorf("component must be a stable name")
	}
	if !validVersion(component.Version) {
		return fmt.Errorf("component %q has an invalid version", component.Component)
	}
	if component.ExecutablePath != "" && !filepath.IsAbs(component.ExecutablePath) {
		return fmt.Errorf("component %q executable path must be absolute", component.Component)
	}
	if !validDigest(component.BinaryDigest) {
		return fmt.Errorf("component %q binary digest must be sha256", component.Component)
	}
	if !validDigest(component.ProtocolSchemaDigest) {
		return fmt.Errorf("component %q protocol schema digest must be sha256", component.Component)
	}
	return nil
}

type ProfileSelectionKey struct {
	BaseRouteID           upstream.RouteID      `json:"base_route_id"`
	Provider              upstream.ProviderID   `json:"provider"`
	ModelID               string                `json:"model_id"`
	ModelRevision         string                `json:"model_revision"`
	Deployment            upstream.DeploymentID `json:"deployment"`
	Region                string                `json:"region"`
	EndpointIdentity      upstream.EndpointID   `json:"endpoint_identity"`
	Protocol              upstream.ProtocolID   `json:"protocol"`
	ProtocolSchemaVersion string                `json:"protocol_schema_version"`
	Offering              upstream.OfferingID   `json:"offering"`
	AuthRoute             string                `json:"auth_route"`
	ExecutionSurface      ExecutionSurface      `json:"execution_surface"`
	HarnessStack          []HarnessComponent    `json:"harness_stack"`
}

func (key ProfileSelectionKey) Validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"base_route_id", string(key.BaseRouteID)},
		{"provider", string(key.Provider)},
		{"model_id", key.ModelID},
		{"model_revision", key.ModelRevision},
		{"deployment", string(key.Deployment)},
		{"region", key.Region},
		{"endpoint_identity", string(key.EndpointIdentity)},
		{"protocol", string(key.Protocol)},
		{"protocol_schema_version", key.ProtocolSchemaVersion},
		{"offering", string(key.Offering)},
		{"auth_route", key.AuthRoute},
	}
	for _, field := range required {
		if !validStableReference(field.value) {
			return fmt.Errorf("%s must be a stable non-empty reference", field.name)
		}
	}
	switch key.ExecutionSurface {
	case ExecutionSurfaceDirectAPI:
		if len(key.HarnessStack) != 0 {
			return fmt.Errorf("direct_api selection must not declare a harness stack")
		}
	case ExecutionSurfaceAppServer, ExecutionSurfaceAgentSDK, ExecutionSurfaceOfficialCLI, ExecutionSurfaceOfficialSDK:
		if len(key.HarnessStack) == 0 {
			return fmt.Errorf("%s selection requires a harness stack", key.ExecutionSurface)
		}
	default:
		return fmt.Errorf("unknown execution surface %q", key.ExecutionSurface)
	}
	seen := make(map[string]struct{}, len(key.HarnessStack))
	for index, component := range key.HarnessStack {
		if err := component.Validate(); err != nil {
			return fmt.Errorf("harness stack %d: %w", index, err)
		}
		if _, duplicate := seen[component.Component]; duplicate {
			return fmt.Errorf("harness stack duplicates component %q", component.Component)
		}
		seen[component.Component] = struct{}{}
	}
	return nil
}

func (key ProfileSelectionKey) Clone() ProfileSelectionKey {
	clone := key
	clone.HarnessStack = append([]HarnessComponent(nil), key.HarnessStack...)
	return clone
}

func (key ProfileSelectionKey) Digest() (string, error) {
	if err := key.Validate(); err != nil {
		return "", err
	}
	return digestJSON(key)
}

func (key ProfileSelectionKey) HarnessStackDigest() (string, error) {
	for index, component := range key.HarnessStack {
		if err := component.Validate(); err != nil {
			return "", fmt.Errorf("harness stack %d: %w", index, err)
		}
	}
	return digestJSON(key.HarnessStack)
}

type SelectionConstraints struct {
	BaseRouteID           upstream.RouteID
	Provider              upstream.ProviderID
	ModelID               string
	ModelRevision         string
	Deployment            upstream.DeploymentID
	Region                string
	EndpointIdentity      upstream.EndpointID
	Protocol              upstream.ProtocolID
	ProtocolSchemaVersion string
	Offering              upstream.OfferingID
	AuthRoute             string
	ExecutionSurface      ExecutionSurface
	HarnessStack          []HarnessComponent
	HarnessStackSpecified bool
	HarnessStackDigest    string
}

func (constraints SelectionConstraints) matches(key ProfileSelectionKey) bool {
	if constraints.BaseRouteID != "" && constraints.BaseRouteID != key.BaseRouteID {
		return false
	}
	if constraints.Provider != "" && constraints.Provider != key.Provider {
		return false
	}
	if constraints.ModelID != "" && constraints.ModelID != key.ModelID {
		return false
	}
	if constraints.ModelRevision != "" && constraints.ModelRevision != key.ModelRevision {
		return false
	}
	if constraints.Deployment != "" && constraints.Deployment != key.Deployment {
		return false
	}
	if constraints.Region != "" && constraints.Region != key.Region {
		return false
	}
	if constraints.EndpointIdentity != "" && constraints.EndpointIdentity != key.EndpointIdentity {
		return false
	}
	if constraints.Protocol != "" && constraints.Protocol != key.Protocol {
		return false
	}
	if constraints.ProtocolSchemaVersion != "" && constraints.ProtocolSchemaVersion != key.ProtocolSchemaVersion {
		return false
	}
	if constraints.Offering != "" && constraints.Offering != key.Offering {
		return false
	}
	if constraints.AuthRoute != "" && constraints.AuthRoute != key.AuthRoute {
		return false
	}
	if constraints.ExecutionSurface != "" && constraints.ExecutionSurface != key.ExecutionSurface {
		return false
	}
	if constraints.HarnessStackSpecified && !equalHarnessStack(constraints.HarnessStack, key.HarnessStack) {
		return false
	}
	if constraints.HarnessStackDigest != "" {
		digest, err := key.HarnessStackDigest()
		if err != nil || digest != constraints.HarnessStackDigest {
			return false
		}
	}
	return true
}

type Attribution string

const (
	AttributionModelIntrinsicClaimed Attribution = "model_intrinsic_claimed"
	AttributionOfficialHarness       Attribution = "official_harness_induced"
	AttributionToolSchema            Attribution = "tool_schema_induced"
	AttributionProviderProtocol      Attribution = "provider_protocol_induced"
	AttributionRuntimePolicy         Attribution = "runtime_policy_induced"
	AttributionUnknown               Attribution = "unknown"
)

type BehaviorEvidence struct {
	ID          string      `json:"id"`
	Reference   string      `json:"reference"`
	Attribution Attribution `json:"attribution"`
	CheckedAt   time.Time   `json:"checked_at"`
	ValidUntil  time.Time   `json:"valid_until"`
	Digest      string      `json:"digest"`
}

type MechanismPreference struct {
	IntentKind  union.IntentKind `json:"intent_kind"`
	MechanismID string           `json:"mechanism_id"`
	Rank        int              `json:"rank"`
	EvidenceID  string           `json:"evidence_id"`
}

type MechanismScore struct {
	ModelAffinity        int `json:"model_affinity"`
	SemanticFidelity     int `json:"semantic_fidelity"`
	EffectObservability  int `json:"effect_observability"`
	VerificationStrength int `json:"verification_strength"`
	Determinism          int `json:"determinism"`
	Efficiency           int `json:"efficiency"`
	TransformationCost   int `json:"transformation_cost"`
	OpaqueHarnessDelta   int `json:"opaque_harness_delta"`
	OperationalRisk      int `json:"operational_risk"`
	FallbackRisk         int `json:"fallback_risk"`
}

func (score MechanismScore) Total() int {
	return score.ModelAffinity + score.SemanticFidelity + score.EffectObservability +
		score.VerificationStrength + score.Determinism + score.Efficiency -
		score.TransformationCost - score.OpaqueHarnessDelta - score.OperationalRisk - score.FallbackRisk
}

type MechanismCapability struct {
	ID                 string                   `json:"id"`
	Kind               string                   `json:"kind"`
	IntentKinds        []union.IntentKind       `json:"intent_kinds"`
	Origin             union.CapabilityOrigin   `json:"origin"`
	Owner              union.ExecutionOwner     `json:"owner"`
	SelectionAuthority union.SelectionAuthority `json:"selection_authority"`
	Fidelity           union.SemanticFidelity   `json:"fidelity"`
	ModelAddressable   bool                     `json:"model_addressable"`
	EffectObservable   bool                     `json:"effect_observable"`
	VerifierAvailable  bool                     `json:"verifier_available"`
	ApprovalSupported  bool                     `json:"approval_supported"`
	Cancellable        bool                     `json:"cancellable"`
	Idempotent         bool                     `json:"idempotent"`
	FallbackSafe       bool                     `json:"fallback_safe"`
	Capabilities       []string                 `json:"capabilities"`
	HardConstraints    []string                 `json:"hard_constraints"`
	ExpectedEffects    []string                 `json:"expected_effects"`
	Score              MechanismScore           `json:"score"`
}

type ModelBehaviorProfile struct {
	ID                   ProfileID             `json:"id"`
	Version              ProfileVersion        `json:"version"`
	ModelFamily          string                `json:"model_family"`
	ExactModelIDs        []string              `json:"exact_model_ids"`
	Evidence             []BehaviorEvidence    `json:"evidence"`
	MechanismPreferences []MechanismPreference `json:"mechanism_preferences"`
	KnownFailurePatterns []string              `json:"known_failure_patterns"`
	BenchmarkDigest      string                `json:"benchmark_digest"`
}

type ControlCapabilities struct {
	Approval          bool `json:"approval"`
	Cancel            bool `json:"cancel"`
	Steer             bool `json:"steer"`
	ProvideToolResult bool `json:"provide_tool_result"`
	SessionResume     bool `json:"session_resume"`
}

type FeatureConflict struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

type HarnessCapabilityProfile struct {
	ID                         ProfileID             `json:"id"`
	Version                    ProfileVersion        `json:"version"`
	ExecutionSurface           ExecutionSurface      `json:"execution_surface"`
	HarnessName                string                `json:"harness_name"`
	HarnessStack               []HarnessComponent    `json:"harness_stack"`
	ContextMode                ContextMode           `json:"context_mode"`
	Transparency               string                `json:"transparency"`
	InstructionControl         string                `json:"instruction_control"`
	ContextControl             string                `json:"context_control"`
	EventFidelity              string                `json:"event_fidelity"`
	ExpectedManifest           InjectionManifest     `json:"expected_manifest"`
	AvailableMechanisms        []MechanismCapability `json:"available_mechanisms"`
	HostedCapabilities         []string              `json:"hosted_capabilities"`
	SupportedNativeFeatures    []string              `json:"supported_native_features"`
	ForbiddenNativeFeatures    []string              `json:"forbidden_native_features"`
	FeatureConflicts           []FeatureConflict     `json:"feature_conflicts"`
	OpaqueFields               []string              `json:"opaque_fields"`
	Controls                   ControlCapabilities   `json:"controls"`
	ProbeDigest                string                `json:"probe_digest"`
	SanitizedEnvironmentDigest string                `json:"sanitized_environment_digest"`
}

type SemanticRouteProfile struct {
	ID                   ProfileID                `json:"id"`
	Version              ProfileVersion           `json:"version"`
	Selection            ProfileSelectionKey      `json:"selection"`
	ModelBehavior        ModelBehaviorProfile     `json:"model_behavior"`
	HarnessCapability    HarnessCapabilityProfile `json:"harness_capability"`
	DefaultPolicy        RuntimePolicy            `json:"default_policy"`
	SemanticCodecVersion string                   `json:"semantic_codec_version"`
	ContextMode          ContextMode              `json:"context_mode"`
}

func (profile SemanticRouteProfile) Clone() SemanticRouteProfile {
	clone := profile
	clone.Selection = profile.Selection.Clone()
	clone.ModelBehavior.ExactModelIDs = append([]string(nil), profile.ModelBehavior.ExactModelIDs...)
	clone.ModelBehavior.Evidence = append([]BehaviorEvidence(nil), profile.ModelBehavior.Evidence...)
	clone.ModelBehavior.MechanismPreferences = append([]MechanismPreference(nil), profile.ModelBehavior.MechanismPreferences...)
	clone.ModelBehavior.KnownFailurePatterns = append([]string(nil), profile.ModelBehavior.KnownFailurePatterns...)
	clone.HarnessCapability.HarnessStack = append([]HarnessComponent(nil), profile.HarnessCapability.HarnessStack...)
	clone.HarnessCapability.ExpectedManifest = profile.HarnessCapability.ExpectedManifest.Clone()
	clone.HarnessCapability.AvailableMechanisms = make([]MechanismCapability, len(profile.HarnessCapability.AvailableMechanisms))
	for index, mechanism := range profile.HarnessCapability.AvailableMechanisms {
		clone.HarnessCapability.AvailableMechanisms[index] = mechanism
		clone.HarnessCapability.AvailableMechanisms[index].IntentKinds = append([]union.IntentKind(nil), mechanism.IntentKinds...)
		clone.HarnessCapability.AvailableMechanisms[index].Capabilities = append([]string(nil), mechanism.Capabilities...)
		clone.HarnessCapability.AvailableMechanisms[index].HardConstraints = append([]string(nil), mechanism.HardConstraints...)
		clone.HarnessCapability.AvailableMechanisms[index].ExpectedEffects = append([]string(nil), mechanism.ExpectedEffects...)
	}
	clone.HarnessCapability.HostedCapabilities = append([]string(nil), profile.HarnessCapability.HostedCapabilities...)
	clone.HarnessCapability.SupportedNativeFeatures = append([]string(nil), profile.HarnessCapability.SupportedNativeFeatures...)
	clone.HarnessCapability.ForbiddenNativeFeatures = append([]string(nil), profile.HarnessCapability.ForbiddenNativeFeatures...)
	clone.HarnessCapability.FeatureConflicts = append([]FeatureConflict(nil), profile.HarnessCapability.FeatureConflicts...)
	clone.HarnessCapability.OpaqueFields = append([]string(nil), profile.HarnessCapability.OpaqueFields...)
	clone.DefaultPolicy = profile.DefaultPolicy.Clone()
	return clone
}

func (profile SemanticRouteProfile) Validate(now time.Time) error {
	if !validStableName(string(profile.ID)) || !validVersion(string(profile.Version)) {
		return fmt.Errorf("profile ID and version are required")
	}
	if err := profile.Selection.Validate(); err != nil {
		return fmt.Errorf("selection: %w", err)
	}
	if profile.ModelBehavior.ID == "" || profile.ModelBehavior.Version == "" {
		return fmt.Errorf("model behavior profile identity is required")
	}
	if err := profile.ModelBehavior.Validate(now); err != nil {
		return err
	}
	if !containsString(profile.ModelBehavior.ExactModelIDs, profile.Selection.ModelID) {
		return fmt.Errorf("selected model is not exact in model behavior profile")
	}
	if profile.HarnessCapability.ExecutionSurface != profile.Selection.ExecutionSurface ||
		!equalHarnessStack(profile.HarnessCapability.HarnessStack, profile.Selection.HarnessStack) {
		return fmt.Errorf("harness capability identity does not match selection")
	}
	if err := profile.HarnessCapability.Validate(); err != nil {
		return err
	}
	if profile.ContextMode != profile.HarnessCapability.ContextMode {
		return fmt.Errorf("profile and harness context modes differ")
	}
	if profile.ContextMode != ContextSemanticStable && profile.ContextMode != ContextVendorDefault && profile.ContextMode != ContextCustomExplicit {
		return fmt.Errorf("unknown context mode %q", profile.ContextMode)
	}
	if !validVersion(profile.SemanticCodecVersion) {
		return fmt.Errorf("semantic codec version is required")
	}
	if err := profile.DefaultPolicy.Validate(); err != nil {
		return fmt.Errorf("default policy: %w", err)
	}
	if profile.DefaultPolicy.Identity.BaseRouteID != profile.Selection.BaseRouteID ||
		profile.DefaultPolicy.Identity.Provider != profile.Selection.Provider ||
		profile.DefaultPolicy.Identity.Offering != profile.Selection.Offering ||
		profile.DefaultPolicy.Identity.ModelID != profile.Selection.ModelID {
		return fmt.Errorf("default policy identity is not bound to the exact selection key")
	}
	return nil
}

func (profile SemanticRouteProfile) Digest(now time.Time) (string, error) {
	if err := profile.Validate(now); err != nil {
		return "", err
	}
	clone := profile.Clone()
	normalizeSemanticRouteProfile(&clone)
	return digestJSON(clone)
}

func normalizeSemanticRouteProfile(profile *SemanticRouteProfile) {
	if profile == nil {
		return
	}
	sort.Strings(profile.ModelBehavior.ExactModelIDs)
	sort.Slice(profile.ModelBehavior.Evidence, func(i, j int) bool {
		return profile.ModelBehavior.Evidence[i].ID < profile.ModelBehavior.Evidence[j].ID
	})
	sort.Slice(profile.ModelBehavior.MechanismPreferences, func(i, j int) bool {
		left, right := profile.ModelBehavior.MechanismPreferences[i], profile.ModelBehavior.MechanismPreferences[j]
		if left.IntentKind != right.IntentKind {
			return left.IntentKind < right.IntentKind
		}
		if left.Rank != right.Rank {
			return left.Rank < right.Rank
		}
		return left.MechanismID < right.MechanismID
	})
	sort.Strings(profile.ModelBehavior.KnownFailurePatterns)
	sort.Strings(profile.HarnessCapability.OpaqueFields)
	sort.Strings(profile.HarnessCapability.HostedCapabilities)
	sort.Strings(profile.HarnessCapability.SupportedNativeFeatures)
	sort.Strings(profile.HarnessCapability.ForbiddenNativeFeatures)
	sort.Slice(profile.HarnessCapability.FeatureConflicts, func(i, j int) bool {
		left, right := profile.HarnessCapability.FeatureConflicts[i], profile.HarnessCapability.FeatureConflicts[j]
		if left.Left != right.Left {
			return left.Left < right.Left
		}
		return left.Right < right.Right
	})
	sort.Slice(profile.HarnessCapability.AvailableMechanisms, func(i, j int) bool {
		return profile.HarnessCapability.AvailableMechanisms[i].ID < profile.HarnessCapability.AvailableMechanisms[j].ID
	})
	for index := range profile.HarnessCapability.AvailableMechanisms {
		mechanism := &profile.HarnessCapability.AvailableMechanisms[index]
		sort.Slice(mechanism.IntentKinds, func(i, j int) bool { return mechanism.IntentKinds[i] < mechanism.IntentKinds[j] })
		sort.Strings(mechanism.Capabilities)
		sort.Strings(mechanism.HardConstraints)
		sort.Strings(mechanism.ExpectedEffects)
	}
	profile.HarnessCapability.ExpectedManifest.normalize()
	profile.DefaultPolicy.normalize()
}

func (profile ModelBehaviorProfile) Validate(now time.Time) error {
	if !validStableName(string(profile.ID)) || !validVersion(string(profile.Version)) || !validStableName(profile.ModelFamily) {
		return fmt.Errorf("model behavior profile identity is invalid")
	}
	if len(profile.ExactModelIDs) == 0 {
		return fmt.Errorf("model behavior profile requires exact model IDs")
	}
	if !validDigest(profile.BenchmarkDigest) {
		return fmt.Errorf("model behavior benchmark digest is invalid")
	}
	if err := uniqueReferences(profile.ExactModelIDs, "exact model IDs"); err != nil {
		return err
	}
	evidenceIDs := make(map[string]struct{}, len(profile.Evidence))
	for _, evidence := range profile.Evidence {
		if !validStableName(evidence.ID) || !validReferenceValue(evidence.Reference) || !validDigest(evidence.Digest) ||
			evidence.CheckedAt.IsZero() || !evidence.ValidUntil.After(now) || evidence.ValidUntil.Before(evidence.CheckedAt) {
			return fmt.Errorf("model behavior evidence %q is invalid or stale", evidence.ID)
		}
		switch evidence.Attribution {
		case AttributionModelIntrinsicClaimed, AttributionOfficialHarness, AttributionToolSchema,
			AttributionProviderProtocol, AttributionRuntimePolicy, AttributionUnknown:
		default:
			return fmt.Errorf("model behavior evidence %q has invalid attribution", evidence.ID)
		}
		if _, duplicate := evidenceIDs[evidence.ID]; duplicate {
			return fmt.Errorf("model behavior evidence %q is duplicated", evidence.ID)
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}
	for _, preference := range profile.MechanismPreferences {
		if !knownIntentKind(preference.IntentKind) || !validStableName(preference.MechanismID) || preference.Rank < 0 {
			return fmt.Errorf("model behavior mechanism preference is invalid")
		}
		if _, exists := evidenceIDs[preference.EvidenceID]; !exists {
			return fmt.Errorf("model behavior mechanism preference has no evidence")
		}
	}
	return nil
}

func (profile HarnessCapabilityProfile) Validate() error {
	if !validStableName(string(profile.ID)) || !validVersion(string(profile.Version)) ||
		!validStableName(profile.HarnessName) || profile.ExecutionSurface == "" {
		return fmt.Errorf("harness capability profile identity is invalid")
	}
	if !validDigest(profile.ProbeDigest) || !validDigest(profile.SanitizedEnvironmentDigest) {
		return fmt.Errorf("harness capability profile digests are invalid")
	}
	if err := profile.ExpectedManifest.Validate(false); err != nil {
		return fmt.Errorf("harness expected manifest: %w", err)
	}
	if err := uniqueReferences(profile.SupportedNativeFeatures, "supported native features"); err != nil {
		return err
	}
	if err := uniqueReferences(profile.ForbiddenNativeFeatures, "forbidden native features"); err != nil {
		return err
	}
	for _, conflict := range profile.FeatureConflicts {
		if !validStableName(conflict.Left) || !validStableName(conflict.Right) || conflict.Left == conflict.Right {
			return fmt.Errorf("harness feature conflict is invalid")
		}
	}
	seen := make(map[string]struct{}, len(profile.AvailableMechanisms))
	for _, mechanism := range profile.AvailableMechanisms {
		if err := mechanism.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[mechanism.ID]; duplicate {
			return fmt.Errorf("harness mechanism %q is duplicated", mechanism.ID)
		}
		seen[mechanism.ID] = struct{}{}
	}
	return uniqueReferences(profile.HostedCapabilities, "hosted capabilities")
}

func (mechanism MechanismCapability) Validate() error {
	if !validStableName(mechanism.ID) || !validStableName(mechanism.Kind) || len(mechanism.IntentKinds) == 0 {
		return fmt.Errorf("mechanism capability identity is invalid")
	}
	for _, intentKind := range mechanism.IntentKinds {
		if !knownIntentKind(intentKind) {
			return fmt.Errorf("mechanism %q has unknown intent kind %q", mechanism.ID, intentKind)
		}
	}
	switch mechanism.Origin {
	case union.CapabilityOriginNative, union.CapabilityOriginProviderHosted, union.CapabilityOriginHarnessHosted,
		union.CapabilityOriginCallerHosted, union.CapabilityOriginEmulated, union.CapabilityOriginUnavailable:
	default:
		return fmt.Errorf("mechanism %q has invalid origin", mechanism.ID)
	}
	switch mechanism.Owner {
	case union.ExecutionOwnerModel, union.ExecutionOwnerProvider, union.ExecutionOwnerHarness,
		union.ExecutionOwnerPraxis, union.ExecutionOwnerExternal:
	default:
		return fmt.Errorf("mechanism %q has invalid owner", mechanism.ID)
	}
	switch mechanism.SelectionAuthority {
	case union.SelectionAuthorityRuntime, union.SelectionAuthorityModelWithinSet,
		union.SelectionAuthorityHarness, union.SelectionAuthorityProvider:
	default:
		return fmt.Errorf("mechanism %q has invalid selection authority", mechanism.ID)
	}
	switch mechanism.Fidelity {
	case union.SemanticFidelityExact, union.SemanticFidelityTransformed,
		union.SemanticFidelityDegraded, union.SemanticFidelityUnavailable:
	default:
		return fmt.Errorf("mechanism %q has invalid fidelity", mechanism.ID)
	}
	if err := uniqueReferences(mechanism.Capabilities, "mechanism capabilities"); err != nil {
		return err
	}
	for name, value := range map[string]int{
		"model affinity": mechanism.Score.ModelAffinity, "semantic fidelity": mechanism.Score.SemanticFidelity,
		"effect observability": mechanism.Score.EffectObservability, "verification strength": mechanism.Score.VerificationStrength,
		"determinism": mechanism.Score.Determinism, "efficiency": mechanism.Score.Efficiency,
		"transformation cost": mechanism.Score.TransformationCost, "opaque harness delta": mechanism.Score.OpaqueHarnessDelta,
		"operational risk": mechanism.Score.OperationalRisk, "fallback risk": mechanism.Score.FallbackRisk,
	} {
		if value < 0 || value > 100 {
			return fmt.Errorf("mechanism %q %s score must be between 0 and 100", mechanism.ID, name)
		}
	}
	return nil
}

func uniqueReferences(values []string, name string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validReferenceValue(value) {
			return fmt.Errorf("%s contains invalid value %q", name, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func equalHarnessStack(left, right []HarnessComponent) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

var stableNamePattern = regexp.MustCompile("^[a-z0-9][a-z0-9._/-]*$")
var digestPattern = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

func validStableName(value string) bool {
	return value != "" && len(value) <= 256 && stableNamePattern.MatchString(value)
}

func validStableReference(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 512 && !strings.ContainsAny(value, "\x00\r\n|")
}

func validVersion(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
}

func validDigest(value string) bool { return digestPattern.MatchString(value) }

func DigestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func digestJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return DigestString(string(encoded)), nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
