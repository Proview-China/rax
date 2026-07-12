package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type IssueCode string

const (
	IssueInvalidSchema                 IssueCode = "invalid_schema"
	IssueInvalidValidationTime         IssueCode = "invalid_validation_time"
	IssueEmptyCatalog                  IssueCode = "empty_catalog"
	IssueDuplicateRouteID              IssueCode = "duplicate_route_id"
	IssueRouteIDMismatch               IssueCode = "route_id_mismatch"
	IssueInvalidRoute                  IssueCode = "invalid_route"
	IssueConflictingOfferingID         IssueCode = "conflicting_offering_id"
	IssueConflictingDeploymentID       IssueCode = "conflicting_deployment_id"
	IssueConflictingEndpointID         IssueCode = "conflicting_endpoint_id"
	IssueConflictingCredentialID       IssueCode = "conflicting_credential_id"
	IssueMissingSource                 IssueCode = "missing_source"
	IssueInvalidSource                 IssueCode = "invalid_source"
	IssueConflictingSource             IssueCode = "conflicting_source"
	IssueInvalidEvidenceStatus         IssueCode = "invalid_evidence_status"
	IssueInvalidEvidenceTTL            IssueCode = "invalid_evidence_ttl"
	IssueInvalidEvidenceWindow         IssueCode = "invalid_evidence_window"
	IssueEvidenceExpired               IssueCode = "evidence_expired"
	IssueInvalidEvidenceDigest         IssueCode = "invalid_evidence_digest"
	IssueInvalidImplementationStatus   IssueCode = "invalid_implementation_status"
	IssueInvalidHostActivation         IssueCode = "invalid_host_activation"
	IssueInvalidAdapterID              IssueCode = "invalid_adapter_id"
	IssueUnavailableEvidenceCallable   IssueCode = "unavailable_evidence_callable"
	IssueTermsBlockedCallable          IssueCode = "terms_blocked_callable"
	IssueUsageBlockedCallable          IssueCode = "usage_blocked_callable"
	IssueImplementationNotCallable     IssueCode = "implementation_not_callable"
	IssueMissingImplementationEvidence IssueCode = "missing_implementation_evidence"
	IssueMissingLiveEvidence           IssueCode = "missing_live_evidence"
	IssueInvalidSDKMetadata            IssueCode = "invalid_sdk_metadata"
	IssueInvalidCapabilityMetadata     IssueCode = "invalid_capability_metadata"
	IssueInvalidRouteMetadata          IssueCode = "invalid_route_metadata"
	IssueUnsafeArtifactPath            IssueCode = "unsafe_artifact_path"
	IssueMissingArtifact               IssueCode = "missing_artifact"
	IssueInvalidStateTransition        IssueCode = "invalid_state_transition"
)

type Issue struct {
	Code    IssueCode
	RouteID upstream.RouteID
	Field   string
	Message string
}

type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		location := issue.Field
		if issue.RouteID != "" {
			location = string(issue.RouteID) + "." + location
		}
		parts = append(parts, fmt.Sprintf("%s (%s): %s", location, issue.Code, issue.Message))
	}
	return "invalid upstream catalog: " + strings.Join(parts, "; ")
}

func (e *ValidationError) Has(code IssueCode) bool {
	if e == nil {
		return false
	}
	for _, issue := range e.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

var metadataIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)
var headerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)

func Validate(document Document, now time.Time) error {
	var issues []Issue
	add := func(code IssueCode, routeID upstream.RouteID, field, message string) {
		issues = append(issues, Issue{Code: code, RouteID: routeID, Field: field, Message: message})
	}
	if document.SchemaVersion != SchemaVersion {
		add(IssueInvalidSchema, "", "schema_version", "unsupported catalog schema")
	}
	if len(document.Entries) == 0 {
		add(IssueEmptyCatalog, "", "entries", "at least one route entry is required")
	}
	if now.IsZero() {
		add(IssueInvalidValidationTime, "", "now", "validation time must be explicit and non-zero")
	}

	seenRoutes := make(map[upstream.RouteID]struct{}, len(document.Entries))
	sourceIndex := make(map[string]OfficialSource)
	offeringIndex := make(map[upstream.OfferingID]upstream.Offering)
	deploymentIndex := make(map[upstream.DeploymentID]upstream.Deployment)
	endpointIndex := make(map[upstream.EndpointID]upstream.Endpoint)
	credentialIndex := make(map[upstream.CredentialProfileID]upstream.CredentialProfile)
	for index, entry := range document.Entries {
		routeID := entry.ID
		prefix := fmt.Sprintf("entries[%d]", index)
		if _, exists := seenRoutes[entry.ID]; exists {
			add(IssueDuplicateRouteID, routeID, prefix+".id", "route ID is duplicated")
		}
		seenRoutes[entry.ID] = struct{}{}
		if entry.ID != entry.Route.ID {
			add(IssueRouteIDMismatch, routeID, prefix+".route.id", "entry and route IDs differ")
		}
		if err := entry.Route.Validate(); err != nil {
			var routeError *upstream.ValidationError
			if errors.As(err, &routeError) {
				for _, field := range routeError.Fields {
					add(IssueInvalidRoute, routeID, prefix+".route."+field.Field, field.Problem)
				}
			} else {
				add(IssueInvalidRoute, routeID, prefix+".route", err.Error())
			}
		}
		validateSharedDefinitions(add, routeID, prefix, entry.Route, offeringIndex, deploymentIndex, endpointIndex, credentialIndex)
		validateSources(add, sourceIndex, routeID, prefix, entry.Sources)
		validateEvidence(add, routeID, prefix, entry, now)
		validateSDKs(add, routeID, prefix, entry.SDKs, entry.Implementation.Callable)
		validateCapabilities(add, routeID, prefix, entry.Capabilities)
		validateRouteMetadata(add, routeID, prefix, entry)
		validateImplementation(add, routeID, prefix, entry)
	}
	if len(issues) == 0 {
		return nil
	}
	sortIssues(issues)
	return &ValidationError{Issues: issues}
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].RouteID != issues[j].RouteID {
			return issues[i].RouteID < issues[j].RouteID
		}
		if issues[i].Field != issues[j].Field {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Code < issues[j].Code
	})
}

func validateSharedDefinitions(
	add func(IssueCode, upstream.RouteID, string, string),
	routeID upstream.RouteID,
	prefix string,
	route upstream.UpstreamRoute,
	offerings map[upstream.OfferingID]upstream.Offering,
	deployments map[upstream.DeploymentID]upstream.Deployment,
	endpoints map[upstream.EndpointID]upstream.Endpoint,
	credentials map[upstream.CredentialProfileID]upstream.CredentialProfile,
) {
	canonical := Entry{Route: route}
	canonicalizeEntry(&canonical)
	route = canonical.Route
	if route.Offering.ID != "" {
		if previous, exists := offerings[route.Offering.ID]; exists && !reflect.DeepEqual(previous, route.Offering) {
			add(IssueConflictingOfferingID, routeID, prefix+".route.offering.id", "offering ID has a conflicting definition in another route")
		} else if !exists {
			offerings[route.Offering.ID] = route.Offering
		}
	}
	if route.Deployment.ID != "" {
		if previous, exists := deployments[route.Deployment.ID]; exists && !reflect.DeepEqual(previous, route.Deployment) {
			add(IssueConflictingDeploymentID, routeID, prefix+".route.deployment.id", "deployment ID has a conflicting definition in another route")
		} else if !exists {
			deployments[route.Deployment.ID] = route.Deployment
		}
	}
	if route.Endpoint.ID != "" {
		if previous, exists := endpoints[route.Endpoint.ID]; exists && !reflect.DeepEqual(previous, route.Endpoint) {
			add(IssueConflictingEndpointID, routeID, prefix+".route.endpoint.id", "endpoint ID has a conflicting definition in another route")
		} else if !exists {
			endpoints[route.Endpoint.ID] = route.Endpoint
		}
	}
	if route.Credential.ID != "" {
		if previous, exists := credentials[route.Credential.ID]; exists && !reflect.DeepEqual(previous, route.Credential) {
			add(IssueConflictingCredentialID, routeID, prefix+".route.credential.id", "credential profile ID has a conflicting definition in another route")
		} else if !exists {
			credentials[route.Credential.ID] = route.Credential
		}
	}
}

func validateSources(add func(IssueCode, upstream.RouteID, string, string), sourceIndex map[string]OfficialSource, routeID upstream.RouteID, prefix string, sources []OfficialSource) {
	if len(sources) == 0 {
		add(IssueMissingSource, routeID, prefix+".official_sources", "at least one official source is required")
		return
	}
	seen := make(map[string]struct{}, len(sources))
	for index, source := range sources {
		field := fmt.Sprintf("%s.official_sources[%d]", prefix, index)
		if !metadataIDPattern.MatchString(source.ID) || strings.TrimSpace(source.Publisher) == "" || !source.Kind.valid() || !validOfficialURL(source.URL) {
			add(IssueInvalidSource, routeID, field, "source requires stable ID, publisher, kind, and HTTPS URL without credentials")
		}
		if _, exists := seen[source.ID]; exists {
			add(IssueInvalidSource, routeID, field+".id", "source ID is duplicated within route")
		}
		seen[source.ID] = struct{}{}
		if previous, exists := sourceIndex[source.ID]; exists && previous != source {
			add(IssueConflictingSource, routeID, field+".id", "source ID conflicts with another route")
		} else {
			sourceIndex[source.ID] = source
		}
	}
}

func validateEvidence(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, prefix string, entry Entry, now time.Time) {
	evidence := entry.Evidence
	if !evidence.Status.valid() {
		add(IssueInvalidEvidenceStatus, routeID, prefix+".evidence.status", "unsupported evidence status")
	}
	ttl, ttlValid := evidence.TTLClass.Duration()
	if !ttlValid {
		add(IssueInvalidEvidenceTTL, routeID, prefix+".evidence.ttl_class", "TTL class must be one of 7d, 14d, 30d, or 90d")
	}
	if evidence.CheckedAt.IsZero() || evidence.ValidUntil.IsZero() || !evidence.ValidUntil.After(evidence.CheckedAt) || (!now.IsZero() && evidence.CheckedAt.After(now)) {
		add(IssueInvalidEvidenceWindow, routeID, prefix+".evidence", "checked_at and valid_until must form a past-to-future validity window")
	}
	if ttlValid && !evidence.CheckedAt.IsZero() && !evidence.ValidUntil.Equal(evidence.CheckedAt.Add(ttl)) {
		add(IssueInvalidEvidenceTTL, routeID, prefix+".evidence.valid_until", "valid_until must equal checked_at plus the declared TTL class")
	}
	if !now.IsZero() && !evidence.ValidUntil.IsZero() && !now.Before(evidence.ValidUntil) {
		add(IssueEvidenceExpired, routeID, prefix+".evidence.valid_until", "official evidence is expired")
	}
	if evidence.Status == EvidenceInvalidated && strings.TrimSpace(evidence.InvalidatedBySourceID) == "" {
		add(IssueInvalidEvidenceStatus, routeID, prefix+".evidence.invalidated_by_source_id", "invalidated evidence requires a replacement source ID")
	} else if evidence.Status != EvidenceInvalidated && evidence.InvalidatedBySourceID != "" {
		add(IssueInvalidEvidenceStatus, routeID, prefix+".evidence.invalidated_by_source_id", "invalidating source ID is valid only for invalidated evidence")
	} else if evidence.InvalidatedBySourceID != "" && !containsSourceID(entry.Sources, evidence.InvalidatedBySourceID) {
		add(IssueInvalidEvidenceStatus, routeID, prefix+".evidence.invalidated_by_source_id", "invalidating source ID is not present in this route's official sources")
	}
	expectedDigest, err := ComputeEvidenceDigest(entry)
	if err != nil || evidence.Digest != expectedDigest {
		add(IssueInvalidEvidenceDigest, routeID, prefix+".evidence.digest", "digest does not match the canonical source-backed route assertions")
	}
	if entry.Implementation.Callable && evidence.Status != EvidenceFresh {
		code := IssueUnavailableEvidenceCallable
		if evidence.Status == EvidenceTermsBlocked {
			code = IssueTermsBlockedCallable
		}
		add(code, routeID, prefix+".implementation.callable", "only fresh evidence may be callable")
	}
}

func containsSourceID(sources []OfficialSource, id string) bool {
	for _, source := range sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func validateSDKs(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, prefix string, sdks []SDKMetadata, callable bool) {
	if len(sdks) == 0 {
		add(IssueInvalidSDKMetadata, routeID, prefix+".sdks", "at least one SDK or transport metadata record is required")
		return
	}
	seen := make(map[string]struct{}, len(sdks))
	for index, sdk := range sdks {
		field := fmt.Sprintf("%s.sdks[%d]", prefix, index)
		if strings.TrimSpace(sdk.Language) == "" || strings.TrimSpace(sdk.Package) == "" || strings.TrimSpace(sdk.Owner) == "" || strings.TrimSpace(sdk.Version) == "" || strings.TrimSpace(sdk.License) == "" || !sdk.Ownership.valid() || !sdk.Transport.valid() {
			add(IssueInvalidSDKMetadata, routeID, field, "language, package, owner, ownership, transport, version, and license are required")
		}
		if callable && (!sdk.Official || sdk.Ownership == SDKOwnershipCommunity) {
			add(IssueInvalidSDKMetadata, routeID, field, "callable routes cannot rely on unapproved community metadata")
		}
		key := sdk.Language + "\x00" + sdk.Package + "\x00" + string(sdk.Transport)
		if _, exists := seen[key]; exists {
			add(IssueInvalidSDKMetadata, routeID, field, "SDK metadata is duplicated")
		}
		seen[key] = struct{}{}
	}
}

func validateCapabilities(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, prefix string, capabilities []CapabilityMetadata) {
	if len(capabilities) == 0 {
		add(IssueInvalidCapabilityMetadata, routeID, prefix+".capabilities", "at least one capability is required")
		return
	}
	seen := make(map[string]struct{}, len(capabilities))
	for index, capability := range capabilities {
		field := fmt.Sprintf("%s.capabilities[%d]", prefix, index)
		if !metadataIDPattern.MatchString(capability.ID) || !capability.Support.valid() {
			add(IssueInvalidCapabilityMetadata, routeID, field, "capability requires a stable ID and valid support level")
		}
		if _, exists := seen[capability.ID]; exists {
			add(IssueInvalidCapabilityMetadata, routeID, field+".id", "capability ID is duplicated")
		}
		seen[capability.ID] = struct{}{}
		if (capability.Support == CapabilityPartial || capability.Support == CapabilityUnsupported || capability.Support == CapabilityUnknown) && len(capability.Limitations) == 0 {
			add(IssueInvalidCapabilityMetadata, routeID, field+".limitations", "partial, unsupported, and unknown capabilities require an explicit limitation")
		}
		for limitationIndex, limitation := range capability.Limitations {
			if strings.TrimSpace(limitation) == "" {
				add(IssueInvalidCapabilityMetadata, routeID, fmt.Sprintf("%s.limitations[%d]", field, limitationIndex), "limitation must not be blank")
			}
		}
	}
}

func validateRouteMetadata(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, prefix string, entry Entry) {
	if !entry.Maturity.valid() {
		add(IssueInvalidRouteMetadata, routeID, prefix+".maturity", "unsupported maturity")
	}
	if !entry.ModelDiscovery.Method.valid() {
		add(IssueInvalidRouteMetadata, routeID, prefix+".model_discovery.method", "unsupported model discovery method")
	}
	if !entry.ModelDiscovery.AliasPolicy.valid() {
		add(IssueInvalidRouteMetadata, routeID, prefix+".model_discovery.alias_policy", "unsupported model alias policy")
	}
	seenAliases := make(map[string]struct{}, len(entry.ModelDiscovery.Aliases))
	for index, alias := range entry.ModelDiscovery.Aliases {
		field := fmt.Sprintf("%s.model_discovery.aliases[%d]", prefix, index)
		if !metadataIDPattern.MatchString(alias.Alias) || strings.TrimSpace(alias.ProviderModelRef) == "" || strings.ContainsAny(alias.ProviderModelRef, "\r\n") {
			add(IssueInvalidRouteMetadata, routeID, field, "alias requires a stable ID and single-line provider model reference")
		}
		if _, exists := seenAliases[alias.Alias]; exists {
			add(IssueInvalidRouteMetadata, routeID, field+".alias", "model alias is duplicated")
		}
		seenAliases[alias.Alias] = struct{}{}
	}
	validateUniqueStrings(add, IssueInvalidRouteMetadata, routeID, prefix+".ignored_fields", entry.IgnoredFields, false)
	validateUniqueStrings(add, IssueInvalidRouteMetadata, routeID, prefix+".extension_fields", entry.ExtensionFields, false)
	validateUniqueStrings(add, IssueInvalidRouteMetadata, routeID, prefix+".stream_events", entry.StreamEvents, capabilityNeedsStreamEvents(entry.Capabilities))
	ignored := make(map[string]struct{}, len(entry.IgnoredFields))
	for _, field := range entry.IgnoredFields {
		ignored[field] = struct{}{}
	}
	for index, field := range entry.ExtensionFields {
		if _, exists := ignored[field]; exists {
			add(IssueInvalidRouteMetadata, routeID, fmt.Sprintf("%s.extension_fields[%d]", prefix, index), "field cannot be both ignored and an extension")
		}
	}
	if strings.TrimSpace(entry.ErrorDialect.Envelope) == "" || strings.ContainsAny(entry.ErrorDialect.Envelope, "\r\n") || strings.TrimSpace(entry.ErrorDialect.CodeField) == "" || strings.ContainsAny(entry.ErrorDialect.CodeField, "\r\n") {
		add(IssueInvalidRouteMetadata, routeID, prefix+".error_dialect", "error envelope and code field must be non-blank single-line identifiers")
	}
	validateHeaders(add, routeID, prefix+".error_dialect.request_id_headers", entry.ErrorDialect.RequestIDHeaders, true)
	validateHeaders(add, routeID, prefix+".error_dialect.retry_headers", entry.ErrorDialect.RetryHeaders, false)
	if !entry.Boundaries.Production.valid() || !entry.Boundaries.Quota.valid() || !entry.Boundaries.Expiry.valid() {
		add(IssueInvalidRouteMetadata, routeID, prefix+".boundaries", "production, quota, and expiry boundaries must be explicit")
	}
}

func capabilityNeedsStreamEvents(capabilities []CapabilityMetadata) bool {
	for _, capability := range capabilities {
		if capability.ID == "streaming" {
			return capability.Support == CapabilityNative || capability.Support == CapabilityCompatible || capability.Support == CapabilityPartial
		}
	}
	return false
}

func validateUniqueStrings(add func(IssueCode, upstream.RouteID, string, string), code IssueCode, routeID upstream.RouteID, field string, values []string, required bool) {
	if required && len(values) == 0 {
		add(code, routeID, field, "at least one value is required")
		return
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			add(code, routeID, fmt.Sprintf("%s[%d]", field, index), "value must be non-blank and single-line")
		}
		if _, exists := seen[value]; exists {
			add(code, routeID, fmt.Sprintf("%s[%d]", field, index), "value is duplicated")
		}
		seen[value] = struct{}{}
	}
}

func validateHeaders(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, field string, values []string, required bool) {
	if required && len(values) == 0 {
		add(IssueInvalidRouteMetadata, routeID, field, "at least one header is required")
		return
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value != strings.ToLower(value) || !headerNamePattern.MatchString(value) {
			add(IssueInvalidRouteMetadata, routeID, fmt.Sprintf("%s[%d]", field, index), "header must be a lowercase HTTP field name")
		}
		if _, exists := seen[value]; exists {
			add(IssueInvalidRouteMetadata, routeID, fmt.Sprintf("%s[%d]", field, index), "header is duplicated")
		}
		seen[value] = struct{}{}
	}
}

func validateImplementation(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, prefix string, entry Entry) {
	implementation := entry.Implementation
	if !implementation.Status.valid() {
		add(IssueInvalidImplementationStatus, routeID, prefix+".implementation.status", "unsupported implementation status")
		return
	}
	rank := implementation.Status.rank()
	if implementation.AdapterID != "" && !metadataIDPattern.MatchString(implementation.AdapterID) {
		add(IssueInvalidAdapterID, routeID, prefix+".implementation.adapter_id", "adapter ID must be a stable runtime registry identifier")
	}
	if implementation.Callable && rank < ImplementationImplementedOffline.rank() {
		add(IssueImplementationNotCallable, routeID, prefix+".implementation.callable", "research, design, and plan records cannot be callable")
	}
	if implementation.Callable && entry.Route.Offering.Entitlement.AllowedUsage == upstream.AllowedUsageOfficialClientOnly {
		add(IssueUsageBlockedCallable, routeID, prefix+".implementation.callable", "official-client-only offering cannot be called by Praxis")
	}
	if implementation.HostActivationRequirement != "" {
		if implementation.HostActivationRequirement != HostActivationTrustedSubscriptionAuthorizationResolver {
			add(IssueInvalidHostActivation, routeID, prefix+".implementation.host_activation_requirement", "unsupported host activation requirement")
		}
		if implementation.Callable {
			add(IssueInvalidHostActivation, routeID, prefix+".implementation.callable", "a host-blocked route cannot be callable")
		}
		if entry.Route.Offering.Kind != upstream.OfferingTokenPlan && entry.Route.Offering.Kind != upstream.OfferingCodingPlan {
			add(IssueInvalidHostActivation, routeID, prefix+".implementation.host_activation_requirement", "trusted subscription authorization applies only to subscription offerings")
		}
		if rank < ImplementationImplementedOffline.rank() || implementation.AdapterID == "" {
			add(IssueInvalidHostActivation, routeID, prefix+".implementation", "a host-blocked route must retain its implemented adapter and offline evidence")
		}
	}
	if rank >= ImplementationImplementedOffline.rank() && (len(implementation.CodePaths) == 0 || len(implementation.TestEvidence) == 0) {
		add(IssueMissingImplementationEvidence, routeID, prefix+".implementation", "offline implementation requires code paths and test evidence")
	}
	if rank >= ImplementationImplementedOffline.rank() && implementation.AdapterID == "" {
		add(IssueInvalidAdapterID, routeID, prefix+".implementation.adapter_id", "offline implementation requires an explicit runtime adapter ID")
	}
	validateArtifactPaths(add, routeID, prefix+".implementation.code_paths", implementation.CodePaths)
	validateArtifactPaths(add, routeID, prefix+".implementation.test_evidence", implementation.TestEvidence)
	if rank >= ImplementationLiveVerified.rank() && len(implementation.LiveEvidence) == 0 {
		add(IssueMissingLiveEvidence, routeID, prefix+".implementation.live_evidence", "live verification requires route-specific smoke evidence")
	}
}

func validateArtifactPaths(add func(IssueCode, upstream.RouteID, string, string), routeID upstream.RouteID, field string, values []string) {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		pathField := fmt.Sprintf("%s[%d]", field, index)
		if !safeArtifactPath(value) {
			add(IssueUnsafeArtifactPath, routeID, pathField, "artifact path must be a clean, safe, module-relative slash path")
		}
		if _, exists := seen[value]; exists {
			add(IssueUnsafeArtifactPath, routeID, pathField, "artifact path is duplicated")
		}
		seen[value] = struct{}{}
	}
}

func safeArtifactPath(value string) bool {
	return value != "" && len(value) <= 512 && value == strings.TrimSpace(value) &&
		!strings.ContainsAny(value, "\\:\x00\r\n") &&
		fs.ValidPath(value) && path.Clean(value) == value && value != "."
}

// ValidateArtifacts verifies that every declared implementation and test
// artifact exists under an injected module-root filesystem.
func ValidateArtifacts(filesystem fs.FS, document Document) error {
	var issues []Issue
	add := func(code IssueCode, routeID upstream.RouteID, field, message string) {
		issues = append(issues, Issue{Code: code, RouteID: routeID, Field: field, Message: message})
	}
	if filesystem == nil {
		add(IssueMissingArtifact, "", "filesystem", "artifact filesystem is nil")
		return &ValidationError{Issues: issues}
	}
	for entryIndex, entry := range document.Entries {
		groups := []struct {
			name  string
			paths []string
		}{
			{name: "code_paths", paths: entry.Implementation.CodePaths},
			{name: "test_evidence", paths: entry.Implementation.TestEvidence},
		}
		for _, group := range groups {
			for pathIndex, artifact := range group.paths {
				field := fmt.Sprintf("entries[%d].implementation.%s[%d]", entryIndex, group.name, pathIndex)
				if !safeArtifactPath(artifact) {
					add(IssueUnsafeArtifactPath, entry.ID, field, "artifact path is unsafe")
					continue
				}
				if _, err := fs.Stat(filesystem, artifact); err != nil {
					add(IssueMissingArtifact, entry.ID, field, "artifact does not exist")
				}
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	sortIssues(issues)
	return &ValidationError{Issues: issues}
}

func validOfficialURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

// ValidateTransition rejects illegal evidence or implementation state jumps.
func ValidateTransition(previous, next Entry, now time.Time) error {
	var issues []Issue
	if previous.ID != next.ID || previous.Route.ID != next.Route.ID {
		issues = append(issues, Issue{Code: IssueInvalidStateTransition, RouteID: next.ID, Field: "id", Message: "route identity cannot change during a state transition"})
	}
	identityDimensions := []struct {
		field    string
		previous any
		next     any
	}{
		{field: "route.model", previous: previous.Route.Model, next: next.Route.Model},
		{field: "route.provider", previous: previous.Route.Provider, next: next.Route.Provider},
		{field: "route.offering", previous: previous.Route.Offering, next: next.Route.Offering},
		{field: "route.deployment", previous: previous.Route.Deployment, next: next.Route.Deployment},
		{field: "route.protocol", previous: previous.Route.Protocol, next: next.Route.Protocol},
		{field: "route.endpoint", previous: previous.Route.Endpoint, next: next.Route.Endpoint},
		{field: "route.credential", previous: previous.Route.Credential, next: next.Route.Credential},
	}
	for _, dimension := range identityDimensions {
		if !reflect.DeepEqual(dimension.previous, dimension.next) {
			issues = append(issues, Issue{Code: IssueInvalidStateTransition, RouteID: next.ID, Field: dimension.field, Message: "seven-dimensional route identity is immutable"})
		}
	}
	if previous.Implementation.AdapterID != next.Implementation.AdapterID {
		issues = append(issues, Issue{Code: IssueInvalidStateTransition, RouteID: next.ID, Field: "implementation.adapter_id", Message: "runtime adapter identity is immutable"})
	}
	if !validEvidenceTransition(previous.Evidence.Status, next.Evidence.Status) {
		issues = append(issues, Issue{Code: IssueInvalidStateTransition, RouteID: next.ID, Field: "evidence.status", Message: "illegal evidence status transition"})
	}
	if !validImplementationTransition(previous.Implementation.Status, next.Implementation.Status) {
		issues = append(issues, Issue{Code: IssueInvalidStateTransition, RouteID: next.ID, Field: "implementation.status", Message: "implementation status must advance one reviewed stage at a time"})
	}
	if err := Validate(Document{SchemaVersion: SchemaVersion, Entries: []Entry{next}}, now); err != nil {
		var validationError *ValidationError
		if errors.As(err, &validationError) {
			issues = append(issues, validationError.Issues...)
		}
	}
	if len(issues) == 0 {
		return nil
	}
	sortIssues(issues)
	return &ValidationError{Issues: issues}
}

func validEvidenceTransition(from, to EvidenceStatus) bool {
	if !from.valid() || !to.valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case EvidenceUnverified:
		return to == EvidenceFresh || to == EvidenceTermsBlocked || to == EvidenceInvalidated || to == EvidenceDeprecated
	case EvidenceFresh:
		return to == EvidenceStale || to == EvidenceInvalidated || to == EvidenceTermsBlocked || to == EvidenceDeprecated
	case EvidenceStale:
		return to == EvidenceFresh || to == EvidenceInvalidated || to == EvidenceTermsBlocked || to == EvidenceDeprecated
	case EvidenceInvalidated, EvidenceTermsBlocked:
		return to == EvidenceUnverified || to == EvidenceFresh || to == EvidenceDeprecated
	case EvidenceDeprecated:
		return false
	default:
		return false
	}
}

func validImplementationTransition(from, to ImplementationStatus) bool {
	if !from.valid() || !to.valid() {
		return false
	}
	return from == to || to.rank() == from.rank()+1
}

func (status EvidenceStatus) valid() bool {
	switch status {
	case EvidenceFresh, EvidenceStale, EvidenceInvalidated, EvidenceUnverified, EvidenceTermsBlocked, EvidenceDeprecated:
		return true
	default:
		return false
	}
}

func (status ImplementationStatus) valid() bool { return status.rank() >= 0 }

func (status ImplementationStatus) rank() int {
	switch status {
	case ImplementationResearchOnly:
		return 0
	case ImplementationDesigned:
		return 1
	case ImplementationPlanned:
		return 2
	case ImplementationImplementedOffline:
		return 3
	case ImplementationLiveVerified:
		return 4
	case ImplementationProductionApproved:
		return 5
	default:
		return -1
	}
}

func (kind SourceKind) valid() bool {
	switch kind {
	case SourceAPIReference, SourceSDK, SourceTerms, SourceProductDocs, SourceModelCatalog:
		return true
	default:
		return false
	}
}

func (support CapabilitySupport) valid() bool {
	switch support {
	case CapabilityNative, CapabilityCompatible, CapabilityPartial, CapabilityUnsupported, CapabilityUnknown:
		return true
	default:
		return false
	}
}

func (ownership SDKOwnership) valid() bool {
	switch ownership {
	case SDKOwnershipProviderNative, SDKOwnershipModelVendor, SDKOwnershipProtocolUpstream, SDKOwnershipCloudNative, SDKOwnershipCommunity:
		return true
	default:
		return false
	}
}

func (kind TransportKind) valid() bool {
	switch kind {
	case TransportSDK, TransportHTTP, TransportSSE, TransportWebSocket, TransportGRPC, TransportSidecar:
		return true
	default:
		return false
	}
}

func (maturity Maturity) valid() bool {
	switch maturity {
	case MaturityGA, MaturityPreview, MaturityExperimental, MaturityUnknown:
		return true
	default:
		return false
	}
}

func (method ModelDiscoveryMethod) valid() bool {
	switch method {
	case ModelDiscoveryRuntimeSelected, ModelDiscoveryStaticCatalog, ModelDiscoveryProviderAPI:
		return true
	default:
		return false
	}
}

func (policy ModelAliasPolicy) valid() bool {
	switch policy {
	case ModelAliasExactProviderID, ModelAliasStable, ModelAliasProviderManaged:
		return true
	default:
		return false
	}
}

func (boundary ProductionBoundary) valid() bool {
	switch boundary {
	case ProductionRequiresReview, ProductionAllowed, ProductionProhibited, ProductionUnknown:
		return true
	default:
		return false
	}
}

func (boundary QuotaBoundary) valid() bool {
	switch boundary {
	case QuotaProviderAccount, QuotaSubscriptionWindow, QuotaProvisionedCapacity, QuotaSelfHosted, QuotaUnknown:
		return true
	default:
		return false
	}
}

func (boundary ExpiryBoundary) valid() bool {
	switch boundary {
	case ExpiryCredentialOrAccount, ExpirySubscriptionPeriod, ExpiryNone, ExpiryUnknown:
		return true
	default:
		return false
	}
}
