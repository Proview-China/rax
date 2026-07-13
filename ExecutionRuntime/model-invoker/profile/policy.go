package profile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type VerificationStrength string

const (
	VerificationNone     VerificationStrength = "none"
	VerificationObserved VerificationStrength = "observed"
	VerificationStrong   VerificationStrength = "verified"
)

type StringSetConstraint struct {
	Specified bool     `json:"specified"`
	Values    []string `json:"values"`
}

type IntentSetConstraint struct {
	Specified bool               `json:"specified"`
	Values    []union.IntentKind `json:"values"`
}

type PathSetConstraint struct {
	Specified bool     `json:"specified"`
	Values    []string `json:"values"`
}

type ArgvSetConstraint struct {
	Specified bool       `json:"specified"`
	Values    [][]string `json:"values"`
}

type IdentityLock struct {
	BaseRouteID upstream.RouteID    `json:"base_route_id"`
	Provider    upstream.ProviderID `json:"provider"`
	Offering    upstream.OfferingID `json:"offering"`
	ModelID     string              `json:"model_id"`
}

type NetworkMode string

const (
	NetworkDenied       NetworkMode = "denied"
	NetworkAllowlist    NetworkMode = "allowlist"
	NetworkUnrestricted NetworkMode = "unrestricted"
)

type FilesystemPolicy struct {
	ReadableRoots      PathSetConstraint `json:"readable_roots"`
	WritablePaths      PathSetConstraint `json:"writable_paths"`
	DeniedPaths        []string          `json:"denied_paths"`
	FollowSymlinks     bool              `json:"follow_symlinks"`
	MaxFileSize        int64             `json:"max_file_size"`
	MaxTotalDelta      int64             `json:"max_total_delta"`
	AllowDelete        bool              `json:"allow_delete"`
	AllowMove          bool              `json:"allow_move"`
	RequireAtomicWrite bool              `json:"require_atomic_write"`
	RequireBackup      bool              `json:"require_backup"`
}

type ProcessPolicy struct {
	AllowedArgv    ArgvSetConstraint `json:"allowed_argv"`
	DeniedArgv     [][]string        `json:"denied_argv"`
	AllowedCWDs    PathSetConstraint `json:"allowed_cwds"`
	NetworkAccess  NetworkMode       `json:"network_access"`
	MaxTimeout     time.Duration     `json:"max_timeout"`
	AllowShellMeta bool              `json:"allow_shell_meta"`
}

type NetworkPolicy struct {
	Mode         NetworkMode         `json:"mode"`
	AllowedHosts StringSetConstraint `json:"allowed_hosts"`
	DeniedHosts  []string            `json:"denied_hosts"`
	AllowDNS     bool                `json:"allow_dns"`
}

type ComputerPolicy struct {
	Enabled                      bool                `json:"enabled"`
	AllowedOrigins               StringSetConstraint `json:"allowed_origins"`
	DeniedOrigins                []string            `json:"denied_origins"`
	AllowIrreversibleActions     bool                `json:"allow_irreversible_actions"`
	RequireExternalStateEvidence bool                `json:"require_external_state_evidence"`
	MaxActions                   int                 `json:"max_actions"`
}

type ApprovalStrength string

const (
	ApprovalNone         ApprovalStrength = "none"
	ApprovalOnRisk       ApprovalStrength = "on_risk"
	ApprovalOnSideEffect ApprovalStrength = "on_side_effect"
	ApprovalAlways       ApprovalStrength = "always"
)

type ApprovalPolicy struct {
	Strength                        ApprovalStrength `json:"strength"`
	PreauthorizedExactActionIDs     []string         `json:"preauthorized_exact_action_ids"`
	Timeout                         time.Duration    `json:"timeout"`
	ChangedRevisionRequiresApproval bool             `json:"changed_revision_requires_approval"`
	UnknownDecisionIsDeny           bool             `json:"unknown_decision_is_deny"`
}

type SecretPolicy struct {
	AllowedReferenceStores StringSetConstraint `json:"allowed_reference_stores"`
	DeniedEnvironmentNames []string            `json:"denied_environment_names"`
	ForbidPlaintext        bool                `json:"forbid_plaintext"`
	RequireRedaction       bool                `json:"require_redaction"`
}

type RetryFallbackPolicy struct {
	MaxAttempts                    int                 `json:"max_attempts"`
	MaxFallbacks                   int                 `json:"max_fallbacks"`
	AllowedFallbackMechanismIDs    StringSetConstraint `json:"allowed_fallback_mechanism_ids"`
	RequireSideEffectStateNone     bool                `json:"require_side_effect_state_none"`
	RequireReconcileBeforeFallback bool                `json:"require_reconcile_before_fallback"`
	AllowModelSwitch               bool                `json:"allow_model_switch"`
	AllowRouteSwitch               bool                `json:"allow_route_switch"`
}

type RuntimePolicy struct {
	ID                        ProfileID            `json:"id"`
	Version                   ProfileVersion       `json:"version"`
	Identity                  IdentityLock         `json:"identity"`
	AllowedIntentKinds        IntentSetConstraint  `json:"allowed_intent_kinds"`
	DeniedIntentKinds         []union.IntentKind   `json:"denied_intent_kinds"`
	AllowedMechanismIDs       StringSetConstraint  `json:"allowed_mechanism_ids"`
	DeniedMechanismIDs        []string             `json:"denied_mechanism_ids"`
	DeniedCapabilities        []string             `json:"denied_capabilities"`
	AllowedP2ManifestPaths    StringSetConstraint  `json:"allowed_p2_manifest_paths"`
	MaxWallTime               time.Duration        `json:"max_wall_time"`
	MaxActions                int                  `json:"max_actions"`
	MaxFallbacks              int                  `json:"max_fallbacks"`
	MaxConcurrency            int                  `json:"max_concurrency"`
	Verification              VerificationStrength `json:"verification"`
	Filesystem                FilesystemPolicy     `json:"filesystem"`
	Process                   ProcessPolicy        `json:"process"`
	Network                   NetworkPolicy        `json:"network"`
	Computer                  ComputerPolicy       `json:"computer"`
	Approval                  ApprovalPolicy       `json:"approval"`
	Secret                    SecretPolicy         `json:"secret"`
	RetryFallback             RetryFallbackPolicy  `json:"retry_fallback"`
	AllowLegacyCompatibility  bool                 `json:"allow_legacy_compatibility"`
	MechanismPreferenceWeight map[string]int       `json:"mechanism_preference_weight"`
}

const maxMechanismPreferenceWeight = 1_000_000

func (policy RuntimePolicy) Clone() RuntimePolicy {
	clone := policy
	clone.AllowedIntentKinds.Values = append([]union.IntentKind(nil), policy.AllowedIntentKinds.Values...)
	clone.DeniedIntentKinds = append([]union.IntentKind(nil), policy.DeniedIntentKinds...)
	clone.AllowedMechanismIDs.Values = append([]string(nil), policy.AllowedMechanismIDs.Values...)
	clone.DeniedMechanismIDs = append([]string(nil), policy.DeniedMechanismIDs...)
	clone.DeniedCapabilities = append([]string(nil), policy.DeniedCapabilities...)
	clone.AllowedP2ManifestPaths.Values = append([]string(nil), policy.AllowedP2ManifestPaths.Values...)
	clone.Filesystem = policy.Filesystem.clone()
	clone.Process = policy.Process.clone()
	clone.Network = policy.Network.clone()
	clone.Computer = policy.Computer.clone()
	clone.Approval.PreauthorizedExactActionIDs = append([]string(nil), policy.Approval.PreauthorizedExactActionIDs...)
	clone.Secret.AllowedReferenceStores.Values = append([]string(nil), policy.Secret.AllowedReferenceStores.Values...)
	clone.Secret.DeniedEnvironmentNames = append([]string(nil), policy.Secret.DeniedEnvironmentNames...)
	clone.RetryFallback.AllowedFallbackMechanismIDs.Values = append([]string(nil), policy.RetryFallback.AllowedFallbackMechanismIDs.Values...)
	clone.MechanismPreferenceWeight = make(map[string]int, len(policy.MechanismPreferenceWeight))
	for mechanismID, weight := range policy.MechanismPreferenceWeight {
		clone.MechanismPreferenceWeight[mechanismID] = weight
	}
	return clone
}

func (policy *RuntimePolicy) normalize() {
	if policy == nil {
		return
	}
	sort.Slice(policy.AllowedIntentKinds.Values, func(i, j int) bool { return policy.AllowedIntentKinds.Values[i] < policy.AllowedIntentKinds.Values[j] })
	sort.Slice(policy.DeniedIntentKinds, func(i, j int) bool { return policy.DeniedIntentKinds[i] < policy.DeniedIntentKinds[j] })
	sort.Strings(policy.AllowedMechanismIDs.Values)
	sort.Strings(policy.DeniedMechanismIDs)
	sort.Strings(policy.DeniedCapabilities)
	sort.Strings(policy.AllowedP2ManifestPaths.Values)
	policy.Filesystem.normalize()
	policy.Process.normalize()
	policy.Network.normalize()
	policy.Computer.normalize()
	sort.Strings(policy.Approval.PreauthorizedExactActionIDs)
	sort.Strings(policy.Secret.AllowedReferenceStores.Values)
	sort.Strings(policy.Secret.DeniedEnvironmentNames)
	sort.Strings(policy.RetryFallback.AllowedFallbackMechanismIDs.Values)
}

func (policy RuntimePolicy) Validate() error {
	if !validStableName(string(policy.ID)) || !validVersion(string(policy.Version)) {
		return fmt.Errorf("policy ID and version are required")
	}
	if err := validateIntentSet(policy.AllowedIntentKinds, "allowed intent kinds"); err != nil {
		return err
	}
	if err := validateIntentValues(policy.DeniedIntentKinds, "denied intent kinds"); err != nil {
		return err
	}
	if err := validateStringSet(policy.AllowedMechanismIDs, "allowed mechanism IDs", validStableName); err != nil {
		return err
	}
	if err := validateStringValues(policy.DeniedMechanismIDs, "denied mechanism IDs", validStableName); err != nil {
		return err
	}
	if err := validateStringValues(policy.DeniedCapabilities, "denied capabilities", validStableName); err != nil {
		return err
	}
	if err := validateStringSet(policy.AllowedP2ManifestPaths, "allowed P2 manifest paths", validManifestPath); err != nil {
		return err
	}
	if policy.MaxWallTime < 0 || policy.MaxActions < 0 || policy.MaxFallbacks < 0 || policy.MaxConcurrency < 0 {
		return fmt.Errorf("policy limits must not be negative")
	}
	if verificationRank(policy.Verification) < 0 {
		return fmt.Errorf("unknown verification strength %q", policy.Verification)
	}
	for mechanismID, weight := range policy.MechanismPreferenceWeight {
		if !validStableName(mechanismID) {
			return fmt.Errorf("preference mechanism ID %q is invalid", mechanismID)
		}
		if weight < -maxMechanismPreferenceWeight || weight > maxMechanismPreferenceWeight {
			return fmt.Errorf("preference mechanism weight is outside the supported range")
		}
	}
	for _, check := range []struct {
		name string
		err  error
	}{
		{"filesystem", policy.Filesystem.Validate()},
		{"process", policy.Process.Validate()},
		{"network", policy.Network.Validate()},
		{"computer", policy.Computer.Validate()},
		{"approval", policy.Approval.Validate()},
		{"secret", policy.Secret.Validate()},
		{"retry/fallback", policy.RetryFallback.Validate()},
	} {
		if check.err != nil {
			return fmt.Errorf("%s policy: %w", check.name, check.err)
		}
	}
	if networkRank(policy.Process.NetworkAccess) > networkRank(policy.Network.Mode) {
		return fmt.Errorf("process network access cannot exceed global network policy")
	}
	return nil
}

func (policy RuntimePolicy) Digest() (string, error) {
	if err := policy.Validate(); err != nil {
		return "", err
	}
	clone := policy.Clone()
	clone.normalize()
	return digestJSON(clone)
}

func (policy FilesystemPolicy) clone() FilesystemPolicy {
	clone := policy
	clone.ReadableRoots.Values = append([]string(nil), policy.ReadableRoots.Values...)
	clone.WritablePaths.Values = append([]string(nil), policy.WritablePaths.Values...)
	clone.DeniedPaths = append([]string(nil), policy.DeniedPaths...)
	return clone
}

func (policy *FilesystemPolicy) normalize() {
	sort.Strings(policy.ReadableRoots.Values)
	sort.Strings(policy.WritablePaths.Values)
	sort.Strings(policy.DeniedPaths)
}

func (policy FilesystemPolicy) Validate() error {
	if err := validatePathSet(policy.ReadableRoots, "readable roots"); err != nil {
		return err
	}
	if err := validatePathSet(policy.WritablePaths, "writable paths"); err != nil {
		return err
	}
	if err := validatePaths(policy.DeniedPaths, "denied paths"); err != nil {
		return err
	}
	if policy.MaxFileSize < 0 || policy.MaxTotalDelta < 0 {
		return fmt.Errorf("filesystem size limits must not be negative")
	}
	return nil
}

func (policy ProcessPolicy) clone() ProcessPolicy {
	clone := policy
	clone.AllowedArgv.Values = cloneArgv(policy.AllowedArgv.Values)
	clone.DeniedArgv = cloneArgv(policy.DeniedArgv)
	clone.AllowedCWDs.Values = append([]string(nil), policy.AllowedCWDs.Values...)
	return clone
}

func (policy *ProcessPolicy) normalize() {
	sortArgv(policy.AllowedArgv.Values)
	sortArgv(policy.DeniedArgv)
	sort.Strings(policy.AllowedCWDs.Values)
}

func (policy ProcessPolicy) Validate() error {
	if err := validateArgvSet(policy.AllowedArgv, "allowed argv"); err != nil {
		return err
	}
	if err := validateArgvValues(policy.DeniedArgv, "denied argv"); err != nil {
		return err
	}
	if err := validatePathSet(policy.AllowedCWDs, "allowed working directories"); err != nil {
		return err
	}
	if networkRank(policy.NetworkAccess) < 0 {
		return fmt.Errorf("process network access is invalid")
	}
	if policy.MaxTimeout < 0 {
		return fmt.Errorf("process timeout must not be negative")
	}
	return nil
}

func (policy NetworkPolicy) clone() NetworkPolicy {
	clone := policy
	clone.AllowedHosts.Values = append([]string(nil), policy.AllowedHosts.Values...)
	clone.DeniedHosts = append([]string(nil), policy.DeniedHosts...)
	return clone
}

func (policy *NetworkPolicy) normalize() {
	sort.Strings(policy.AllowedHosts.Values)
	sort.Strings(policy.DeniedHosts)
}

func (policy NetworkPolicy) Validate() error {
	if networkRank(policy.Mode) < 0 {
		return fmt.Errorf("network mode is invalid")
	}
	if err := validateStringSet(policy.AllowedHosts, "allowed hosts", validReferenceValue); err != nil {
		return err
	}
	if err := validateStringValues(policy.DeniedHosts, "denied hosts", validReferenceValue); err != nil {
		return err
	}
	if policy.Mode == NetworkAllowlist && !policy.AllowedHosts.Specified {
		return fmt.Errorf("allowlist mode requires an explicit allowed-host set")
	}
	if policy.Mode == NetworkDenied && len(policy.AllowedHosts.Values) != 0 {
		return fmt.Errorf("denied network mode cannot contain allowed hosts")
	}
	return nil
}

func (policy ComputerPolicy) clone() ComputerPolicy {
	clone := policy
	clone.AllowedOrigins.Values = append([]string(nil), policy.AllowedOrigins.Values...)
	clone.DeniedOrigins = append([]string(nil), policy.DeniedOrigins...)
	return clone
}

func (policy *ComputerPolicy) normalize() {
	sort.Strings(policy.AllowedOrigins.Values)
	sort.Strings(policy.DeniedOrigins)
}

func (policy ComputerPolicy) Validate() error {
	if err := validateStringSet(policy.AllowedOrigins, "allowed computer origins", validReferenceValue); err != nil {
		return err
	}
	if err := validateStringValues(policy.DeniedOrigins, "denied computer origins", validReferenceValue); err != nil {
		return err
	}
	if policy.MaxActions < 0 {
		return fmt.Errorf("computer max actions must not be negative")
	}
	if !policy.Enabled && policy.AllowIrreversibleActions {
		return fmt.Errorf("disabled computer policy cannot allow irreversible actions")
	}
	return nil
}

func (policy ApprovalPolicy) Validate() error {
	if approvalRank(policy.Strength) < 0 {
		return fmt.Errorf("approval strength is invalid")
	}
	if err := validateStringValues(policy.PreauthorizedExactActionIDs, "preauthorized action IDs", validStableName); err != nil {
		return err
	}
	if policy.Timeout < 0 {
		return fmt.Errorf("approval timeout must not be negative")
	}
	return nil
}

func (policy SecretPolicy) Validate() error {
	if err := validateStringSet(policy.AllowedReferenceStores, "allowed secret reference stores", validStableName); err != nil {
		return err
	}
	return validateStringValues(policy.DeniedEnvironmentNames, "denied environment names", validReferenceValue)
}

func (policy RetryFallbackPolicy) Validate() error {
	if policy.MaxAttempts < 1 || policy.MaxFallbacks < 0 {
		return fmt.Errorf("retry attempts must be positive and fallbacks non-negative")
	}
	if err := validateStringSet(policy.AllowedFallbackMechanismIDs, "allowed fallback mechanism IDs", validStableName); err != nil {
		return err
	}
	if policy.AllowModelSwitch || policy.AllowRouteSwitch {
		return fmt.Errorf("model and Route switching are not mechanism fallbacks")
	}
	return nil
}

type PolicyScope string

const (
	PolicyScopeOrganization PolicyScope = "organization"
	PolicyScopeUser         PolicyScope = "user"
	PolicyScopeWorkspace    PolicyScope = "workspace"
	PolicyScopeTask         PolicyScope = "task"
)

type PermissionConstraint string

const (
	PermissionUnspecified PermissionConstraint = ""
	PermissionAllow       PermissionConstraint = "allow"
	PermissionDeny        PermissionConstraint = "deny"
)

type FilesystemPolicyLayer struct {
	ReadableRoots      PathSetConstraint    `json:"readable_roots"`
	WritablePaths      PathSetConstraint    `json:"writable_paths"`
	DeniedPaths        []string             `json:"denied_paths"`
	FollowSymlinks     PermissionConstraint `json:"follow_symlinks"`
	MaxFileSize        int64                `json:"max_file_size"`
	MaxTotalDelta      int64                `json:"max_total_delta"`
	Delete             PermissionConstraint `json:"delete"`
	Move               PermissionConstraint `json:"move"`
	RequireAtomicWrite bool                 `json:"require_atomic_write"`
	RequireBackup      bool                 `json:"require_backup"`
}

type ProcessPolicyLayer struct {
	AllowedArgv   ArgvSetConstraint    `json:"allowed_argv"`
	DeniedArgv    [][]string           `json:"denied_argv"`
	AllowedCWDs   PathSetConstraint    `json:"allowed_cwds"`
	NetworkAccess NetworkMode          `json:"network_access"`
	MaxTimeout    time.Duration        `json:"max_timeout"`
	ShellMeta     PermissionConstraint `json:"shell_meta"`
}

type NetworkPolicyLayer struct {
	Mode         NetworkMode          `json:"mode"`
	AllowedHosts StringSetConstraint  `json:"allowed_hosts"`
	DeniedHosts  []string             `json:"denied_hosts"`
	DNS          PermissionConstraint `json:"dns"`
}

type ComputerPolicyLayer struct {
	Enabled                      PermissionConstraint `json:"enabled"`
	AllowedOrigins               StringSetConstraint  `json:"allowed_origins"`
	DeniedOrigins                []string             `json:"denied_origins"`
	IrreversibleActions          PermissionConstraint `json:"irreversible_actions"`
	RequireExternalStateEvidence bool                 `json:"require_external_state_evidence"`
	MaxActions                   int                  `json:"max_actions"`
}

type ApprovalPolicyLayer struct {
	Strength                        ApprovalStrength    `json:"strength"`
	PreauthorizedExactActionIDs     StringSetConstraint `json:"preauthorized_exact_action_ids"`
	Timeout                         time.Duration       `json:"timeout"`
	ChangedRevisionRequiresApproval bool                `json:"changed_revision_requires_approval"`
	UnknownDecisionIsDeny           bool                `json:"unknown_decision_is_deny"`
}

type SecretPolicyLayer struct {
	AllowedReferenceStores StringSetConstraint `json:"allowed_reference_stores"`
	DeniedEnvironmentNames []string            `json:"denied_environment_names"`
	ForbidPlaintext        bool                `json:"forbid_plaintext"`
	RequireRedaction       bool                `json:"require_redaction"`
}

type RetryFallbackPolicyLayer struct {
	MaxAttempts                    int                  `json:"max_attempts"`
	MaxFallbacks                   int                  `json:"max_fallbacks"`
	AllowedFallbackMechanismIDs    StringSetConstraint  `json:"allowed_fallback_mechanism_ids"`
	RequireSideEffectStateNone     bool                 `json:"require_side_effect_state_none"`
	RequireReconcileBeforeFallback bool                 `json:"require_reconcile_before_fallback"`
	ModelSwitch                    PermissionConstraint `json:"model_switch"`
	RouteSwitch                    PermissionConstraint `json:"route_switch"`
}

type PolicyLayer struct {
	ID                        string                    `json:"id"`
	Scope                     PolicyScope               `json:"scope"`
	Identity                  IdentityLock              `json:"identity"`
	AllowedIntentKinds        IntentSetConstraint       `json:"allowed_intent_kinds"`
	DeniedIntentKinds         []union.IntentKind        `json:"denied_intent_kinds"`
	AllowedMechanismIDs       StringSetConstraint       `json:"allowed_mechanism_ids"`
	DeniedMechanismIDs        []string                  `json:"denied_mechanism_ids"`
	DeniedCapabilities        []string                  `json:"denied_capabilities"`
	AllowedP2ManifestPaths    StringSetConstraint       `json:"allowed_p2_manifest_paths"`
	MaxWallTime               time.Duration             `json:"max_wall_time"`
	MaxActions                int                       `json:"max_actions"`
	MaxFallbacks              int                       `json:"max_fallbacks"`
	MaxConcurrency            int                       `json:"max_concurrency"`
	Verification              VerificationStrength      `json:"verification"`
	Filesystem                *FilesystemPolicyLayer    `json:"filesystem,omitempty"`
	Process                   *ProcessPolicyLayer       `json:"process,omitempty"`
	Network                   *NetworkPolicyLayer       `json:"network,omitempty"`
	Computer                  *ComputerPolicyLayer      `json:"computer,omitempty"`
	Approval                  *ApprovalPolicyLayer      `json:"approval,omitempty"`
	Secret                    *SecretPolicyLayer        `json:"secret,omitempty"`
	RetryFallback             *RetryFallbackPolicyLayer `json:"retry_fallback,omitempty"`
	LegacyCompatibility       PermissionConstraint      `json:"legacy_compatibility"`
	MechanismPreferenceWeight map[string]int            `json:"mechanism_preference_weight"`
}

func MergeRuntimePolicy(base RuntimePolicy, layers ...PolicyLayer) (RuntimePolicy, error) {
	if err := base.Validate(); err != nil {
		return RuntimePolicy{}, fmt.Errorf("base policy: %w", err)
	}
	ordered := append([]PolicyLayer(nil), layers...)
	sort.SliceStable(ordered, func(i, j int) bool { return scopeRank(ordered[i].Scope) < scopeRank(ordered[j].Scope) })
	seenScopes := make(map[PolicyScope]struct{}, len(ordered))
	result := base.Clone()
	for _, layer := range ordered {
		if !validStableName(layer.ID) || scopeRank(layer.Scope) < 0 {
			return RuntimePolicy{}, fmt.Errorf("policy layer identity or scope is invalid")
		}
		if layer.MaxWallTime < 0 || layer.MaxActions < 0 || layer.MaxFallbacks < 0 || layer.MaxConcurrency < 0 {
			return RuntimePolicy{}, fmt.Errorf("policy layer %q has negative limits", layer.ID)
		}
		if _, duplicate := seenScopes[layer.Scope]; duplicate {
			return RuntimePolicy{}, fmt.Errorf("policy scope %q is duplicated", layer.Scope)
		}
		seenScopes[layer.Scope] = struct{}{}
		if err := checkIdentityLock(result.Identity, layer.Identity); err != nil {
			return RuntimePolicy{}, fmt.Errorf("policy layer %q: %w", layer.ID, err)
		}
		var err error
		result.AllowedIntentKinds, err = mergeIntentConstraint(result.AllowedIntentKinds, layer.AllowedIntentKinds, "allowed intent kinds")
		if err != nil {
			return RuntimePolicy{}, err
		}
		result.AllowedMechanismIDs, err = mergeStringConstraint(result.AllowedMechanismIDs, layer.AllowedMechanismIDs, "allowed mechanism IDs", validStableName)
		if err != nil {
			return RuntimePolicy{}, err
		}
		result.AllowedP2ManifestPaths, err = mergeStringConstraint(result.AllowedP2ManifestPaths, layer.AllowedP2ManifestPaths, "allowed P2 manifest paths", validManifestPath)
		if err != nil {
			return RuntimePolicy{}, err
		}
		result.DeniedIntentKinds = unionIntentKinds(result.DeniedIntentKinds, layer.DeniedIntentKinds)
		result.DeniedMechanismIDs = unionStrings(result.DeniedMechanismIDs, layer.DeniedMechanismIDs)
		result.DeniedCapabilities = unionStrings(result.DeniedCapabilities, layer.DeniedCapabilities)
		result.MaxWallTime = minPositiveDuration(result.MaxWallTime, layer.MaxWallTime)
		result.MaxActions = minPositive(result.MaxActions, layer.MaxActions)
		result.MaxFallbacks = minPositive(result.MaxFallbacks, layer.MaxFallbacks)
		result.MaxConcurrency = minPositive(result.MaxConcurrency, layer.MaxConcurrency)
		if verificationRank(layer.Verification) > verificationRank(result.Verification) {
			result.Verification = layer.Verification
		}
		if layer.Filesystem != nil {
			result.Filesystem, err = mergeFilesystemPolicy(result.Filesystem, *layer.Filesystem)
		}
		if err == nil && layer.Process != nil {
			result.Process, err = mergeProcessPolicy(result.Process, *layer.Process)
		}
		if err == nil && layer.Network != nil {
			result.Network, err = mergeNetworkPolicy(result.Network, *layer.Network)
		}
		if err == nil && layer.Computer != nil {
			result.Computer, err = mergeComputerPolicy(result.Computer, *layer.Computer)
		}
		if err == nil && layer.Approval != nil {
			result.Approval, err = mergeApprovalPolicy(result.Approval, *layer.Approval)
		}
		if err == nil && layer.Secret != nil {
			result.Secret, err = mergeSecretPolicy(result.Secret, *layer.Secret)
		}
		if err == nil && layer.RetryFallback != nil {
			result.RetryFallback, err = mergeRetryFallbackPolicy(result.RetryFallback, *layer.RetryFallback)
		}
		if err != nil {
			return RuntimePolicy{}, fmt.Errorf("policy layer %q: %w", layer.ID, err)
		}
		result.AllowLegacyCompatibility, err = constrainPermission(result.AllowLegacyCompatibility, layer.LegacyCompatibility)
		if err != nil {
			return RuntimePolicy{}, fmt.Errorf("policy layer %q legacy compatibility: %w", layer.ID, err)
		}
		if result.MechanismPreferenceWeight == nil {
			result.MechanismPreferenceWeight = make(map[string]int)
		}
		for mechanismID, weight := range layer.MechanismPreferenceWeight {
			if !validStableName(mechanismID) {
				return RuntimePolicy{}, fmt.Errorf("policy layer %q has invalid mechanism preference %q", layer.ID, mechanismID)
			}
			if weight < -maxMechanismPreferenceWeight || weight > maxMechanismPreferenceWeight {
				return RuntimePolicy{}, fmt.Errorf("policy layer %q mechanism preference weight is outside the supported range", layer.ID)
			}
			result.MechanismPreferenceWeight[mechanismID] = weight
		}
	}
	result.normalize()
	if err := result.Validate(); err != nil {
		return RuntimePolicy{}, err
	}
	return result, nil
}

func mergeFilesystemPolicy(base FilesystemPolicy, layer FilesystemPolicyLayer) (FilesystemPolicy, error) {
	if layer.MaxFileSize < 0 || layer.MaxTotalDelta < 0 {
		return FilesystemPolicy{}, fmt.Errorf("filesystem limits must not be negative")
	}
	var err error
	base.ReadableRoots, err = mergePathConstraint(base.ReadableRoots, layer.ReadableRoots, "readable roots")
	if err != nil {
		return FilesystemPolicy{}, err
	}
	base.WritablePaths, err = mergePathConstraint(base.WritablePaths, layer.WritablePaths, "writable paths")
	if err != nil {
		return FilesystemPolicy{}, err
	}
	if err := validatePaths(layer.DeniedPaths, "denied paths"); err != nil {
		return FilesystemPolicy{}, err
	}
	base.DeniedPaths = unionStrings(base.DeniedPaths, layer.DeniedPaths)
	base.FollowSymlinks, err = constrainPermission(base.FollowSymlinks, layer.FollowSymlinks)
	if err != nil {
		return FilesystemPolicy{}, err
	}
	base.AllowDelete, err = constrainPermission(base.AllowDelete, layer.Delete)
	if err != nil {
		return FilesystemPolicy{}, err
	}
	base.AllowMove, err = constrainPermission(base.AllowMove, layer.Move)
	if err != nil {
		return FilesystemPolicy{}, err
	}
	base.MaxFileSize = minPositive64(base.MaxFileSize, layer.MaxFileSize)
	base.MaxTotalDelta = minPositive64(base.MaxTotalDelta, layer.MaxTotalDelta)
	base.RequireAtomicWrite = base.RequireAtomicWrite || layer.RequireAtomicWrite
	base.RequireBackup = base.RequireBackup || layer.RequireBackup
	return base, nil
}

func mergeProcessPolicy(base ProcessPolicy, layer ProcessPolicyLayer) (ProcessPolicy, error) {
	if layer.MaxTimeout < 0 {
		return ProcessPolicy{}, fmt.Errorf("process timeout must not be negative")
	}
	var err error
	base.AllowedArgv, err = mergeArgvConstraint(base.AllowedArgv, layer.AllowedArgv, "allowed argv")
	if err != nil {
		return ProcessPolicy{}, err
	}
	if err := validateArgvValues(layer.DeniedArgv, "denied argv"); err != nil {
		return ProcessPolicy{}, err
	}
	base.DeniedArgv = unionArgv(base.DeniedArgv, layer.DeniedArgv)
	base.AllowedCWDs, err = mergePathConstraint(base.AllowedCWDs, layer.AllowedCWDs, "allowed working directories")
	if err != nil {
		return ProcessPolicy{}, err
	}
	if layer.NetworkAccess != "" {
		if networkRank(layer.NetworkAccess) < 0 {
			return ProcessPolicy{}, fmt.Errorf("invalid process network mode")
		}
		base.NetworkAccess = stricterNetworkMode(base.NetworkAccess, layer.NetworkAccess)
	}
	base.MaxTimeout = minPositiveDuration(base.MaxTimeout, layer.MaxTimeout)
	base.AllowShellMeta, err = constrainPermission(base.AllowShellMeta, layer.ShellMeta)
	if err != nil {
		return ProcessPolicy{}, err
	}
	return base, nil
}

func mergeNetworkPolicy(base NetworkPolicy, layer NetworkPolicyLayer) (NetworkPolicy, error) {
	var err error
	if layer.Mode != "" {
		if networkRank(layer.Mode) < 0 {
			return NetworkPolicy{}, fmt.Errorf("invalid network mode")
		}
		base.Mode = stricterNetworkMode(base.Mode, layer.Mode)
	}
	base.AllowedHosts, err = mergeStringConstraint(base.AllowedHosts, layer.AllowedHosts, "allowed hosts", validReferenceValue)
	if err != nil {
		return NetworkPolicy{}, err
	}
	if err := validateStringValues(layer.DeniedHosts, "denied hosts", validReferenceValue); err != nil {
		return NetworkPolicy{}, err
	}
	base.DeniedHosts = unionStrings(base.DeniedHosts, layer.DeniedHosts)
	base.AllowDNS, err = constrainPermission(base.AllowDNS, layer.DNS)
	if err != nil {
		return NetworkPolicy{}, err
	}
	if base.Mode == NetworkDenied {
		base.AllowedHosts = StringSetConstraint{Specified: true}
		base.AllowDNS = false
	}
	return base, nil
}

func mergeComputerPolicy(base ComputerPolicy, layer ComputerPolicyLayer) (ComputerPolicy, error) {
	if layer.MaxActions < 0 {
		return ComputerPolicy{}, fmt.Errorf("computer max actions must not be negative")
	}
	var err error
	base.Enabled, err = constrainPermission(base.Enabled, layer.Enabled)
	if err != nil {
		return ComputerPolicy{}, err
	}
	base.AllowedOrigins, err = mergeStringConstraint(base.AllowedOrigins, layer.AllowedOrigins, "allowed computer origins", validReferenceValue)
	if err != nil {
		return ComputerPolicy{}, err
	}
	if err := validateStringValues(layer.DeniedOrigins, "denied computer origins", validReferenceValue); err != nil {
		return ComputerPolicy{}, err
	}
	base.DeniedOrigins = unionStrings(base.DeniedOrigins, layer.DeniedOrigins)
	base.AllowIrreversibleActions, err = constrainPermission(base.AllowIrreversibleActions, layer.IrreversibleActions)
	if err != nil {
		return ComputerPolicy{}, err
	}
	base.RequireExternalStateEvidence = base.RequireExternalStateEvidence || layer.RequireExternalStateEvidence
	base.MaxActions = minPositive(base.MaxActions, layer.MaxActions)
	return base, nil
}

func mergeApprovalPolicy(base ApprovalPolicy, layer ApprovalPolicyLayer) (ApprovalPolicy, error) {
	if layer.Timeout < 0 {
		return ApprovalPolicy{}, fmt.Errorf("approval timeout must not be negative")
	}
	if layer.Strength != "" {
		if approvalRank(layer.Strength) < 0 {
			return ApprovalPolicy{}, fmt.Errorf("invalid approval strength")
		}
		if approvalRank(layer.Strength) > approvalRank(base.Strength) {
			base.Strength = layer.Strength
		}
	}
	if layer.PreauthorizedExactActionIDs.Specified {
		current := StringSetConstraint{Specified: base.PreauthorizedExactActionIDs != nil, Values: base.PreauthorizedExactActionIDs}
		merged, err := mergeStringConstraint(current, layer.PreauthorizedExactActionIDs, "preauthorized action IDs", validStableName)
		if err != nil {
			return ApprovalPolicy{}, err
		}
		base.PreauthorizedExactActionIDs = merged.Values
	}
	base.Timeout = minPositiveDuration(base.Timeout, layer.Timeout)
	base.ChangedRevisionRequiresApproval = base.ChangedRevisionRequiresApproval || layer.ChangedRevisionRequiresApproval
	base.UnknownDecisionIsDeny = base.UnknownDecisionIsDeny || layer.UnknownDecisionIsDeny
	return base, nil
}

func mergeSecretPolicy(base SecretPolicy, layer SecretPolicyLayer) (SecretPolicy, error) {
	var err error
	base.AllowedReferenceStores, err = mergeStringConstraint(base.AllowedReferenceStores, layer.AllowedReferenceStores, "allowed secret reference stores", validStableName)
	if err != nil {
		return SecretPolicy{}, err
	}
	if err := validateStringValues(layer.DeniedEnvironmentNames, "denied environment names", validReferenceValue); err != nil {
		return SecretPolicy{}, err
	}
	base.DeniedEnvironmentNames = unionStrings(base.DeniedEnvironmentNames, layer.DeniedEnvironmentNames)
	base.ForbidPlaintext = base.ForbidPlaintext || layer.ForbidPlaintext
	base.RequireRedaction = base.RequireRedaction || layer.RequireRedaction
	return base, nil
}

func mergeRetryFallbackPolicy(base RetryFallbackPolicy, layer RetryFallbackPolicyLayer) (RetryFallbackPolicy, error) {
	if layer.MaxAttempts < 0 || layer.MaxFallbacks < 0 {
		return RetryFallbackPolicy{}, fmt.Errorf("retry/fallback limits must not be negative")
	}
	var err error
	base.MaxAttempts = minPositive(base.MaxAttempts, layer.MaxAttempts)
	base.MaxFallbacks = minPositive(base.MaxFallbacks, layer.MaxFallbacks)
	base.AllowedFallbackMechanismIDs, err = mergeStringConstraint(base.AllowedFallbackMechanismIDs, layer.AllowedFallbackMechanismIDs, "allowed fallback mechanism IDs", validStableName)
	if err != nil {
		return RetryFallbackPolicy{}, err
	}
	base.RequireSideEffectStateNone = base.RequireSideEffectStateNone || layer.RequireSideEffectStateNone
	base.RequireReconcileBeforeFallback = base.RequireReconcileBeforeFallback || layer.RequireReconcileBeforeFallback
	base.AllowModelSwitch, err = constrainPermission(base.AllowModelSwitch, layer.ModelSwitch)
	if err != nil {
		return RetryFallbackPolicy{}, err
	}
	base.AllowRouteSwitch, err = constrainPermission(base.AllowRouteSwitch, layer.RouteSwitch)
	if err != nil {
		return RetryFallbackPolicy{}, err
	}
	return base, nil
}

func scopeRank(scope PolicyScope) int {
	switch scope {
	case PolicyScopeOrganization:
		return 0
	case PolicyScopeUser:
		return 1
	case PolicyScopeWorkspace:
		return 2
	case PolicyScopeTask:
		return 3
	default:
		return -1
	}
}

func checkIdentityLock(base, layer IdentityLock) error {
	checks := []struct {
		name, base, layer string
	}{
		{"base route", string(base.BaseRouteID), string(layer.BaseRouteID)},
		{"provider", string(base.Provider), string(layer.Provider)},
		{"offering", string(base.Offering), string(layer.Offering)},
		{"model", base.ModelID, layer.ModelID},
	}
	for _, check := range checks {
		if check.layer != "" && check.layer != check.base {
			return fmt.Errorf("%s identity cannot be overridden", check.name)
		}
	}
	return nil
}

func constrainPermission(current bool, constraint PermissionConstraint) (bool, error) {
	switch constraint {
	case PermissionUnspecified:
		return current, nil
	case PermissionDeny:
		return false, nil
	case PermissionAllow:
		if !current {
			return false, fmt.Errorf("a narrower scope cannot relax a denied permission")
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown permission constraint %q", constraint)
	}
}

func mergeStringConstraint(left, right StringSetConstraint, name string, validate func(string) bool) (StringSetConstraint, error) {
	if err := validateStringSet(right, name, validate); err != nil {
		return StringSetConstraint{}, err
	}
	if !right.Specified {
		return StringSetConstraint{Specified: left.Specified, Values: append([]string(nil), left.Values...)}, nil
	}
	if !left.Specified {
		return StringSetConstraint{Specified: true, Values: uniqueSorted(right.Values)}, nil
	}
	return StringSetConstraint{Specified: true, Values: intersectStrings(left.Values, right.Values)}, nil
}

func mergeIntentConstraint(left, right IntentSetConstraint, name string) (IntentSetConstraint, error) {
	if err := validateIntentSet(right, name); err != nil {
		return IntentSetConstraint{}, err
	}
	if !right.Specified {
		return IntentSetConstraint{Specified: left.Specified, Values: append([]union.IntentKind(nil), left.Values...)}, nil
	}
	if !left.Specified {
		return IntentSetConstraint{Specified: true, Values: uniqueIntentKinds(right.Values)}, nil
	}
	rightSet := make(map[union.IntentKind]struct{}, len(right.Values))
	for _, value := range right.Values {
		rightSet[value] = struct{}{}
	}
	var values []union.IntentKind
	for _, value := range left.Values {
		if _, exists := rightSet[value]; exists {
			values = append(values, value)
		}
	}
	return IntentSetConstraint{Specified: true, Values: uniqueIntentKinds(values)}, nil
}

func mergePathConstraint(left, right PathSetConstraint, name string) (PathSetConstraint, error) {
	if err := validatePathSet(right, name); err != nil {
		return PathSetConstraint{}, err
	}
	if !right.Specified {
		return PathSetConstraint{Specified: left.Specified, Values: append([]string(nil), left.Values...)}, nil
	}
	if !left.Specified {
		return PathSetConstraint{Specified: true, Values: uniqueSorted(right.Values)}, nil
	}
	return PathSetConstraint{Specified: true, Values: intersectPaths(left.Values, right.Values)}, nil
}

func mergeArgvConstraint(left, right ArgvSetConstraint, name string) (ArgvSetConstraint, error) {
	if err := validateArgvSet(right, name); err != nil {
		return ArgvSetConstraint{}, err
	}
	if !right.Specified {
		return ArgvSetConstraint{Specified: left.Specified, Values: cloneArgv(left.Values)}, nil
	}
	if !left.Specified {
		return ArgvSetConstraint{Specified: true, Values: uniqueArgv(right.Values)}, nil
	}
	rightKeys := make(map[string]struct{}, len(right.Values))
	for _, argv := range right.Values {
		rightKeys[argvKey(argv)] = struct{}{}
	}
	var values [][]string
	for _, argv := range left.Values {
		if _, exists := rightKeys[argvKey(argv)]; exists {
			values = append(values, append([]string(nil), argv...))
		}
	}
	return ArgvSetConstraint{Specified: true, Values: uniqueArgv(values)}, nil
}

func verificationRank(value VerificationStrength) int {
	switch value {
	case VerificationNone:
		return 0
	case VerificationObserved:
		return 1
	case VerificationStrong:
		return 2
	default:
		return -1
	}
}

func approvalRank(value ApprovalStrength) int {
	switch value {
	case ApprovalNone:
		return 0
	case ApprovalOnRisk:
		return 1
	case ApprovalOnSideEffect:
		return 2
	case ApprovalAlways:
		return 3
	default:
		return -1
	}
}

func networkRank(value NetworkMode) int {
	switch value {
	case NetworkDenied:
		return 0
	case NetworkAllowlist:
		return 1
	case NetworkUnrestricted:
		return 2
	default:
		return -1
	}
}

func stricterNetworkMode(left, right NetworkMode) NetworkMode {
	if networkRank(left) <= networkRank(right) {
		return left
	}
	return right
}

func minPositive(left, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left <= right {
		return left
	}
	return right
}

func minPositive64(left, right int64) int64 {
	if left <= 0 {
		return right
	}
	if right <= 0 || left <= right {
		return left
	}
	return right
}

func minPositiveDuration(left, right time.Duration) time.Duration {
	if left <= 0 {
		return right
	}
	if right <= 0 || left <= right {
		return left
	}
	return right
}

func validateStringSet(constraint StringSetConstraint, name string, validate func(string) bool) error {
	if !constraint.Specified && len(constraint.Values) != 0 {
		return fmt.Errorf("%s values require specified=true", name)
	}
	return validateStringValues(constraint.Values, name, validate)
}

func validateIntentSet(constraint IntentSetConstraint, name string) error {
	if !constraint.Specified && len(constraint.Values) != 0 {
		return fmt.Errorf("%s values require specified=true", name)
	}
	return validateIntentValues(constraint.Values, name)
}

func validateIntentValues(values []union.IntentKind, name string) error {
	seen := make(map[union.IntentKind]struct{}, len(values))
	for _, value := range values {
		if !knownIntentKind(value) {
			return fmt.Errorf("%s contains invalid value %q", name, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func knownIntentKind(value union.IntentKind) bool {
	switch value {
	case union.IntentCreateFile, union.IntentModifyFile, union.IntentRewriteFile, union.IntentDeleteFile,
		union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory,
		union.IntentProduceStructured, union.IntentCallTool, union.IntentExecuteCode, union.IntentComputerUse:
		return true
	default:
		return false
	}
}

func validateStringValues(values []string, name string, validate func(string) bool) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validate(value) {
			return fmt.Errorf("%s contains invalid value %q", name, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validatePathSet(constraint PathSetConstraint, name string) error {
	if !constraint.Specified && len(constraint.Values) != 0 {
		return fmt.Errorf("%s values require specified=true", name)
	}
	return validatePaths(constraint.Values, name)
}

func validatePaths(values []string, name string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("%s contains unsafe path %q", name, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains duplicate path %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateArgvSet(constraint ArgvSetConstraint, name string) error {
	if !constraint.Specified && len(constraint.Values) != 0 {
		return fmt.Errorf("%s values require specified=true", name)
	}
	return validateArgvValues(constraint.Values, name)
}

func validateArgvValues(values [][]string, name string) error {
	seen := make(map[string]struct{}, len(values))
	for _, argv := range values {
		if len(argv) == 0 {
			return fmt.Errorf("%s contains an empty argv", name)
		}
		for _, argument := range argv {
			if argument == "" || strings.ContainsAny(argument, "\x00\r\n") {
				return fmt.Errorf("%s contains an unsafe argument", name)
			}
		}
		key := argvKey(argv)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s contains duplicate argv", name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validReferenceValue(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsAny(value, "\x00\r\n")
}

func intersectStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	var result []string
	for _, value := range left {
		if _, exists := rightSet[value]; exists {
			result = append(result, value)
		}
	}
	return uniqueSorted(result)
}

// intersectPaths computes the semantic intersection of two sets of allowed
// path roots. A narrower child root remains allowed when the other policy only
// names its parent; exact string intersection would incorrectly turn a valid
// workspace -> task narrowing into an empty permission set.
func intersectPaths(left, right []string) []string {
	var result []string
	for _, leftPath := range left {
		for _, rightPath := range right {
			switch {
			case pathContains(leftPath, rightPath):
				result = append(result, rightPath)
			case pathContains(rightPath, leftPath):
				result = append(result, leftPath)
			}
		}
	}
	return compactPathRoots(result)
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func compactPathRoots(values []string) []string {
	values = uniqueSorted(values)
	result := make([]string, 0, len(values))
	for _, candidate := range values {
		covered := false
		for _, root := range result {
			if pathContains(root, candidate) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, candidate)
		}
	}
	return result
}

func unionStrings(left, right []string) []string {
	return uniqueSorted(append(append([]string(nil), left...), right...))
}

func unionIntentKinds(left, right []union.IntentKind) []union.IntentKind {
	return uniqueIntentKinds(append(append([]union.IntentKind(nil), left...), right...))
}

func uniqueIntentKinds(values []union.IntentKind) []union.IntentKind {
	set := make(map[union.IntentKind]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]union.IntentKind, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func argvKey(argv []string) string { return strings.Join(argv, "\x00") }

func cloneArgv(values [][]string) [][]string {
	clone := make([][]string, len(values))
	for index := range values {
		clone[index] = append([]string(nil), values[index]...)
	}
	return clone
}

func sortArgv(values [][]string) {
	sort.Slice(values, func(i, j int) bool { return argvKey(values[i]) < argvKey(values[j]) })
}

func uniqueArgv(values [][]string) [][]string {
	set := make(map[string][]string, len(values))
	for _, argv := range values {
		set[argvKey(argv)] = append([]string(nil), argv...)
	}
	result := make([][]string, 0, len(set))
	for _, argv := range set {
		result = append(result, argv)
	}
	sortArgv(result)
	return result
}

func unionArgv(left, right [][]string) [][]string {
	return uniqueArgv(append(cloneArgv(left), right...))
}
