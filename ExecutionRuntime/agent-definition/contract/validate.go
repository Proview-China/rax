package contract

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var (
	visibleIDV1 = regexp.MustCompile(`^[!-~]+$`)
	mediaTypeV1 = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)
)

func invalid(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorInvalidArgument, reason, message)
}
func precondition(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorPreconditionFailed, reason, message)
}

func validateIDV1(value, field string) error {
	if len(value) == 0 || len(value) > 512 || strings.TrimSpace(value) != value || !visibleIDV1.MatchString(value) {
		return invalid(core.ReasonInvalidReference, field+" must be bounded visible ASCII")
	}
	return nil
}

func ValidateNamespacedNameV1(value string) error {
	if len(value) < 3 || len(value) > 128 || strings.Count(value, "/") != 1 {
		return invalid(core.ReasonInvalidNamespace, "name must be one namespace/name pair")
	}
	parts := strings.Split(value, "/")
	if !validLowerNameV1(parts[0], true) || !validLowerNameV1(parts[1], false) {
		return invalid(core.ReasonInvalidNamespace, "namespaced name must use canonical lowercase ASCII")
	}
	return nil
}

func (r ObjectRefV1) Validate() error {
	if err := validateIDV1(r.ID, "object ref id"); err != nil {
		return err
	}
	if r.Revision == 0 {
		return invalid(core.ReasonRevisionConflict, "object ref revision is required")
	}
	return r.Digest.Validate()
}

func (r VersionRangeV1) Validate() error {
	minimum, err := core.ParseSemanticVersion(r.MinimumInclusive)
	if err != nil || minimum.String() != r.MinimumInclusive || len(minimum.Build) != 0 {
		return invalid(core.ReasonInvalidSemanticVersion, "minimum version must be canonical SemVer without build metadata")
	}
	maximum, err := core.ParseSemanticVersion(r.MaximumExclusive)
	if err != nil || maximum.String() != r.MaximumExclusive || len(maximum.Build) != 0 {
		return invalid(core.ReasonInvalidSemanticVersion, "maximum version must be canonical SemVer without build metadata")
	}
	if core.CompareSemanticVersion(minimum, maximum) >= 0 {
		return invalid(core.ReasonInvalidSemanticVersion, "version range must be non-empty")
	}
	return nil
}

func (r SchemaRefV1) Validate() error {
	if !validLowerNameV1(r.Namespace, true) || !validLowerNameV1(r.Name, false) {
		return invalid(core.ReasonInvalidNamespace, "schema namespace and name must be canonical lowercase ASCII")
	}
	version, err := core.ParseSemanticVersion(r.Version)
	if err != nil || version.String() != r.Version {
		return invalid(core.ReasonInvalidSemanticVersion, "schema version must be canonical SemVer")
	}
	if !mediaTypeV1.MatchString(r.MediaType) {
		return invalid(core.ReasonInvalidReference, "schema media type is invalid")
	}
	return r.ContentDigest.Validate()
}

func (r AgentDefinitionRefV1) Validate() error {
	if err := ValidateNamespacedNameV1(r.DefinitionID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return invalid(core.ReasonRevisionConflict, "definition ref revision is required")
	}
	return r.Digest.Validate()
}

func ValidateSourceV1(source AgentDefinitionSourceV1, catalog ValidationCatalogV1) error {
	if source.ContractVersion != ContractVersionV1 {
		return invalid(core.ReasonComponentMismatch, "agent definition contract version is not supported")
	}
	if err := ValidateNamespacedNameV1(source.DefinitionID); err != nil {
		return err
	}
	if source.Revision == 0 {
		return invalid(core.ReasonRevisionConflict, "definition revision is required")
	}
	for _, ref := range []ObjectRefV1{source.IdentityRef, source.ProfileSelectionRef, source.ProvenanceRef, source.ApprovalRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if source.EffectiveWindow.NotBeforeUnixNano <= 0 || source.EffectiveWindow.NotAfterUnixNano <= source.EffectiveWindow.NotBeforeUnixNano {
		return invalid(core.ReasonInvalidReference, "effective window is invalid")
	}
	if strings.TrimSpace(source.ChangeReason) == "" || len(source.ChangeReason) > 2048 {
		return invalid(core.ReasonInvalidReference, "change reason is required and bounded")
	}
	if len(source.Components) == 0 || len(source.Components) > MaxDefinitionEntriesV1 || len(source.SecretRefs) > MaxDefinitionEntriesV1 || len(source.Extensions) > MaxDefinitionEntriesV1 {
		return invalid(core.ReasonCanonicalLimitExceeded, "definition collection exceeds its bound")
	}
	if err := validatePoliciesV1(source.PolicyRefs); err != nil {
		return err
	}
	if err := validateComponentsV1(source.Components, catalog); err != nil {
		return err
	}
	if err := validateSecretsV1(source.SecretRefs); err != nil {
		return err
	}
	return validateExtensionsV1(source.Extensions, catalog)
}

func (d AgentDefinitionV1) Validate(catalog ValidationCatalogV1) error {
	if err := validateDefinitionWithoutDigestV1(d, catalog); err != nil {
		return err
	}
	digest, err := DefinitionDigestV1(d, catalog)
	if err != nil {
		return err
	}
	if digest != d.Digest {
		return precondition(core.ReasonInvalidDigest, "definition digest does not match canonical content")
	}
	return nil
}

func validateDefinitionWithoutDigestV1(d AgentDefinitionV1, catalog ValidationCatalogV1) error {
	if err := ValidateSourceV1(d.AgentDefinitionSourceV1, catalog); err != nil {
		return err
	}
	if d.CreatedUnixNano <= 0 || d.CreatedUnixNano < d.EffectiveWindow.NotBeforeUnixNano || d.CreatedUnixNano >= d.EffectiveWindow.NotAfterUnixNano {
		return precondition(core.ReasonBindingExpired, "definition creation is outside its effective window")
	}
	sourceDigest, err := SourceDigestV1(d.AgentDefinitionSourceV1, catalog)
	if err != nil {
		return err
	}
	if sourceDigest != d.SourceDigest {
		return precondition(core.ReasonInvalidDigest, "definition source digest does not match canonical source")
	}
	return nil
}

func validatePoliciesV1(p PolicyRefsV1) error {
	refs := []ObjectRefV1{p.Runtime, p.Authority, p.Review, p.Budget, p.Sandbox, p.Context, p.Continuity, p.ToolMCP, p.MemoryKnowledge}
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateComponentsV1(values []ComponentRequirementV1, catalog ValidationCatalogV1) error {
	if err := validateCatalogV1(catalog); err != nil {
		return err
	}
	kinds := set(catalog.Kinds)
	caps := set(catalog.Capabilities)
	for _, required := range requiredCoreKindsV1 {
		kinds[required] = struct{}{}
	}
	ids := map[string]struct{}{}
	presentCore := map[string]bool{}
	for _, value := range values {
		if err := ValidateNamespacedNameV1(value.ComponentID); err != nil {
			return err
		}
		if _, ok := ids[value.ComponentID]; ok {
			return invalid(core.ReasonDuplicateCanonicalKey, "component id is duplicated")
		}
		ids[value.ComponentID] = struct{}{}
		if err := ValidateNamespacedNameV1(value.Kind); err != nil {
			return err
		}
		if _, ok := kinds[value.Kind]; !ok {
			return precondition(core.ReasonUnknownGovernanceCategory, "component kind is not registered")
		}
		if err := value.SemanticVersion.Validate(); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV1(value.ContractName); err != nil {
			return err
		}
		if err := value.ContractVersion.Validate(); err != nil {
			return err
		}
		if len(value.RequiredCapabilities) == 0 || len(value.RequiredCapabilities) > MaxDefinitionEntriesV1 {
			return invalid(core.ReasonUnknownCapability, "component requires a bounded non-empty capability set")
		}
		seenCaps := map[string]struct{}{}
		for _, capability := range value.RequiredCapabilities {
			if err := ValidateNamespacedNameV1(capability); err != nil {
				return err
			}
			if _, ok := seenCaps[capability]; ok {
				return invalid(core.ReasonDuplicateCanonicalKey, "required capability is duplicated")
			}
			seenCaps[capability] = struct{}{}
			if _, ok := caps[capability]; !ok {
				return precondition(core.ReasonUnknownCapability, "required capability is not registered")
			}
		}
		if value.SupportMode != SupportModeProductionV1 {
			return precondition(core.ReasonComponentMismatch, "sealed V1 components must use production support mode")
		}
		if !validLocality(value.LocalityConstraint) {
			return invalid(core.ReasonInvalidReference, "component locality constraint is invalid")
		}
		if value.ResidualPolicy.Allowed {
			if err := value.ResidualPolicy.InspectOwnerRef.Validate(); err != nil {
				return precondition(core.ReasonOwnerMissing, "allowed residual requires an Inspect owner")
			}
			if err := value.ResidualPolicy.CleanupOwnerRef.Validate(); err != nil {
				return precondition(core.ReasonOwnerMissing, "allowed residual requires a Cleanup owner")
			}
		} else if value.ResidualPolicy.InspectOwnerRef.ID != "" || value.ResidualPolicy.CleanupOwnerRef.ID != "" {
			return invalid(core.ReasonOwnerConflict, "denied residual cannot declare residual owners")
		}
		deps := map[string]struct{}{}
		for _, dependency := range value.DependencyIDs {
			if err := ValidateNamespacedNameV1(dependency); err != nil {
				return err
			}
			if dependency == value.ComponentID {
				return precondition(core.ReasonDependencyCycle, "component cannot depend on itself")
			}
			if _, ok := deps[dependency]; ok {
				return invalid(core.ReasonDuplicateCanonicalKey, "dependency is duplicated")
			}
			deps[dependency] = struct{}{}
		}
		for _, coreKind := range requiredCoreKindsV1 {
			if value.Kind == coreKind {
				if !value.Required {
					return precondition(core.ReasonComponentMissing, "required V1 core component cannot be optional")
				}
				if presentCore[coreKind] {
					return invalid(core.ReasonDuplicateCanonicalKey, "required core kind is duplicated")
				}
				presentCore[coreKind] = true
			}
		}
	}
	for _, value := range values {
		for _, dep := range value.DependencyIDs {
			if _, ok := ids[dep]; !ok {
				return precondition(core.ReasonComponentMissing, "component dependency is absent")
			}
		}
	}
	if componentCycle(values) {
		return precondition(core.ReasonDependencyCycle, "component dependency graph contains a cycle")
	}
	for _, kind := range requiredCoreKindsV1 {
		if !presentCore[kind] {
			return precondition(core.ReasonComponentMissing, "required V1 core component is absent: "+kind)
		}
	}
	return nil
}

func validateSecretsV1(values []SecretRefV1) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if filepath.IsAbs(value.SecretID) || strings.Contains(value.SecretID, "\\") || strings.Contains(value.SecretID, "..") || strings.HasPrefix(strings.ToLower(value.SecretID), "file:") {
			return invalid(core.ReasonInvalidReference, "secret id must be a namespaced identifier, not a local path or URI")
		}
		if err := ValidateNamespacedNameV1(value.SecretID); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV1(value.Class); err != nil {
			return err
		}
		if err := value.RequestedScopeDigest.Validate(); err != nil {
			return err
		}
		if _, ok := seen[value.SecretID]; ok {
			return invalid(core.ReasonDuplicateCanonicalKey, "secret id is duplicated")
		}
		seen[value.SecretID] = struct{}{}
	}
	return nil
}

func validateExtensionsV1(values []ExtensionV1, catalog ValidationCatalogV1) error {
	registered := set(catalog.RegisteredExtensionKeys)
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := ValidateNamespacedNameV1(value.Key); err != nil {
			return err
		}
		if _, ok := seen[value.Key]; ok {
			return invalid(core.ReasonDuplicateCanonicalKey, "extension key is duplicated")
		}
		seen[value.Key] = struct{}{}
		if err := value.Schema.Validate(); err != nil {
			return err
		}
		if err := value.ContentDigest.Validate(); err != nil {
			return err
		}
		if len(value.Payload) == 0 || len(value.Payload) > MaxExtensionBytesV1 {
			return invalid(core.ReasonCanonicalLimitExceeded, "extension payload is empty or too large")
		}
		var strictRaw json.RawMessage
		if err := core.DecodeStrictJSON(value.Payload, &strictRaw); err != nil {
			return err
		}
		var payload any
		dec := json.NewDecoder(bytes.NewReader(value.Payload))
		dec.UseNumber()
		if err := dec.Decode(&payload); err != nil {
			return invalid(core.ReasonInvalidCanonicalForm, "extension payload must be strict JSON")
		}
		if dec.Decode(&struct{}{}) == nil {
			return invalid(core.ReasonInvalidCanonicalForm, "extension payload has trailing JSON")
		}
		canonical, _ := json.Marshal(payload)
		if core.DigestBytes(canonical) != value.ContentDigest {
			return precondition(core.ReasonInvalidDigest, "extension content digest mismatch")
		}
		if err := validateOpaqueSafety(payload); err != nil {
			return err
		}
		_, known := registered[value.Key]
		if value.Required && !known {
			return precondition(core.ReasonUnknownRequiredExtension, "required extension is not registered")
		}
		// Unknown optional extensions remain canonical opaque content.
	}
	return nil
}

func validateOpaqueSafety(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			for _, marker := range []string{"secret", "token", "password", "private_key", "api_key", "authorization", "credential", "cookie"} {
				if strings.Contains(normalized, marker) {
					return invalid(core.ReasonInvalidReference, "opaque extension contains an obvious secret-bearing key")
				}
			}
			if err := validateOpaqueSafety(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := validateOpaqueSafety(item); err != nil {
				return err
			}
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		lower := strings.ToLower(trimmed)
		slashed := strings.ReplaceAll(trimmed, "\\", "/")
		if strings.Contains(lower, "sk-") || strings.HasPrefix(lower, "bearer ") || strings.Contains(strings.ToUpper(trimmed), "-----BEGIN ") || strings.HasPrefix(lower, "file:") || filepath.IsAbs(trimmed) || strings.Contains(trimmed, "\\") || windowsPathV1(trimmed) || pathTraversalV1(slashed) {
			return invalid(core.ReasonInvalidReference, "opaque extension contains an obvious secret value, file URI, or local path")
		}
	}
	return nil
}

func windowsPathV1(value string) bool {
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, `//`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func pathTraversalV1(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func validLocality(v LocalityConstraintV1) bool {
	switch v {
	case LocalityHostControlPlaneV1, LocalityInstanceDataPlaneV1, LocalityExternalStatePlaneV1, LocalityRemoteProviderV1:
		return true
	}
	return false
}

func validLowerNameV1(value string, namespace bool) bool {
	if value == "" || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	last := value[len(value)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}
	for _, character := range []byte(value) {
		allowed := (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-'
		if namespace {
			allowed = allowed || character == '.'
		} else {
			allowed = allowed || character == '_' || character == '.'
		}
		if !allowed {
			return false
		}
	}
	return true
}

func validateCatalogV1(catalog ValidationCatalogV1) error {
	groups := []struct {
		name   string
		values []string
	}{
		{"kind", catalog.Kinds},
		{"capability", catalog.Capabilities},
		{"registered extension", catalog.RegisteredExtensionKeys},
	}
	for _, group := range groups {
		seen := map[string]struct{}{}
		for _, value := range group.values {
			if err := ValidateNamespacedNameV1(value); err != nil {
				return err
			}
			if _, exists := seen[value]; exists {
				return invalid(core.ReasonDuplicateCanonicalKey, group.name+" catalog contains a duplicate")
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}
func set(values []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, v := range values {
		m[v] = struct{}{}
	}
	return m
}

func componentCycle(values []ComponentRequirementV1) bool {
	graph := map[string][]string{}
	for _, v := range values {
		graph[v.ComponentID] = v.DependencyIDs
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, d := range graph[id] {
			if visit(d) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	keys := make([]string, 0, len(graph))
	for k := range graph {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if visit(k) {
			return true
		}
	}
	return false
}

func (c DefinitionCurrentV1) Validate() error {
	if err := c.Definition.Validate(); err != nil {
		return err
	}
	if c.Revision == 0 || c.UpdatedUnixNano <= 0 || c.CheckedUnixNano < c.UpdatedUnixNano {
		return invalid(core.ReasonInvalidReference, "current projection coordinates are invalid")
	}
	switch c.State {
	case DefinitionCurrentActiveV1, DefinitionCurrentRevokedV1, DefinitionCurrentExpiredV1:
	default:
		return invalid(core.ReasonInvalidState, "current state is invalid")
	}
	copy := c
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(DigestDomainV1, DigestVersionV1, "DefinitionCurrentV1", copy)
	if err != nil {
		return err
	}
	if digest != c.ProjectionDigest {
		return precondition(core.ReasonInvalidDigest, "current projection digest drifted")
	}
	return nil
}
