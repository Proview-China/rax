package contract

import "sort"

type HostConfigV1 struct {
	ContractVersion      string   `json:"contract_version"`
	HostID               string   `json:"host_id"`
	DefinitionSourceRef  string   `json:"definition_source_ref"`
	StatePlaneBindings   []string `json:"state_plane_bindings"`
	ProviderEndpointRefs []string `json:"provider_endpoint_refs"`
	SecretBrokerRef      string   `json:"secret_broker_ref"`
	CatalogRef           string   `json:"catalog_ref"`
	ResolutionFactsRef   string   `json:"resolution_facts_ref"`
	RuntimeServiceRefs   []string `json:"runtime_service_refs"`
	ListenRef            string   `json:"listen_ref"`
	DiagnosticsPolicyRef string   `json:"diagnostics_policy_ref"`
}

func (c HostConfigV1) Validate() error {
	if c.ContractVersion != ContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "host config contract version is unsupported")
	}
	for _, item := range []struct{ field, value string }{
		{"host_id", c.HostID}, {"definition_source_ref", c.DefinitionSourceRef},
		{"secret_broker_ref", c.SecretBrokerRef}, {"catalog_ref", c.CatalogRef},
		{"resolution_facts_ref", c.ResolutionFactsRef}, {"listen_ref", c.ListenRef},
		{"diagnostics_policy_ref", c.DiagnosticsPolicyRef},
	} {
		if err := ValidateIdentifierV1(item.field, item.value); err != nil {
			return err
		}
	}
	if len(c.StatePlaneBindings) == 0 || len(c.RuntimeServiceRefs) == 0 {
		return NewError(ErrorInvalidArgument, "required_reference_missing", "state plane and runtime service refs are required")
	}
	for _, values := range [][]string{c.StatePlaneBindings, c.ProviderEndpointRefs, c.RuntimeServiceRefs} {
		if err := validateUniqueRefsV1(values); err != nil {
			return err
		}
	}
	return nil
}

func validateUniqueRefsV1(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := ValidateIdentifierV1("reference", value); err != nil {
			return err
		}
		if _, ok := seen[value]; ok {
			return NewError(ErrorConflict, "duplicate_reference", "host config contains a duplicate reference")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (c HostConfigV1) CanonicalV1() HostConfigV1 {
	clone := c
	clone.StatePlaneBindings = append([]string(nil), c.StatePlaneBindings...)
	clone.ProviderEndpointRefs = append([]string(nil), c.ProviderEndpointRefs...)
	clone.RuntimeServiceRefs = append([]string(nil), c.RuntimeServiceRefs...)
	sort.Strings(clone.StatePlaneBindings)
	sort.Strings(clone.ProviderEndpointRefs)
	sort.Strings(clone.RuntimeServiceRefs)
	return clone
}

func (c HostConfigV1) DigestV1() (DigestV1, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return DigestJSONV1(c.CanonicalV1())
}
