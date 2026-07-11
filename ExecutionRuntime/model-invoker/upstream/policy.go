package upstream

import (
	"fmt"
	"strings"
)

type SubjectKind string

const (
	SubjectPersonal SubjectKind = "personal"
	SubjectService  SubjectKind = "service"
)

type TenancyKind string

const (
	TenancySingle TenancyKind = "single_tenant"
	TenancyMulti  TenancyKind = "multi_tenant"
)

type ExecutionMode string

const (
	ExecutionForeground ExecutionMode = "foreground"
	ExecutionBatch      ExecutionMode = "batch"
	ExecutionBackground ExecutionMode = "background"
)

type ClientIdentitySource string

const (
	ClientIdentityRuntimeObserved ClientIdentitySource = "runtime_observed"
	ClientIdentityBuildManifest   ClientIdentitySource = "build_manifest"
)

// ClientIdentity records the real invoking client identity. Restricted
// offerings accept only runtime-observed or build-manifest identities; an
// arbitrary user-supplied claim has no valid source value.
type ClientIdentity struct {
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	UserAgent string               `json:"user_agent"`
	Source    ClientIdentitySource `json:"source"`
}

type InvocationContext struct {
	Explicit       bool            `json:"explicit"`
	Usage          InvocationUsage `json:"usage"`
	Subject        SubjectKind     `json:"subject"`
	Tenancy        TenancyKind     `json:"tenancy"`
	Execution      ExecutionMode   `json:"execution"`
	Production     bool            `json:"production"`
	ClientIdentity ClientIdentity  `json:"client_identity,omitempty"`
}

type SubjectPolicy string

const (
	SubjectPolicyAny          SubjectPolicy = "any"
	SubjectPolicyPersonalOnly SubjectPolicy = "personal_only"
	SubjectPolicyServiceOnly  SubjectPolicy = "service_only"
)

type TenancyPolicy string

const (
	TenancyPolicyAny              TenancyPolicy = "any"
	TenancyPolicySingleTenantOnly TenancyPolicy = "single_tenant_only"
)

type ExecutionPolicy string

const (
	ExecutionPolicyAny            ExecutionPolicy = "any"
	ExecutionPolicyForegroundOnly ExecutionPolicy = "foreground_only"
)

type ProductionPolicy string

const (
	ProductionPolicyAllowed   ProductionPolicy = "allowed"
	ProductionPolicyForbidden ProductionPolicy = "forbidden"
)

type PolicyReasonCode string

const (
	PolicyAllowed                  PolicyReasonCode = "allowed"
	PolicyInvalidEntitlement       PolicyReasonCode = "invalid_entitlement"
	PolicyInvalidUsageContext      PolicyReasonCode = "invalid_usage_context"
	PolicyInvalidSubject           PolicyReasonCode = "invalid_subject"
	PolicyInvalidTenancy           PolicyReasonCode = "invalid_tenancy"
	PolicyInvalidExecution         PolicyReasonCode = "invalid_execution"
	PolicyExplicitContextRequired  PolicyReasonCode = "explicit_context_required"
	PolicyUsageNotAllowed          PolicyReasonCode = "usage_not_allowed"
	PolicyOfficialClientOnly       PolicyReasonCode = "official_client_only"
	PolicyPersonalSubjectRequired  PolicyReasonCode = "personal_subject_required"
	PolicyServiceSubjectRequired   PolicyReasonCode = "service_subject_required"
	PolicySingleTenantRequired     PolicyReasonCode = "single_tenant_required"
	PolicyForegroundRequired       PolicyReasonCode = "foreground_required"
	PolicyProductionForbidden      PolicyReasonCode = "production_forbidden"
	PolicyClientIdentityRequired   PolicyReasonCode = "client_identity_required"
	PolicyInvalidClientIdentity    PolicyReasonCode = "invalid_client_identity"
	PolicyClientIdentityNotAllowed PolicyReasonCode = "client_identity_not_allowed"
)

type PolicyReason struct {
	Code    PolicyReasonCode `json:"code"`
	Field   string           `json:"field"`
	Message string           `json:"message"`
}

type PolicyDecision struct {
	Allowed                   bool             `json:"allowed"`
	Code                      PolicyReasonCode `json:"code"`
	Reasons                   []PolicyReason   `json:"reasons,omitempty"`
	AllowsAutomaticPAYGSwitch bool             `json:"allows_automatic_payg_switch"`
}

func (decision PolicyDecision) Clone() PolicyDecision {
	clone := decision
	clone.Reasons = append([]PolicyReason(nil), decision.Reasons...)
	return clone
}

// Decide applies the offering's static terms gate. It performs no quota,
// expiry or network check; those dynamic checks belong to the runtime.
func (entitlement CommercialEntitlement) Decide(context InvocationContext) PolicyDecision {
	reasons := validateInvocationContext(context)
	add := func(code PolicyReasonCode, field, message string) {
		reasons = append(reasons, PolicyReason{Code: code, Field: field, Message: message})
	}

	if !entitlement.SubjectPolicy.valid() || !entitlement.TenancyPolicy.valid() || !entitlement.ExecutionPolicy.valid() || !entitlement.ProductionPolicy.valid() || !entitlement.AllowedUsage.valid() {
		add(PolicyInvalidEntitlement, "entitlement", "offering has an invalid static policy")
	}
	if (entitlement.RequiresExplicitContext || entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly) && !context.Explicit {
		add(PolicyExplicitContextRequired, "explicit", "offering requires an explicitly declared invocation context")
	}
	switch entitlement.AllowedUsage {
	case AllowedUsageGeneralAPI:
		if context.Usage != InvocationGeneralAPI && context.Usage != InvocationInteractiveCoding {
			add(PolicyUsageNotAllowed, "usage", "offering does not permit this usage")
		}
	case AllowedUsageInteractiveCodingOnly:
		if context.Usage != InvocationInteractiveCoding {
			add(PolicyUsageNotAllowed, "usage", "offering is limited to interactive coding")
		}
	case AllowedUsageOfficialClientOnly:
		add(PolicyOfficialClientOnly, "usage", "Praxis cannot invoke an official-client-only offering")
	default:
		add(PolicyUsageNotAllowed, "usage", "offering has no valid allowed-usage policy")
	}

	subjectPolicy := entitlement.SubjectPolicy
	if subjectPolicy == "" && entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly {
		subjectPolicy = SubjectPolicyPersonalOnly
	}
	switch subjectPolicy {
	case "", SubjectPolicyAny:
	case SubjectPolicyPersonalOnly:
		if context.Subject != SubjectPersonal {
			add(PolicyPersonalSubjectRequired, "subject", "offering is limited to a personal subject")
		}
	case SubjectPolicyServiceOnly:
		if context.Subject != SubjectService {
			add(PolicyServiceSubjectRequired, "subject", "offering requires a service subject")
		}
	}

	tenancyPolicy := entitlement.TenancyPolicy
	if tenancyPolicy == "" && entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly {
		tenancyPolicy = TenancyPolicySingleTenantOnly
	}
	if tenancyPolicy == TenancyPolicySingleTenantOnly && context.Tenancy != TenancySingle {
		add(PolicySingleTenantRequired, "tenancy", "offering does not permit multi-tenant execution")
	}

	executionPolicy := entitlement.ExecutionPolicy
	if executionPolicy == "" && entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly {
		executionPolicy = ExecutionPolicyForegroundOnly
	}
	if executionPolicy == ExecutionPolicyForegroundOnly && context.Execution != ExecutionForeground {
		add(PolicyForegroundRequired, "execution", "offering permits foreground interaction only")
	}

	productionPolicy := entitlement.ProductionPolicy
	if productionPolicy == "" && entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly {
		productionPolicy = ProductionPolicyForbidden
	}
	if productionPolicy == ProductionPolicyForbidden && context.Production {
		add(PolicyProductionForbidden, "production", "offering does not permit production execution")
	}

	requiresIdentity := entitlement.RequiresClientIdentity || entitlement.AllowedUsage == AllowedUsageInteractiveCodingOnly || len(entitlement.AllowedClientNames) > 0
	if context.ClientIdentity == (ClientIdentity{}) {
		if requiresIdentity {
			add(PolicyClientIdentityRequired, "client_identity", "offering requires the real client identity")
		}
	} else if err := context.ClientIdentity.Validate(); err != nil {
		add(PolicyInvalidClientIdentity, "client_identity", err.Error())
	}
	if len(entitlement.AllowedClientNames) > 0 && context.ClientIdentity.Name != "" && !containsExact(entitlement.AllowedClientNames, context.ClientIdentity.Name) {
		add(PolicyClientIdentityNotAllowed, "client_identity.name", "client is not in the offering allowlist")
	}

	if len(reasons) == 0 {
		return PolicyDecision{Allowed: true, Code: PolicyAllowed, AllowsAutomaticPAYGSwitch: false}
	}
	return PolicyDecision{Allowed: false, Code: reasons[0].Code, Reasons: reasons, AllowsAutomaticPAYGSwitch: false}
}

func (identity ClientIdentity) Validate() error {
	var problems []string
	if !refPattern.MatchString(identity.Name) {
		problems = append(problems, "name must be a stable client identifier")
	}
	if !refPattern.MatchString(identity.Version) {
		problems = append(problems, "version must be a stable version identifier")
	}
	if strings.TrimSpace(identity.UserAgent) == "" || len(identity.UserAgent) > 512 || strings.ContainsAny(identity.UserAgent, "\r\n") {
		problems = append(problems, "user_agent must be non-blank, bounded, and single-line")
	} else {
		lowerUserAgent := strings.ToLower(identity.UserAgent)
		if !strings.Contains(lowerUserAgent, strings.ToLower(identity.Name)) || !strings.Contains(lowerUserAgent, strings.ToLower(identity.Version)) {
			problems = append(problems, "user_agent must contain the declared client name and version")
		}
	}
	if identity.Source != ClientIdentityRuntimeObserved && identity.Source != ClientIdentityBuildManifest {
		problems = append(problems, "source must attest a runtime-observed or build-manifest identity")
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid client identity: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateInvocationContext(context InvocationContext) []PolicyReason {
	var reasons []PolicyReason
	add := func(code PolicyReasonCode, field, message string) {
		reasons = append(reasons, PolicyReason{Code: code, Field: field, Message: message})
	}
	if context.Usage != InvocationGeneralAPI && context.Usage != InvocationInteractiveCoding {
		add(PolicyInvalidUsageContext, "usage", "unsupported invocation usage")
	}
	if context.Subject != SubjectPersonal && context.Subject != SubjectService {
		add(PolicyInvalidSubject, "subject", "unsupported subject kind")
	}
	if context.Tenancy != TenancySingle && context.Tenancy != TenancyMulti {
		add(PolicyInvalidTenancy, "tenancy", "unsupported tenancy kind")
	}
	if context.Execution != ExecutionForeground && context.Execution != ExecutionBatch && context.Execution != ExecutionBackground {
		add(PolicyInvalidExecution, "execution", "unsupported execution mode")
	}
	return reasons
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (policy SubjectPolicy) valid() bool {
	return policy == "" || policy == SubjectPolicyAny || policy == SubjectPolicyPersonalOnly || policy == SubjectPolicyServiceOnly
}

func (policy TenancyPolicy) valid() bool {
	return policy == "" || policy == TenancyPolicyAny || policy == TenancyPolicySingleTenantOnly
}

func (policy ExecutionPolicy) valid() bool {
	return policy == "" || policy == ExecutionPolicyAny || policy == ExecutionPolicyForegroundOnly
}

func (policy ProductionPolicy) valid() bool {
	return policy == "" || policy == ProductionPolicyAllowed || policy == ProductionPolicyForbidden
}
