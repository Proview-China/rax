package upstream

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	idPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)
	refPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$`)
	hostPattern      = regexp.MustCompile(`^[A-Za-z0-9._:{}\-\[\]]+$`)
	protocolPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_./-]{0,127}$`)
	envNamePattern   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)
	httpTokenPattern = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")
	keyPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,31}$`)
)

// FieldError identifies one invalid route field.
type FieldError struct {
	Field   string
	Problem string
}

// ValidationError contains every independently detectable route error.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return formatFieldErrors("invalid upstream route", e.Fields)
}

// HasField reports whether validation found a problem for field or one of its
// descendants.
func (e *ValidationError) HasField(field string) bool {
	if e == nil {
		return false
	}
	return hasField(e.Fields, field)
}

// Validate checks identity, policy, endpoint and credential-binding invariants.
func (r UpstreamRoute) Validate() error {
	var fields []FieldError
	add := func(field, problem string) {
		fields = append(fields, FieldError{Field: field, Problem: problem})
	}

	validateID(add, "id", string(r.ID))
	validateID(add, "provider", string(r.Provider))
	validateID(add, "model.canonical_family", r.Model.CanonicalFamily)
	validateText(add, "model.provider_model_ref", r.Model.ProviderModelRef)
	validateID(add, "offering.id", string(r.Offering.ID))
	if !r.Offering.Kind.valid() {
		add("offering.kind", "unsupported value")
	}
	if !r.Offering.Entitlement.AllowedUsage.valid() {
		add("offering.entitlement.allowed_usage", "unsupported value")
	}
	if billingPlan := r.Offering.BillingPlan; billingPlan != nil {
		validateID(add, "offering.billing_plan.id", billingPlan.ID)
		if billingPlan.Kind != BillingPlanSavings {
			add("offering.billing_plan.kind", "unsupported value")
		}
		validateText(add, "offering.billing_plan.billing_owner", billingPlan.BillingOwner)
		if billingPlan.AppliesToOfferingID != r.Offering.ID {
			add("offering.billing_plan.applies_to_offering_id", "must reference the containing offering")
		}
	}
	if r.Offering.Entitlement.AllowsAutomaticPAYGSwitch {
		add("offering.entitlement.allows_automatic_payg_switch", "implicit billing fallback is forbidden")
	}
	if !r.Offering.Entitlement.SubjectPolicy.valid() {
		add("offering.entitlement.subject_policy", "unsupported subject policy")
	}
	if !r.Offering.Entitlement.TenancyPolicy.valid() {
		add("offering.entitlement.tenancy_policy", "unsupported tenancy policy")
	}
	if !r.Offering.Entitlement.ExecutionPolicy.valid() {
		add("offering.entitlement.execution_policy", "unsupported execution policy")
	}
	if !r.Offering.Entitlement.ProductionPolicy.valid() {
		add("offering.entitlement.production_policy", "unsupported production policy")
	}
	for index, restriction := range r.Offering.Entitlement.ClientRestrictions {
		if strings.TrimSpace(restriction) == "" {
			add(fmt.Sprintf("offering.entitlement.client_restrictions[%d]", index), "must not be blank")
		}
	}
	seenClients := make(map[string]struct{}, len(r.Offering.Entitlement.AllowedClientNames))
	for index, client := range r.Offering.Entitlement.AllowedClientNames {
		field := fmt.Sprintf("offering.entitlement.allowed_client_names[%d]", index)
		if !refPattern.MatchString(client) {
			add(field, "must be a stable client identifier")
		}
		if _, exists := seenClients[client]; exists {
			add(field, "duplicates an earlier client identifier")
		}
		seenClients[client] = struct{}{}
	}

	validateID(add, "deployment.id", string(r.Deployment.ID))
	if !r.Deployment.Kind.valid() {
		add("deployment.kind", "unsupported value")
	}
	validateOptionalRef(add, "deployment.region", r.Deployment.Region)
	validateOptionalRef(add, "deployment.project_ref", r.Deployment.ProjectRef)
	validateOptionalRef(add, "deployment.workspace_ref", r.Deployment.WorkspaceRef)
	validateOptionalRef(add, "deployment.resource_ref", r.Deployment.ResourceRef)
	validateOptionalRef(add, "deployment.deployment_name", r.Deployment.DeploymentName)

	if !protocolPattern.MatchString(string(r.Protocol.ID)) {
		add("protocol.id", "must be a stable lowercase protocol identifier")
	}
	if strings.ContainsAny(r.Protocol.APIVersion, "\r\n") {
		add("protocol.api_version", "must be a single line")
	}

	if err := r.Endpoint.Validate(r.Deployment); err != nil {
		var endpointError *EndpointValidationError
		if errors.As(err, &endpointError) {
			for _, field := range endpointError.Fields {
				add("endpoint."+field.Field, field.Problem)
			}
		} else {
			add("endpoint", err.Error())
		}
	}

	validateID(add, "credential.id", string(r.Credential.ID))
	if !r.Credential.Type.valid() {
		add("credential.type", "unsupported value")
	}
	if r.Credential.AuthPlacement != "" && !r.Credential.AuthPlacement.valid() {
		add("credential.auth_placement", "unsupported auth placement")
	}
	if r.Credential.Lifecycle != "" && !r.Credential.Lifecycle.valid() {
		add("credential.lifecycle", "unsupported credential lifecycle")
	}
	if r.Credential.Type != CredentialAnonymous {
		if r.Credential.AuthPlacement == "" {
			add("credential.auth_placement", "non-anonymous credentials require explicit auth placement")
		}
		if r.Credential.Lifecycle == "" {
			add("credential.lifecycle", "non-anonymous credentials require explicit lifecycle")
		}
		if len(r.Credential.AllowedProviderIDs) == 0 {
			add("credential.allowed_provider_ids", "non-anonymous credentials require an explicit provider binding")
		}
		if len(r.Credential.AllowedOfferingIDs) == 0 {
			add("credential.allowed_offering_ids", "non-anonymous credentials require an explicit offering binding")
		}
		if len(r.Credential.AllowedDeploymentIDs) == 0 {
			add("credential.allowed_deployment_ids", "non-anonymous credentials require an explicit deployment binding")
		}
		if r.Deployment.Region != "" && len(r.Credential.AllowedRegions) == 0 {
			add("credential.allowed_regions", "regional deployments require an explicit region binding")
		}
	}
	validateText(add, "credential.audience", r.Credential.Audience)
	if r.Credential.Type == CredentialAnonymous {
		if len(r.Credential.References) != 0 {
			add("credential.references", "anonymous credentials cannot reference secrets")
		}
	} else if len(r.Credential.References) == 0 {
		add("credential.references", "at least one secret reference is required")
	}
	seenReferences := make(map[string]struct{}, len(r.Credential.References))
	for index, reference := range r.Credential.References {
		prefix := fmt.Sprintf("credential.references[%d]", index)
		if reference.Purpose == "" {
			add(prefix+".purpose", "typed credential purpose is required")
		} else if reference.Purpose != "" && !reference.Purpose.valid() {
			add(prefix+".purpose", "unsupported credential purpose")
		}
		if !validReferencePart(reference.Store) {
			add(prefix+".store", "must identify a supported secret store")
		}
		if !refPattern.MatchString(reference.Name) {
			add(prefix+".name", "must be a secret identifier, not a secret value")
		}
		if reference.Store == "env" && !envNamePattern.MatchString(reference.Name) {
			add(prefix+".name", "environment secret references must use an environment variable name")
		}
		key := reference.Store + "\x00" + reference.Name
		if _, exists := seenReferences[key]; exists {
			add(prefix, "duplicates an earlier secret reference")
		}
		seenReferences[key] = struct{}{}
	}
	validateCredentialPurposes(add, r.Credential)
	validateCredentialAuth(add, r.Credential)
	validateUniqueReferences(add, "credential.scopes", r.Credential.Scopes, refPattern, "must be a stable scope")
	validateUniqueReferences(add, "credential.key_prefixes", r.Credential.KeyPrefixes, keyPrefixPattern, "must be a short non-secret key prefix")
	validateUniqueReferences(add, "credential.denied_key_prefixes", r.Credential.DeniedKeyPrefixes, keyPrefixPattern, "must be a short non-secret key prefix")
	validateCredentialBindings(add, r)
	if len(r.Credential.AllowedEndpointIDs) == 0 {
		add("credential.allowed_endpoint_ids", "at least one endpoint binding is required")
	}
	seenEndpoints := make(map[EndpointID]struct{}, len(r.Credential.AllowedEndpointIDs))
	endpointAllowed := false
	for index, endpointID := range r.Credential.AllowedEndpointIDs {
		validateID(add, fmt.Sprintf("credential.allowed_endpoint_ids[%d]", index), string(endpointID))
		if endpointID == r.Endpoint.ID {
			endpointAllowed = true
		}
		if _, exists := seenEndpoints[endpointID]; exists {
			add(fmt.Sprintf("credential.allowed_endpoint_ids[%d]", index), "duplicates an earlier endpoint binding")
		}
		seenEndpoints[endpointID] = struct{}{}
	}
	if r.Credential.Audience != "" && r.Endpoint.CredentialAudience != "" && r.Credential.Audience != r.Endpoint.CredentialAudience {
		add("credential.audience", "does not match endpoint credential audience")
	}
	if r.Endpoint.ID != "" && !endpointAllowed {
		add("credential.allowed_endpoint_ids", "does not permit the selected endpoint")
	}

	if len(fields) == 0 {
		return nil
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return &ValidationError{Fields: fields}
}

func validateID(add func(string, string), field, value string) {
	if !idPattern.MatchString(value) {
		add(field, "must be a stable lowercase identifier")
	}
}

func validateText(add func(string, string), field, value string) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
		add(field, "must be non-blank and single-line")
	}
}

func validateOptionalRef(add func(string, string), field, value string) {
	if value != "" && !refPattern.MatchString(value) {
		add(field, "must be a stable configuration reference")
	}
}

func validReferencePart(value string) bool {
	switch value {
	case "env", "vault", "keyring", "aws_secrets_manager", "gcp_secret_manager", "azure_key_vault", "workload_identity":
		return true
	default:
		return false
	}
}

func (k OfferingKind) valid() bool {
	switch k {
	case OfferingPayAsYouGo, OfferingTokenPlan, OfferingCodingPlan, OfferingProvisioned, OfferingDedicated, OfferingSelfHosted:
		return true
	default:
		return false
	}
}

func (u AllowedUsage) valid() bool {
	switch u {
	case AllowedUsageGeneralAPI, AllowedUsageInteractiveCodingOnly, AllowedUsageOfficialClientOnly:
		return true
	default:
		return false
	}
}

func (k DeploymentKind) valid() bool {
	switch k {
	case DeploymentDirect, DeploymentCloudServerless, DeploymentCloudProvisioned, DeploymentThirdParty, DeploymentSelfHosted:
		return true
	default:
		return false
	}
}

func (t CredentialType) valid() bool {
	switch t {
	case CredentialAPIKey, CredentialOAuth, CredentialADC, CredentialEntraID, CredentialSigV4, CredentialBearer, CredentialAnonymous:
		return true
	default:
		return false
	}
}

func (placement AuthPlacement) valid() bool {
	switch placement {
	case AuthPlacementHeader, AuthPlacementQuery, AuthPlacementSDK, AuthPlacementRequestSigning, AuthPlacementNone:
		return true
	default:
		return false
	}
}

func (lifecycle CredentialLifecycle) valid() bool {
	switch lifecycle {
	case CredentialLifecycleStatic, CredentialLifecycleRenewable, CredentialLifecycleShortLived, CredentialLifecycleWorkloadIdentity, CredentialLifecycleAnonymous:
		return true
	default:
		return false
	}
}

func (purpose CredentialPurpose) valid() bool {
	switch purpose {
	case CredentialPurposeAPIKey,
		CredentialPurposeBearerToken,
		CredentialPurposeClientID,
		CredentialPurposeClientSecret,
		CredentialPurposeTenantID,
		CredentialPurposeAccessKeyID,
		CredentialPurposeSecretAccessKey,
		CredentialPurposeSessionToken,
		CredentialPurposeProfile,
		CredentialPurposeWorkloadIdentity,
		CredentialPurposeCertificate:
		return true
	default:
		return false
	}
}

func validateCredentialPurposes(add func(string, string), credential CredentialProfile) {
	counts := make(map[CredentialPurpose]int, len(credential.References))
	for index, reference := range credential.References {
		purpose := reference.Purpose
		counts[purpose]++
		if purpose != "" && !credentialPurposeAllowed(credential.Type, purpose) {
			add(fmt.Sprintf("credential.references[%d].purpose", index), "purpose is not valid for this credential type")
		}
	}
	require := func(purpose CredentialPurpose) {
		if counts[purpose] == 0 {
			add("credential.references", fmt.Sprintf("credential type %q requires purpose %q", credential.Type, purpose))
		}
	}
	switch credential.Type {
	case CredentialAPIKey:
		require(CredentialPurposeAPIKey)
	case CredentialBearer:
		require(CredentialPurposeBearerToken)
	case CredentialOAuth:
		require(CredentialPurposeClientID)
		routes := boolCount(counts[CredentialPurposeClientSecret] > 0, counts[CredentialPurposeCertificate] > 0, counts[CredentialPurposeWorkloadIdentity] > 0)
		if routes != 1 {
			add("credential.references", "OAuth requires exactly one of client_secret, certificate, or workload_identity purpose")
		}
	case CredentialEntraID:
		workloadRoute := counts[CredentialPurposeWorkloadIdentity] > 0
		clientSecretRoute := counts[CredentialPurposeClientSecret] > 0
		certificateRoute := counts[CredentialPurposeCertificate] > 0
		if workloadRoute {
			if clientSecretRoute || certificateRoute {
				add("credential.references", "Entra workload_identity cannot be mixed with client_secret or certificate")
			}
		} else {
			require(CredentialPurposeClientID)
			require(CredentialPurposeTenantID)
			if clientSecretRoute == certificateRoute {
				add("credential.references", "Entra client credentials require exactly one of client_secret or certificate")
			}
		}
	case CredentialSigV4:
		profileRoute := counts[CredentialPurposeProfile] > 0
		workloadRoute := counts[CredentialPurposeWorkloadIdentity] > 0
		keyPairRoute := counts[CredentialPurposeAccessKeyID] > 0 && counts[CredentialPurposeSecretAccessKey] > 0
		if boolCount(profileRoute, workloadRoute, keyPairRoute) != 1 {
			add("credential.references", "SigV4 requires exactly one of profile, workload_identity, or access-key pair routes")
		}
		if counts[CredentialPurposeSessionToken] > 0 && !keyPairRoute {
			add("credential.references", "SigV4 session_token is valid only with an access-key pair")
		}
	case CredentialADC:
		if boolCount(counts[CredentialPurposeProfile] > 0, counts[CredentialPurposeWorkloadIdentity] > 0) != 1 {
			add("credential.references", "ADC requires exactly one of profile or workload_identity purpose")
		}
	}
	for purpose, count := range counts {
		if purpose != "" && count > 1 {
			add("credential.references", fmt.Sprintf("credential purpose %q is duplicated", purpose))
		}
	}
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func credentialPurposeAllowed(credentialType CredentialType, purpose CredentialPurpose) bool {
	switch credentialType {
	case CredentialAPIKey:
		return purpose == CredentialPurposeAPIKey
	case CredentialBearer:
		return purpose == CredentialPurposeBearerToken
	case CredentialOAuth:
		return purpose == CredentialPurposeClientID || purpose == CredentialPurposeClientSecret || purpose == CredentialPurposeCertificate || purpose == CredentialPurposeWorkloadIdentity || purpose == CredentialPurposeTenantID
	case CredentialEntraID:
		return purpose == CredentialPurposeClientID || purpose == CredentialPurposeClientSecret || purpose == CredentialPurposeCertificate || purpose == CredentialPurposeWorkloadIdentity || purpose == CredentialPurposeTenantID
	case CredentialSigV4:
		return purpose == CredentialPurposeAccessKeyID || purpose == CredentialPurposeSecretAccessKey || purpose == CredentialPurposeSessionToken || purpose == CredentialPurposeProfile || purpose == CredentialPurposeWorkloadIdentity
	case CredentialADC:
		return purpose == CredentialPurposeProfile || purpose == CredentialPurposeWorkloadIdentity
	case CredentialAnonymous:
		return false
	default:
		return false
	}
}

func validateCredentialAuth(add func(string, string), credential CredentialProfile) {
	if credential.AuthPlacement == "" {
		if credential.AuthHeader != "" || credential.AuthParameter != "" || credential.AuthScheme != "" || credential.SigV4Service != "" {
			add("credential.auth_placement", "auth metadata requires an explicit placement")
		}
	} else {
		switch credential.AuthPlacement {
		case AuthPlacementHeader:
			if !httpTokenPattern.MatchString(credential.AuthHeader) {
				add("credential.auth_header", "header placement requires a valid HTTP header name")
			}
			if credential.AuthParameter != "" {
				add("credential.auth_parameter", "header placement cannot use a query parameter")
			}
		case AuthPlacementQuery:
			if !httpTokenPattern.MatchString(credential.AuthParameter) {
				add("credential.auth_parameter", "query placement requires a valid parameter name")
			}
			if credential.AuthHeader != "" || credential.AuthScheme != "" {
				add("credential.auth_header", "query placement cannot declare header authentication")
			}
		case AuthPlacementSDK:
			if credential.AuthHeader != "" || credential.AuthParameter != "" || credential.AuthScheme != "" {
				add("credential.auth_placement", "SDK placement cannot declare wire placement fields")
			}
		case AuthPlacementRequestSigning:
			if credential.AuthHeader != "" || credential.AuthParameter != "" || credential.AuthScheme != "" {
				add("credential.auth_placement", "request signing cannot declare a static auth header or query parameter")
			}
		case AuthPlacementNone:
			if credential.Type != CredentialAnonymous {
				add("credential.auth_placement", "none placement is valid only for anonymous credentials")
			}
		}
	}
	if credential.AuthScheme != "" && !httpTokenPattern.MatchString(credential.AuthScheme) {
		add("credential.auth_scheme", "must be a valid HTTP auth scheme")
	}
	if credential.Type == CredentialSigV4 {
		if credential.AuthPlacement != AuthPlacementRequestSigning {
			add("credential.auth_placement", "SigV4 credentials require request-signing placement")
		}
		if !protocolPattern.MatchString(credential.SigV4Service) {
			add("credential.sigv4_service", "SigV4 credentials require a stable AWS service name")
		}
		if credential.Lifecycle == "" {
			add("credential.lifecycle", "SigV4 credentials require an explicit lifecycle")
		}
	} else if credential.SigV4Service != "" {
		add("credential.sigv4_service", "is valid only for SigV4 credentials")
	}
	if credential.Type == CredentialOAuth || credential.Type == CredentialEntraID {
		if len(credential.Scopes) == 0 {
			add("credential.scopes", "OAuth and Entra ID credentials require explicit scopes")
		}
	}
	if credential.Type == CredentialBearer && credential.AuthPlacement != "" {
		if credential.AuthPlacement != AuthPlacementHeader || !strings.EqualFold(credential.AuthScheme, "Bearer") {
			add("credential.auth_scheme", "bearer credentials require a Bearer header scheme")
		}
	}
	if credential.Type == CredentialAnonymous {
		if credential.Lifecycle != "" && credential.Lifecycle != CredentialLifecycleAnonymous {
			add("credential.lifecycle", "anonymous credentials require anonymous lifecycle")
		}
	} else if credential.Lifecycle == CredentialLifecycleAnonymous {
		add("credential.lifecycle", "anonymous lifecycle is valid only for anonymous credentials")
	}
}

func validateUniqueReferences(add func(string, string), field string, values []string, pattern *regexp.Regexp, problem string) {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		indexedField := fmt.Sprintf("%s[%d]", field, index)
		if !pattern.MatchString(value) {
			add(indexedField, problem)
		}
		if _, exists := seen[value]; exists {
			add(indexedField, "duplicates an earlier value")
		}
		seen[value] = struct{}{}
	}
}

func validateCredentialBindings(add func(string, string), route UpstreamRoute) {
	validateProviderBindings(add, route.Credential.AllowedProviderIDs, route.Provider)
	validateOfferingBindings(add, route.Credential.AllowedOfferingIDs, route.Offering.ID)
	validateDeploymentBindings(add, route.Credential.AllowedDeploymentIDs, route.Deployment.ID)
	validateRegionBindings(add, route.Credential.AllowedRegions, route.Deployment.Region)
}

func validateProviderBindings(add func(string, string), values []ProviderID, selected ProviderID) {
	seen := make(map[ProviderID]struct{}, len(values))
	matched := len(values) == 0
	for index, value := range values {
		field := fmt.Sprintf("credential.allowed_provider_ids[%d]", index)
		validateID(add, field, string(value))
		if value == selected {
			matched = true
		}
		if _, exists := seen[value]; exists {
			add(field, "duplicates an earlier provider binding")
		}
		seen[value] = struct{}{}
	}
	if !matched {
		add("credential.allowed_provider_ids", "does not permit the selected provider")
	}
}

func validateOfferingBindings(add func(string, string), values []OfferingID, selected OfferingID) {
	seen := make(map[OfferingID]struct{}, len(values))
	matched := len(values) == 0
	for index, value := range values {
		field := fmt.Sprintf("credential.allowed_offering_ids[%d]", index)
		validateID(add, field, string(value))
		if value == selected {
			matched = true
		}
		if _, exists := seen[value]; exists {
			add(field, "duplicates an earlier offering binding")
		}
		seen[value] = struct{}{}
	}
	if !matched {
		add("credential.allowed_offering_ids", "does not permit the selected offering")
	}
}

func validateDeploymentBindings(add func(string, string), values []DeploymentID, selected DeploymentID) {
	seen := make(map[DeploymentID]struct{}, len(values))
	matched := len(values) == 0
	for index, value := range values {
		field := fmt.Sprintf("credential.allowed_deployment_ids[%d]", index)
		validateID(add, field, string(value))
		if value == selected {
			matched = true
		}
		if _, exists := seen[value]; exists {
			add(field, "duplicates an earlier deployment binding")
		}
		seen[value] = struct{}{}
	}
	if !matched {
		add("credential.allowed_deployment_ids", "does not permit the selected deployment")
	}
}

func validateRegionBindings(add func(string, string), values []string, selected string) {
	seen := make(map[string]struct{}, len(values))
	matched := len(values) == 0
	for index, value := range values {
		field := fmt.Sprintf("credential.allowed_regions[%d]", index)
		if !refPattern.MatchString(value) {
			add(field, "must be a stable region binding")
		}
		if value == selected {
			matched = true
		}
		if _, exists := seen[value]; exists {
			add(field, "duplicates an earlier region binding")
		}
		seen[value] = struct{}{}
	}
	if !matched {
		add("credential.allowed_regions", "does not permit the selected region")
	}
}
