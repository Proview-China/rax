package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ResourceBindingContractVersionV1 = "praxis.runtime.resource-binding/v1"
	resourceHandleCanonicalDomainV1  = "praxis.runtime.resource-handle-current"
	resourceBindingCanonicalDomainV1 = "praxis.runtime.resource-binding-set"
	MaxResourceBindingsV1            = 256
)

type ResourceHandleKindV1 NamespacedNameV2

func (k ResourceHandleKindV1) Validate() error {
	return ValidateNamespacedNameV2(NamespacedNameV2(k))
}

type ResourceHandleRefV1 struct {
	Owner           core.OwnerRef        `json:"owner"`
	ID              string               `json:"id"`
	Revision        core.Revision        `json:"revision"`
	Digest          core.Digest          `json:"digest"`
	Kind            ResourceHandleKindV1 `json:"kind"`
	ScopeDigest     core.Digest          `json:"scope_digest"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
}

func (r ResourceHandleRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if invalidH4IDV1(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 || r.Kind.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource handle exact Ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.ScopeDigest.Validate()
}

type ResourceHandleCurrentV1 struct {
	ContractVersion       string              `json:"contract_version"`
	Ref                   ResourceHandleRefV1 `json:"ref"`
	CleanupContract       OwnerCurrentRefV1   `json:"cleanup_contract"`
	DeploymentAttestation OwnerCurrentRefV1   `json:"deployment_attestation"`
	CheckedUnixNano       int64               `json:"checked_unix_nano"`
	ExpiresUnixNano       int64               `json:"expires_unix_nano"`
	ProjectionDigest      core.Digest         `json:"projection_digest"`
}

func (p ResourceHandleCurrentV1) Validate() error {
	if p.ContractVersion != ResourceBindingContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource handle current projection is incomplete")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.CleanupContract.Validate(); err != nil {
		return err
	}
	if err := p.DeploymentAttestation.Validate(); err != nil {
		return err
	}
	minimum := p.CleanupContract.ExpiresUnixNano
	if p.DeploymentAttestation.ExpiresUnixNano < minimum {
		minimum = p.DeploymentAttestation.ExpiresUnixNano
	}
	if p.Ref.ExpiresUnixNano != p.ExpiresUnixNano || p.ExpiresUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Resource handle TTL exceeds its cleanup or deployment proof")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource handle current digest drifted")
	}
	return nil
}

func (p ResourceHandleCurrentV1) ValidateCurrent(expected ResourceHandleRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource handle current Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Resource handle current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Resource handle current expired")
	}
	return nil
}

func (p ResourceHandleCurrentV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(resourceHandleCanonicalDomainV1, ResourceBindingContractVersionV1, "ResourceHandleCurrentV1", copy)
}

func SealResourceHandleCurrentV1(p ResourceHandleCurrentV1) (ResourceHandleCurrentV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ResourceBindingContractVersionV1 {
		return ResourceHandleCurrentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource handle contract version drifted")
	}
	p.ContractVersion = ResourceBindingContractVersionV1
	p.Ref.ExpiresUnixNano = p.ExpiresUnixNano
	providedRefDigest := p.Ref.Digest
	providedProjectionDigest := p.ProjectionDigest
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ResourceHandleCurrentV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedProjectionDigest != "" && providedProjectionDigest != digest) {
		return ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Resource handle current supplied a wrong digest")
	}
	p.Ref.Digest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ResourceBindingV1 struct {
	ComponentID           ComponentIDV2       `json:"component_id"`
	Handle                ResourceHandleRefV1 `json:"handle"`
	CleanupContract       OwnerCurrentRefV1   `json:"cleanup_contract"`
	DeploymentAttestation OwnerCurrentRefV1   `json:"deployment_attestation"`
	BindingDigest         core.Digest         `json:"binding_digest"`
}

func (b ResourceBindingV1) Validate() error {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(b.ComponentID)); err != nil {
		return err
	}
	if err := b.Handle.Validate(); err != nil {
		return err
	}
	if err := b.CleanupContract.Validate(); err != nil {
		return err
	}
	if err := b.DeploymentAttestation.Validate(); err != nil {
		return err
	}
	digest, err := digestResourceBindingV1(b)
	if err != nil || digest != b.BindingDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource binding digest drifted")
	}
	return nil
}

func digestResourceBindingV1(b ResourceBindingV1) (core.Digest, error) {
	copy := b
	copy.BindingDigest = ""
	return core.CanonicalJSONDigest(resourceBindingCanonicalDomainV1, ResourceBindingContractVersionV1, "ResourceBindingV1", copy)
}

type ResourceBindingSetRefV1 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r ResourceBindingSetRefV1) Validate() error {
	if invalidH4IDV1(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource BindingSet Ref is incomplete")
	}
	return r.Digest.Validate()
}

type ResourceBindingSetV1 struct {
	ContractVersion  string                  `json:"contract_version"`
	Ref              ResourceBindingSetRefV1 `json:"ref"`
	Bindings         []ResourceBindingV1     `json:"bindings"`
	CheckedUnixNano  int64                   `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                   `json:"expires_unix_nano"`
	ProjectionDigest core.Digest             `json:"projection_digest"`
}

func (s ResourceBindingSetV1) Validate() error {
	if s.ContractVersion != ResourceBindingContractVersionV1 || s.CheckedUnixNano <= 0 || s.ExpiresUnixNano <= s.CheckedUnixNano || len(s.Bindings) == 0 || len(s.Bindings) > MaxResourceBindingsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource BindingSet is incomplete")
	}
	if err := s.Ref.Validate(); err != nil {
		return err
	}
	minimum := int64(0)
	var previous string
	for index, binding := range s.Bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		key := string(binding.ComponentID) + "\x00" + binding.Handle.ID
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Resource bindings must be sorted and unique")
		}
		previous = key
		if minimum == 0 || binding.Handle.ExpiresUnixNano < minimum {
			minimum = binding.Handle.ExpiresUnixNano
		}
	}
	if s.Ref.ExpiresUnixNano != s.ExpiresUnixNano || s.ExpiresUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Resource BindingSet TTL exceeds a member current")
	}
	digest, err := s.DigestV1()
	if err != nil || digest != s.Ref.Digest || digest != s.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet digest drifted")
	}
	return nil
}

func (s ResourceBindingSetV1) ValidateCurrent(expected ResourceBindingSetRefV1, now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < s.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Resource BindingSet clock regressed")
	}
	if !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Resource BindingSet expired")
	}
	return nil
}

func (s ResourceBindingSetV1) DigestV1() (core.Digest, error) {
	copy := s
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	if copy.Bindings == nil {
		copy.Bindings = []ResourceBindingV1{}
	}
	return core.CanonicalJSONDigest(resourceBindingCanonicalDomainV1, ResourceBindingContractVersionV1, "ResourceBindingSetV1", copy)
}

func SealResourceBindingSetV1(s ResourceBindingSetV1) (ResourceBindingSetV1, error) {
	if s.ContractVersion != "" && s.ContractVersion != ResourceBindingContractVersionV1 {
		return ResourceBindingSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource BindingSet contract version drifted")
	}
	s.ContractVersion = ResourceBindingContractVersionV1
	s.Bindings = append([]ResourceBindingV1{}, s.Bindings...)
	for index := range s.Bindings {
		digest, err := digestResourceBindingV1(s.Bindings[index])
		if err != nil {
			return ResourceBindingSetV1{}, err
		}
		if s.Bindings[index].BindingDigest != "" && s.Bindings[index].BindingDigest != digest {
			return ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource binding supplied a wrong digest")
		}
		s.Bindings[index].BindingDigest = digest
	}
	sort.Slice(s.Bindings, func(i, j int) bool {
		left := string(s.Bindings[i].ComponentID) + "\x00" + s.Bindings[i].Handle.ID
		right := string(s.Bindings[j].ComponentID) + "\x00" + s.Bindings[j].Handle.ID
		return left < right
	})
	s.Ref.ExpiresUnixNano = s.ExpiresUnixNano
	providedRefDigest := s.Ref.Digest
	providedProjectionDigest := s.ProjectionDigest
	s.Ref.Digest = ""
	s.ProjectionDigest = ""
	digest, err := s.DigestV1()
	if err != nil {
		return ResourceBindingSetV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedProjectionDigest != "" && providedProjectionDigest != digest) {
		return ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Resource BindingSet supplied a wrong digest")
	}
	s.Ref.Digest = digest
	s.ProjectionDigest = digest
	return s, s.Validate()
}

type ResourceCurrentReaderV1 interface {
	InspectResourceHandleCurrentV1(context.Context, ResourceHandleRefV1) (ResourceHandleCurrentV1, error)
	InspectResourceBindingSetCurrentV1(context.Context, ResourceBindingSetRefV1) (ResourceBindingSetV1, error)
}

// ResourceOwnerRepositoryV1 is the additive write authority for persisting
// resource-owner public references. It never creates resources and never
// exposes OS handles, credentials or provider secrets.
type ResourceOwnerRepositoryV1 interface {
	ResourceCurrentReaderV1
	EnsureResourceHandleCurrentV1(context.Context, ResourceHandleCurrentV1) (ResourceHandleCurrentV1, error)
	EnsureResourceBindingSetCurrentV1(context.Context, ResourceBindingSetV1) (ResourceBindingSetV1, error)
}
