package contract

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolRegistryObjectCurrentContractVersionV1 = "praxis.tool.registry-object-current/v1"

const MaxToolRegistryObjectCurrentTTLV1 = 15 * time.Second

const (
	ToolRegistryCapabilityCurrentKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/registry-capability-current"
	ToolRegistryDescriptorCurrentKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/registry-descriptor-current"
)

const toolRegistryObjectCurrentCanonicalDomainV1 = "praxis.tool"

type ToolRegistryObjectCurrentRefV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (r ToolRegistryObjectCurrentRefV1) Validate() error {
	if (r.Kind != ToolRegistryCapabilityCurrentKindV1 && r.Kind != ToolRegistryDescriptorCurrentKindV1) || ValidateStableID(r.ID) != nil || r.Revision == 0 {
		return invalid("Tool Registry object current Ref is invalid")
	}
	return r.Digest.Validate()
}

type ToolRegistryRecordSourceV1 struct {
	Kind             string        `json:"kind"`
	ID               string        `json:"id"`
	ObjectRevision   core.Revision `json:"object_revision"`
	ObjectDigest     core.Digest   `json:"object_digest"`
	State            string        `json:"state"`
	RegistryRevision core.Revision `json:"registry_revision"`
	UpdatedUnixNano  int64         `json:"updated_unix_nano"`
	Digest           core.Digest   `json:"digest"`
}

func (s ToolRegistryRecordSourceV1) Validate() error {
	if (s.Kind != "capability" && s.Kind != "tool") || !ValidObjectID(s.ID) || s.ObjectRevision == 0 || s.RegistryRevision == 0 || s.UpdatedUnixNano <= 0 || s.State != "active" {
		return invalid("Tool Registry record source is invalid or inactive")
	}
	if err := s.ObjectDigest.Validate(); err != nil {
		return err
	}
	if err := s.Digest.Validate(); err != nil {
		return err
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Tool Registry record source digest drifted")
	}
	return nil
}

func (s ToolRegistryRecordSourceV1) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(toolRegistryObjectCurrentCanonicalDomainV1, ToolRegistryObjectCurrentContractVersionV1, "ToolRegistryRecordSourceV1", s)
}

func SealToolRegistryRecordSourceV1(s ToolRegistryRecordSourceV1) (ToolRegistryRecordSourceV1, error) {
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return ToolRegistryRecordSourceV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolRegistryRecordSourceV1{}, conflict("supplied Tool Registry record source digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type ToolRegistryObjectCurrentProjectionV1 struct {
	ContractVersion  string                         `json:"contract_version"`
	Ref              ToolRegistryObjectCurrentRefV1 `json:"ref"`
	Source           ToolRegistryRecordSourceV1     `json:"source"`
	Object           ObjectRef                      `json:"object"`
	RegistryOwner    core.OwnerRef                  `json:"registry_owner"`
	CheckedUnixNano  int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                          `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                    `json:"projection_digest"`
}

func (p ToolRegistryObjectCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ToolRegistryObjectCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Source.Validate() != nil || p.Object.Validate() != nil || p.RegistryOwner.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return invalid("Tool Registry object current Projection is invalid")
	}
	expectedKind, err := toolRegistryCurrentKindV1(p.Source.Kind)
	if err != nil {
		return err
	}
	expectedID, err := DeriveToolRegistryObjectCurrentIDV1(expectedKind, p.Object, p.RegistryOwner)
	if err != nil {
		return err
	}
	if p.Ref.Kind != expectedKind || p.Ref.ID != expectedID || p.Ref.Revision != p.Source.RegistryRevision || p.Ref.Digest != p.Source.Digest || p.Source.ID != p.Object.ID || p.Source.ObjectRevision != p.Object.Revision || p.Source.ObjectDigest != p.Object.Digest {
		return conflict("Tool Registry object current repeated fields drifted")
	}
	if p.CheckedUnixNano < p.Source.UpdatedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxToolRegistryObjectCurrentTTLV1 {
		return conflict("Tool Registry object current time bounds drifted")
	}
	if err := p.ProjectionDigest.Validate(); err != nil {
		return err
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return conflict("Tool Registry object current Projection digest drifted")
	}
	return nil
}

func (p ToolRegistryObjectCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Registry object current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Registry object current expired")
	}
	return nil
}

func (p ToolRegistryObjectCurrentProjectionV1) ValidateAgainst(object ObjectRef, expected ToolRegistryObjectCurrentRefV1, now time.Time) error {
	if err := p.ValidateCurrent(now); err != nil {
		return err
	}
	if err := object.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Object != object || p.Ref != expected {
		return conflict("Tool Registry object current exact coordinates drifted")
	}
	return nil
}

func (p ToolRegistryObjectCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(toolRegistryObjectCurrentCanonicalDomainV1, ToolRegistryObjectCurrentContractVersionV1, "ToolRegistryObjectCurrentProjectionV1", p)
}

func SealToolRegistryObjectCurrentProjectionV1(p ToolRegistryObjectCurrentProjectionV1) (ToolRegistryObjectCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ToolRegistryObjectCurrentContractVersionV1 {
		return ToolRegistryObjectCurrentProjectionV1{}, invalid("Tool Registry object current contract version drifted")
	}
	p.ContractVersion = ToolRegistryObjectCurrentContractVersionV1
	if err := p.Source.Validate(); err != nil {
		return ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if err := p.Object.Validate(); err != nil {
		return ToolRegistryObjectCurrentProjectionV1{}, err
	}
	kind, err := toolRegistryCurrentKindV1(p.Source.Kind)
	if err != nil {
		return ToolRegistryObjectCurrentProjectionV1{}, err
	}
	id, err := DeriveToolRegistryObjectCurrentIDV1(kind, p.Object, p.RegistryOwner)
	if err != nil {
		return ToolRegistryObjectCurrentProjectionV1{}, err
	}
	expectedRef := ToolRegistryObjectCurrentRefV1{Kind: kind, ID: id, Revision: p.Source.RegistryRevision, Digest: p.Source.Digest}
	if p.Ref != (ToolRegistryObjectCurrentRefV1{}) && p.Ref != expectedRef {
		return ToolRegistryObjectCurrentProjectionV1{}, conflict("supplied Tool Registry object current Ref drifted")
	}
	p.Ref = expectedRef
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolRegistryObjectCurrentProjectionV1{}, conflict("supplied Tool Registry object current Projection digest drifted")
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func DeriveToolRegistryObjectCurrentIDV1(kind runtimeports.NamespacedNameV2, object ObjectRef, owner core.OwnerRef) (string, error) {
	if kind != ToolRegistryCapabilityCurrentKindV1 && kind != ToolRegistryDescriptorCurrentKindV1 {
		return "", invalid("Tool Registry object current Kind is invalid")
	}
	if err := object.Validate(); err != nil {
		return "", err
	}
	if err := owner.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(toolRegistryObjectCurrentCanonicalDomainV1, ToolRegistryObjectCurrentContractVersionV1, "ToolRegistryObjectCurrentIdentityV1", struct {
		Kind   runtimeports.NamespacedNameV2 `json:"kind"`
		Object ObjectRef                     `json:"object"`
		Owner  core.OwnerRef                 `json:"registry_owner"`
	}{Kind: kind, Object: object, Owner: owner})
	if err != nil {
		return "", err
	}
	return "tool-registry-current-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

type ToolRegistryObjectCurrentReaderV1 interface {
	ResolveExactToolCapabilityCurrentV1(context.Context, ObjectRef) (CapabilityDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
	InspectExactToolCapabilityCurrentV1(context.Context, ObjectRef, ToolRegistryObjectCurrentRefV1) (CapabilityDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
	ResolveExactToolDescriptorCurrentV1(context.Context, ObjectRef) (ToolDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
	InspectExactToolDescriptorCurrentV1(context.Context, ObjectRef, ToolRegistryObjectCurrentRefV1) (ToolDescriptor, ToolRegistryObjectCurrentProjectionV1, error)
}

func toolRegistryCurrentKindV1(sourceKind string) (runtimeports.NamespacedNameV2, error) {
	switch sourceKind {
	case "capability":
		return ToolRegistryCapabilityCurrentKindV1, nil
	case "tool":
		return ToolRegistryDescriptorCurrentKindV1, nil
	default:
		return "", invalid("Tool Registry record source Kind is unsupported")
	}
}
