package contract

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type NetworkMode string

const (
	NetworkDenyAll   NetworkMode = "deny_all"
	NetworkAllowList NetworkMode = "allow_list"
)

type NetworkRequirement struct {
	Mode    NetworkMode `json:"mode"`
	Targets []string    `json:"targets,omitempty"`
}

func (n NetworkRequirement) Validate() error {
	switch n.Mode {
	case NetworkDenyAll:
		if len(n.Targets) != 0 {
			return errors.New("deny_all network requirement cannot include targets")
		}
	case NetworkAllowList:
		if len(n.Targets) == 0 {
			return errors.New("allow_list network requirement needs targets")
		}
		if err := ValidateSortedUnique(n.Targets, "network targets"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported network mode %q", n.Mode)
	}
	return nil
}

type ResourceBounds struct {
	CPUUnits        uint64 `json:"cpu_units"`
	MemoryBytes     uint64 `json:"memory_bytes"`
	StorageBytes    uint64 `json:"storage_bytes"`
	PIDLimit        uint64 `json:"pid_limit"`
	WallTimeSeconds uint64 `json:"wall_time_seconds"`
}

func (r ResourceBounds) Validate() error {
	if r.CPUUnits == 0 || r.MemoryBytes == 0 || r.StorageBytes == 0 || r.PIDLimit == 0 || r.WallTimeSeconds == 0 {
		return errors.New("all resource bounds must be finite and positive")
	}
	return nil
}

type SecretRequirement struct {
	SecretRef     Ref    `json:"secret_ref"`
	Class         string `json:"class"`
	InjectionMode string `json:"injection_mode"`
	MaxTTLSeconds uint64 `json:"max_ttl_seconds"`
}

func (s SecretRequirement) Validate() error {
	if err := s.SecretRef.ValidateShape("secret ref"); err != nil {
		return err
	}
	if strings.TrimSpace(s.Class) == "" || strings.TrimSpace(s.InjectionMode) == "" || s.MaxTTLSeconds == 0 {
		return errors.New("secret class, injection mode, and finite ttl are required")
	}
	return nil
}

type RiskClass string

const (
	RiskLow       RiskClass = "low"
	RiskModerate  RiskClass = "moderate"
	RiskHigh      RiskClass = "high"
	RiskUntrusted RiskClass = "untrusted"
)

func (r RiskClass) Validate() error {
	switch r {
	case RiskLow, RiskModerate, RiskHigh, RiskUntrusted:
		return nil
	default:
		return fmt.Errorf("unsupported risk class %q", r)
	}
}

type ExecutionRequirement struct {
	Meta                 Meta                `json:"meta"`
	OSFamily             string              `json:"os_family"`
	Architecture         string              `json:"architecture"`
	ToolchainRefs        []Ref               `json:"toolchain_refs,omitempty"`
	ReadScopes           []string            `json:"read_scopes"`
	WriteScopes          []string            `json:"write_scopes"`
	Network              NetworkRequirement  `json:"network"`
	ProcessScopes        []string            `json:"process_scopes"`
	Secrets              []SecretRequirement `json:"secrets,omitempty"`
	Resources            ResourceBounds      `json:"resources"`
	RequiredCapabilities []BackendCapability `json:"required_capabilities"`
	ProhibitedResiduals  []string            `json:"prohibited_residuals,omitempty"`
	Risk                 RiskClass           `json:"risk"`
	AllowedSurfaces      []ExecutionSurface  `json:"allowed_surfaces"`
	AllowedDowngrades    []ExecutionSurface  `json:"allowed_downgrades,omitempty"`
}

func (r ExecutionRequirement) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(r.OSFamily) == "" || strings.TrimSpace(r.Architecture) == "" {
		return errors.New("os family and architecture are required")
	}
	for i, ref := range r.ToolchainRefs {
		if err := ref.ValidateShape(fmt.Sprintf("toolchain ref %d", i)); err != nil {
			return err
		}
	}
	if err := ValidateSortedUnique(r.ReadScopes, "read scopes"); err != nil {
		return err
	}
	if err := ValidateSortedUnique(r.WriteScopes, "write scopes"); err != nil {
		return err
	}
	if err := r.Network.Validate(); err != nil {
		return err
	}
	if err := ValidateSortedUnique(r.ProcessScopes, "process scopes"); err != nil {
		return err
	}
	for _, secret := range r.Secrets {
		if err := secret.Validate(); err != nil {
			return err
		}
	}
	if err := r.Resources.Validate(); err != nil {
		return err
	}
	if err := r.Risk.Validate(); err != nil {
		return err
	}
	if len(r.RequiredCapabilities) == 0 || len(r.AllowedSurfaces) == 0 {
		return errors.New("required capabilities and allowed surfaces are required")
	}
	if err := validateCapabilities(r.RequiredCapabilities); err != nil {
		return err
	}
	if err := validateSurfaces(r.AllowedSurfaces, "allowed surfaces"); err != nil {
		return err
	}
	if err := validateSurfaces(r.AllowedDowngrades, "allowed downgrades"); err != nil {
		return err
	}
	return ValidateSortedUnique(r.ProhibitedResiduals, "prohibited residuals")
}

func (r ExecutionRequirement) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	return r.Meta.ValidateCurrent(now)
}

type PolicyProjection struct {
	Meta                    Meta               `json:"meta"`
	RequirementRef          Ref                `json:"requirement_ref"`
	SourcePolicyRef         Ref                `json:"source_policy_ref"`
	AuthorityRef            Ref                `json:"authority_ref"`
	ReviewPolicyRef         Ref                `json:"review_policy_ref"`
	BudgetPolicyRef         Ref                `json:"budget_policy_ref"`
	ScopeDigest             string             `json:"scope_digest"`
	CapabilityGrantDigest   string             `json:"capability_grant_digest"`
	ReadScopes              []string           `json:"read_scopes"`
	WriteScopes             []string           `json:"write_scopes"`
	Network                 NetworkRequirement `json:"network"`
	ProcessScopes           []string           `json:"process_scopes"`
	SecretRefs              []Ref              `json:"secret_refs,omitempty"`
	Resources               ResourceBounds     `json:"resources"`
	MinimumConformance      ConformanceLevel   `json:"minimum_conformance"`
	AllowedResiduals        []string           `json:"allowed_residuals,omitempty"`
	ExternalEffectsDisabled bool               `json:"external_effects_disabled"`
}

func (p PolicyProjection) ValidateShape() error {
	if err := p.Meta.ValidateShape(); err != nil {
		return err
	}
	for name, ref := range map[string]Ref{
		"requirement":   p.RequirementRef,
		"source policy": p.SourcePolicyRef,
		"authority":     p.AuthorityRef,
		"review policy": p.ReviewPolicyRef,
		"budget policy": p.BudgetPolicyRef,
	} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	if !ValidDigest(p.ScopeDigest) || !ValidDigest(p.CapabilityGrantDigest) {
		return errors.New("scope and capability grant digests are required")
	}
	if err := ValidateSortedUnique(p.ReadScopes, "policy read scopes"); err != nil {
		return err
	}
	if err := ValidateSortedUnique(p.WriteScopes, "policy write scopes"); err != nil {
		return err
	}
	if err := p.Network.Validate(); err != nil {
		return err
	}
	if err := ValidateSortedUnique(p.ProcessScopes, "policy process scopes"); err != nil {
		return err
	}
	for _, ref := range p.SecretRefs {
		if err := ref.ValidateShape("policy secret ref"); err != nil {
			return err
		}
	}
	if err := p.Resources.Validate(); err != nil {
		return err
	}
	if err := p.MinimumConformance.Validate(); err != nil {
		return err
	}
	return ValidateSortedUnique(p.AllowedResiduals, "allowed residuals")
}

func (p PolicyProjection) ValidateCurrent(now time.Time) error {
	if err := p.ValidateShape(); err != nil {
		return err
	}
	return p.Meta.ValidateCurrent(now)
}
