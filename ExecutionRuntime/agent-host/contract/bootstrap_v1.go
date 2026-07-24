package contract

import (
	"slices"
	"sort"
	"strings"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	HostBootstrapContractVersionV1         = "praxis.agent-host/bootstrap/v1"
	HostBootstrapObjectKindV1              = "praxis.agent-host/HostBootstrapConfigV1"
	HostDeploymentCurrentContractVersionV1 = "praxis.agent-host/deployment-current/v1"
	HostDeploymentCurrentObjectKindV1      = "praxis.agent-host/HostDeploymentCurrentV1"
)

// HostBootstrapConfigV1 contains deployment-time identifiers only. It cannot
// carry constructors, package paths, raw Provider endpoints or secret values.
type HostBootstrapConfigV1 struct {
	ContractVersion                   string   `json:"contract_version"`
	ObjectKind                        string   `json:"object_kind"`
	HostID                            string   `json:"host_id"`
	StatePlaneBindingIDs              []string `json:"state_plane_binding_ids"`
	DefinitionSourceBindingID         string   `json:"definition_source_binding_id"`
	CatalogBindingID                  string   `json:"catalog_binding_id"`
	ResolutionFactsBindingID          string   `json:"resolution_facts_binding_id"`
	SecretBrokerBindingID             string   `json:"secret_broker_binding_id"`
	CredentialRegistryBindingID       string   `json:"credential_registry_binding_id"`
	ProviderEndpointRegistryBindingID string   `json:"provider_endpoint_registry_binding_id"`
	RuntimeServiceBindingIDs          []string `json:"runtime_service_binding_ids"`
	ApplicationServiceBindingIDs      []string `json:"application_service_binding_ids"`
	HarnessServiceBindingIDs          []string `json:"harness_service_binding_ids"`
	ListenBindingID                   string   `json:"listen_binding_id"`
	DiagnosticsPolicyBindingID        string   `json:"diagnostics_policy_binding_id"`
	ShutdownPolicyBindingID           string   `json:"shutdown_policy_binding_id"`
	EnabledControlAPISurfaces         []string `json:"enabled_control_api_surfaces"`
	CreatedUnixNano                   int64    `json:"created_unix_nano"`
	NotAfterUnixNano                  int64    `json:"not_after_unix_nano"`
	ContentDigest                     DigestV1 `json:"content_digest"`
}

func (c HostBootstrapConfigV1) canonicalV1() HostBootstrapConfigV1 {
	c.StatePlaneBindingIDs = sortedStringsV1(c.StatePlaneBindingIDs)
	c.RuntimeServiceBindingIDs = sortedStringsV1(c.RuntimeServiceBindingIDs)
	c.ApplicationServiceBindingIDs = sortedStringsV1(c.ApplicationServiceBindingIDs)
	c.HarnessServiceBindingIDs = sortedStringsV1(c.HarnessServiceBindingIDs)
	c.EnabledControlAPISurfaces = sortedStringsV1(c.EnabledControlAPISurfaces)
	return c
}

func (c HostBootstrapConfigV1) digestV1() (DigestV1, error) {
	c = c.canonicalV1()
	c.ContentDigest = ""
	return DigestJSONV1(struct {
		Domain string                `json:"domain"`
		Type   string                `json:"type"`
		Body   HostBootstrapConfigV1 `json:"body"`
	}{Domain: "praxis.agent-host.bootstrap-v1", Type: "HostBootstrapConfigV1", Body: c})
}

func SealHostBootstrapConfigV1(c HostBootstrapConfigV1) (HostBootstrapConfigV1, error) {
	if c.ContractVersion != "" && c.ContractVersion != HostBootstrapContractVersionV1 {
		return HostBootstrapConfigV1{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "Host bootstrap contract version drifted")
	}
	if c.ObjectKind != "" && c.ObjectKind != HostBootstrapObjectKindV1 {
		return HostBootstrapConfigV1{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "Host bootstrap object kind drifted")
	}
	c.ContractVersion = HostBootstrapContractVersionV1
	c.ObjectKind = HostBootstrapObjectKindV1
	c = c.canonicalV1()
	provided := c.ContentDigest
	c.ContentDigest = ""
	digest, err := c.digestV1()
	if err != nil {
		return HostBootstrapConfigV1{}, err
	}
	if provided != "" && provided != digest {
		return HostBootstrapConfigV1{}, NewError(ErrorConflict, "host_bootstrap_digest_drift", "Host bootstrap supplied a wrong non-zero digest")
	}
	c.ContentDigest = digest
	return c, c.ValidateHistoricalV1()
}

func (c HostBootstrapConfigV1) ValidateHistoricalV1() error {
	if c.ContractVersion != HostBootstrapContractVersionV1 || c.ObjectKind != HostBootstrapObjectKindV1 {
		return NewError(ErrorInvalidArgument, "host_bootstrap_contract_invalid", "Host bootstrap discriminator is unsupported")
	}
	if err := ValidateIdentifierV1("host id", c.HostID); err != nil {
		return err
	}
	canonical := c.canonicalV1()
	if !slices.Equal(c.StatePlaneBindingIDs, canonical.StatePlaneBindingIDs) || !slices.Equal(c.RuntimeServiceBindingIDs, canonical.RuntimeServiceBindingIDs) || !slices.Equal(c.ApplicationServiceBindingIDs, canonical.ApplicationServiceBindingIDs) || !slices.Equal(c.HarnessServiceBindingIDs, canonical.HarnessServiceBindingIDs) || !slices.Equal(c.EnabledControlAPISurfaces, canonical.EnabledControlAPISurfaces) {
		return NewError(ErrorConflict, "host_bootstrap_not_canonical", "Host bootstrap collections are not canonical")
	}
	for _, item := range []struct{ field, value string }{
		{"definition source binding", c.DefinitionSourceBindingID},
		{"catalog binding", c.CatalogBindingID},
		{"resolution facts binding", c.ResolutionFactsBindingID},
		{"secret broker binding", c.SecretBrokerBindingID},
		{"credential registry binding", c.CredentialRegistryBindingID},
		{"provider endpoint registry binding", c.ProviderEndpointRegistryBindingID},
		{"listen binding", c.ListenBindingID},
		{"diagnostics policy binding", c.DiagnosticsPolicyBindingID},
		{"shutdown policy binding", c.ShutdownPolicyBindingID},
	} {
		if err := ValidateIdentifierV1(item.field, item.value); err != nil {
			return err
		}
		if strings.Contains(item.value, "://") {
			return NewError(ErrorInvalidArgument, "raw_endpoint_forbidden", "Host bootstrap accepts binding IDs, not raw endpoints")
		}
	}
	for _, group := range [][]string{c.StatePlaneBindingIDs, c.RuntimeServiceBindingIDs, c.ApplicationServiceBindingIDs, c.HarnessServiceBindingIDs, c.EnabledControlAPISurfaces} {
		if len(group) == 0 {
			return NewError(ErrorInvalidArgument, "host_bootstrap_binding_missing", "Host bootstrap requires every binding group")
		}
		if err := validateUniqueRefsV1(group); err != nil {
			return err
		}
		for _, value := range group {
			if strings.Contains(value, "://") {
				return NewError(ErrorInvalidArgument, "raw_endpoint_forbidden", "Host bootstrap accepts binding IDs, not raw endpoints")
			}
		}
	}
	allowed := map[string]struct{}{"validate": {}, "assemble": {}, "run": {}, "inspect": {}, "stop": {}}
	for _, surface := range c.EnabledControlAPISurfaces {
		if _, ok := allowed[surface]; !ok {
			return NewError(ErrorInvalidArgument, "control_api_surface_unsupported", "Host bootstrap enables an unsupported control API surface")
		}
	}
	if c.CreatedUnixNano <= 0 || c.NotAfterUnixNano <= c.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "host_bootstrap_window_invalid", "Host bootstrap time window is invalid")
	}
	expected, err := c.digestV1()
	if err != nil || expected != c.ContentDigest {
		return NewError(ErrorConflict, "host_bootstrap_digest_drift", "Host bootstrap digest drifted")
	}
	return nil
}

func (c HostBootstrapConfigV1) ValidateCurrentV1(now time.Time) error {
	if err := c.ValidateHistoricalV1(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < c.CreatedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "Host bootstrap was checked before its creation watermark")
	}
	if !now.Before(time.Unix(0, c.NotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_bootstrap_expired", "Host bootstrap expired")
	}
	return nil
}

type HostServiceBindingRoleV1 string

const (
	HostServiceDefinitionSourceV1   HostServiceBindingRoleV1 = "definition_source"
	HostServiceCatalogV1            HostServiceBindingRoleV1 = "catalog"
	HostServiceResolutionFactsV1    HostServiceBindingRoleV1 = "resolution_facts"
	HostServiceSecretBrokerV1       HostServiceBindingRoleV1 = "secret_broker"
	HostServiceCredentialRegistryV1 HostServiceBindingRoleV1 = "credential_registry"
	HostServiceProviderRegistryV1   HostServiceBindingRoleV1 = "provider_endpoint_registry"
	HostServiceRuntimeV1            HostServiceBindingRoleV1 = "runtime_service"
	HostServiceApplicationV1        HostServiceBindingRoleV1 = "application_service"
	HostServiceHarnessV1            HostServiceBindingRoleV1 = "harness_service"
	HostServiceListenV1             HostServiceBindingRoleV1 = "listen"
	HostServiceDiagnosticsV1        HostServiceBindingRoleV1 = "diagnostics_policy"
	HostServiceShutdownV1           HostServiceBindingRoleV1 = "shutdown_policy"
)

type HostServiceBindingRefV1 struct {
	Role            HostServiceBindingRoleV1 `json:"role"`
	ConfiguredID    string                   `json:"configured_id"`
	BindingRef      ExactRefV1               `json:"binding_ref"`
	Capability      string                   `json:"capability"`
	ExpiresUnixNano int64                    `json:"expires_unix_nano"`
}

func (r HostServiceBindingRefV1) Validate() error {
	if !validHostServiceRoleV1(r.Role) {
		return NewError(ErrorInvalidArgument, "host_service_role_invalid", "Host service binding role is unsupported")
	}
	if err := ValidateIdentifierV1("configured binding id", r.ConfiguredID); err != nil {
		return err
	}
	if err := r.BindingRef.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("service capability", r.Capability); err != nil {
		return err
	}
	if r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_service_binding_expiry_invalid", "Host service binding expiry is invalid")
	}
	return nil
}

type HostDeploymentCurrentRefV1 struct {
	HostID          string   `json:"host_id"`
	DeploymentID    string   `json:"deployment_id"`
	Revision        uint64   `json:"revision"`
	BootstrapDigest DigestV1 `json:"bootstrap_digest"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
	Digest          DigestV1 `json:"digest"`
}

func (r HostDeploymentCurrentRefV1) Validate() error {
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("deployment id", r.DeploymentID); err != nil {
		return err
	}
	if r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_deployment_ref_incomplete", "Host deployment current Ref is incomplete")
	}
	if err := r.BootstrapDigest.Validate(); err != nil {
		return err
	}
	return r.Digest.Validate()
}

type HostDeploymentCurrentV1 struct {
	ContractVersion  string                             `json:"contract_version"`
	ObjectKind       string                             `json:"object_kind"`
	Ref              HostDeploymentCurrentRefV1         `json:"ref"`
	ResourceHandles  []runtimeports.ResourceHandleRefV1 `json:"resource_handles"`
	ServiceBindings  []HostServiceBindingRefV1          `json:"service_bindings"`
	CheckedUnixNano  int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                              `json:"expires_unix_nano"`
	ProjectionDigest DigestV1                           `json:"projection_digest"`
}

func (p HostDeploymentCurrentV1) canonicalV1() HostDeploymentCurrentV1 {
	p.ResourceHandles = append([]runtimeports.ResourceHandleRefV1(nil), p.ResourceHandles...)
	sort.Slice(p.ResourceHandles, func(i, j int) bool {
		a, b := p.ResourceHandles[i], p.ResourceHandles[j]
		if a.Owner.Domain != b.Owner.Domain {
			return a.Owner.Domain < b.Owner.Domain
		}
		if a.Owner.ID != b.Owner.ID {
			return a.Owner.ID < b.Owner.ID
		}
		return a.ID < b.ID
	})
	p.ServiceBindings = append([]HostServiceBindingRefV1(nil), p.ServiceBindings...)
	sort.Slice(p.ServiceBindings, func(i, j int) bool {
		if p.ServiceBindings[i].Role != p.ServiceBindings[j].Role {
			return p.ServiceBindings[i].Role < p.ServiceBindings[j].Role
		}
		return p.ServiceBindings[i].ConfiguredID < p.ServiceBindings[j].ConfiguredID
	})
	return p
}

func (p HostDeploymentCurrentV1) digestV1() (DigestV1, error) {
	p = p.canonicalV1()
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   HostDeploymentCurrentV1 `json:"body"`
	}{Domain: "praxis.agent-host.deployment-current-v1", Type: "HostDeploymentCurrentV1", Body: p})
}

func SealHostDeploymentCurrentV1(p HostDeploymentCurrentV1) (HostDeploymentCurrentV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != HostDeploymentCurrentContractVersionV1 {
		return HostDeploymentCurrentV1{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "Host deployment current version drifted")
	}
	if p.ObjectKind != "" && p.ObjectKind != HostDeploymentCurrentObjectKindV1 {
		return HostDeploymentCurrentV1{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "Host deployment current object kind drifted")
	}
	p.ContractVersion = HostDeploymentCurrentContractVersionV1
	p.ObjectKind = HostDeploymentCurrentObjectKindV1
	p = p.canonicalV1()
	providedRef, providedProjection := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.digestV1()
	if err != nil {
		return HostDeploymentCurrentV1{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return HostDeploymentCurrentV1{}, NewError(ErrorConflict, "host_deployment_digest_drift", "Host deployment current supplied a wrong non-zero digest")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.ValidateHistoricalV1()
}

func (p HostDeploymentCurrentV1) ValidateHistoricalV1() error {
	if p.ContractVersion != HostDeploymentCurrentContractVersionV1 || p.ObjectKind != HostDeploymentCurrentObjectKindV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_deployment_current_incomplete", "Host deployment current is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	canonical := p.canonicalV1()
	if !slices.Equal(p.ResourceHandles, canonical.ResourceHandles) || !slices.Equal(p.ServiceBindings, canonical.ServiceBindings) {
		return NewError(ErrorConflict, "host_deployment_not_canonical", "Host deployment collections are not canonical")
	}
	if p.Ref.ExpiresUnixNano != p.ExpiresUnixNano || len(p.ResourceHandles) == 0 || len(p.ServiceBindings) == 0 {
		return NewError(ErrorConflict, "host_deployment_current_drift", "Host deployment current fields drifted")
	}
	seenResources := map[string]struct{}{}
	minimum := int64(^uint64(0) >> 1)
	for _, ref := range p.ResourceHandles {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Owner.Domain) + "\x00" + string(ref.Owner.ID) + "\x00" + ref.ID
		if _, ok := seenResources[key]; ok {
			return NewError(ErrorConflict, "duplicate_resource_handle", "Host deployment duplicates a resource handle")
		}
		seenResources[key] = struct{}{}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	seenBindings := map[string]struct{}{}
	roles := map[HostServiceBindingRoleV1]int{}
	for _, ref := range p.ServiceBindings {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Role) + "\x00" + ref.ConfiguredID
		if _, ok := seenBindings[key]; ok {
			return NewError(ErrorConflict, "duplicate_service_binding", "Host deployment duplicates a service binding")
		}
		seenBindings[key] = struct{}{}
		roles[ref.Role]++
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	for _, role := range requiredHostServiceRolesV1() {
		if roles[role] == 0 {
			return NewError(ErrorPrecondition, "host_service_binding_missing", "Host deployment is missing a required service binding")
		}
	}
	if p.ExpiresUnixNano != minimum {
		return NewError(ErrorConflict, "host_deployment_expiry_drift", "Host deployment expiry is not the exact minimum bound input")
	}
	expected, err := p.digestV1()
	if err != nil || expected != p.ProjectionDigest || p.Ref.Digest != p.ProjectionDigest {
		return NewError(ErrorConflict, "host_deployment_digest_drift", "Host deployment current digest drifted")
	}
	return nil
}

func (p HostDeploymentCurrentV1) ValidateForBootstrapV1(bootstrap HostBootstrapConfigV1, now time.Time) error {
	if err := bootstrap.ValidateCurrentV1(now); err != nil {
		return err
	}
	if err := p.ValidateCurrentV1(p.Ref, now); err != nil {
		return err
	}
	if p.Ref.HostID != bootstrap.HostID || p.Ref.BootstrapDigest != bootstrap.ContentDigest || p.ExpiresUnixNano > bootstrap.NotAfterUnixNano {
		return NewError(ErrorConflict, "host_deployment_bootstrap_drift", "Host deployment current is not bound to the exact bootstrap")
	}
	expectedResources := map[string]struct{}{}
	for _, id := range bootstrap.StatePlaneBindingIDs {
		expectedResources[id] = struct{}{}
	}
	for _, handle := range p.ResourceHandles {
		if _, ok := expectedResources[handle.ID]; !ok {
			return NewError(ErrorConflict, "host_deployment_resource_drift", "Host deployment resource is not declared by the bootstrap")
		}
		delete(expectedResources, handle.ID)
	}
	if len(expectedResources) != 0 {
		return NewError(ErrorConflict, "host_deployment_resource_missing", "Host deployment omitted a bootstrap resource")
	}
	expectedServices := bootstrapServiceBindingsV1(bootstrap)
	for _, binding := range p.ServiceBindings {
		key := string(binding.Role) + "\x00" + binding.ConfiguredID
		if _, ok := expectedServices[key]; !ok {
			return NewError(ErrorConflict, "host_deployment_service_drift", "Host deployment service is not declared by the bootstrap")
		}
		delete(expectedServices, key)
	}
	if len(expectedServices) != 0 {
		return NewError(ErrorConflict, "host_deployment_service_missing", "Host deployment omitted a bootstrap service")
	}
	return nil
}

func (p HostDeploymentCurrentV1) ValidateCurrentV1(expected HostDeploymentCurrentRefV1, now time.Time) error {
	if err := p.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return NewError(ErrorConflict, "host_deployment_ref_drift", "Host deployment current exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "Host deployment current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return NewError(ErrorPrecondition, "host_deployment_expired", "Host deployment current expired")
	}
	return nil
}

func sortedStringsV1(values []string) []string {
	clone := append([]string(nil), values...)
	sort.Strings(clone)
	return clone
}

func validHostServiceRoleV1(role HostServiceBindingRoleV1) bool {
	for _, candidate := range requiredHostServiceRolesV1() {
		if role == candidate {
			return true
		}
	}
	return false
}

func requiredHostServiceRolesV1() []HostServiceBindingRoleV1 {
	return []HostServiceBindingRoleV1{
		HostServiceDefinitionSourceV1, HostServiceCatalogV1, HostServiceResolutionFactsV1,
		HostServiceSecretBrokerV1, HostServiceCredentialRegistryV1, HostServiceProviderRegistryV1,
		HostServiceRuntimeV1, HostServiceApplicationV1, HostServiceHarnessV1,
		HostServiceListenV1, HostServiceDiagnosticsV1, HostServiceShutdownV1,
	}
}

func bootstrapServiceBindingsV1(c HostBootstrapConfigV1) map[string]struct{} {
	result := map[string]struct{}{}
	add := func(role HostServiceBindingRoleV1, ids ...string) {
		for _, id := range ids {
			result[string(role)+"\x00"+id] = struct{}{}
		}
	}
	add(HostServiceDefinitionSourceV1, c.DefinitionSourceBindingID)
	add(HostServiceCatalogV1, c.CatalogBindingID)
	add(HostServiceResolutionFactsV1, c.ResolutionFactsBindingID)
	add(HostServiceSecretBrokerV1, c.SecretBrokerBindingID)
	add(HostServiceCredentialRegistryV1, c.CredentialRegistryBindingID)
	add(HostServiceProviderRegistryV1, c.ProviderEndpointRegistryBindingID)
	add(HostServiceRuntimeV1, c.RuntimeServiceBindingIDs...)
	add(HostServiceApplicationV1, c.ApplicationServiceBindingIDs...)
	add(HostServiceHarnessV1, c.HarnessServiceBindingIDs...)
	add(HostServiceListenV1, c.ListenBindingID)
	add(HostServiceDiagnosticsV1, c.DiagnosticsPolicyBindingID)
	add(HostServiceShutdownV1, c.ShutdownPolicyBindingID)
	return result
}
