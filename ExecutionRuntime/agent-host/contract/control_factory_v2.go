package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ControlAdapterFactoryContractVersionV2 = "praxis.agent-host/control-adapter-factory/v2"
	ControlAdapterEffectNoneV2             = "none"
	MaxControlAdapterResourcesV2           = 64
	MaxControlAdapterOutputsV2             = 64
)

type ControlAdapterFactoryRefV2 struct {
	FactoryID string        `json:"factory_id"`
	Revision  core.Revision `json:"revision"`
	Digest    core.Digest   `json:"digest"`
}

func (r ControlAdapterFactoryRefV2) Validate() error {
	if err := ValidateIdentifierV1("control adapter factory id", r.FactoryID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return NewError(ErrorInvalidArgument, "control_adapter_factory_revision_invalid", "control adapter factory revision must be positive")
	}
	if err := r.Digest.Validate(); err != nil {
		return NewError(ErrorInvalidArgument, "control_adapter_factory_digest_invalid", err.Error())
	}
	return nil
}

type ControlAdapterFactoryDescriptorV2 struct {
	ContractVersion        string                                    `json:"contract_version"`
	Ref                    ControlAdapterFactoryRefV2                `json:"ref"`
	ComponentID            runtimeports.ComponentIDV2                `json:"component_id"`
	ArtifactDigest         core.Digest                               `json:"artifact_digest"`
	ComponentContract      string                                    `json:"component_contract"`
	Capability             runtimeports.CapabilityNameV2             `json:"capability"`
	Binding                runtimeports.BindingAdmissionBindingRefV1 `json:"binding"`
	Generation             runtimeports.OwnerCurrentRefV1            `json:"generation"`
	ResourceBindingSet     runtimeports.ResourceBindingSetRefV1      `json:"resource_binding_set"`
	ResourceHandles        []runtimeports.ResourceHandleRefV1        `json:"resource_handles"`
	OutputPortCapabilities []runtimeports.CapabilityNameV2           `json:"output_port_capabilities"`
	EffectClass            string                                    `json:"effect_class"`
	DescriptorDigest       core.Digest                               `json:"descriptor_digest"`
}

func (d ControlAdapterFactoryDescriptorV2) canonicalV2() ControlAdapterFactoryDescriptorV2 {
	clone := d
	clone.ResourceHandles = append([]runtimeports.ResourceHandleRefV1{}, d.ResourceHandles...)
	clone.OutputPortCapabilities = append([]runtimeports.CapabilityNameV2{}, d.OutputPortCapabilities...)
	sort.Slice(clone.ResourceHandles, func(i, j int) bool {
		left, right := clone.ResourceHandles[i], clone.ResourceHandles[j]
		if left.Owner != right.Owner {
			return ownerKeyV2(left.Owner) < ownerKeyV2(right.Owner)
		}
		return left.ID < right.ID
	})
	sort.Slice(clone.OutputPortCapabilities, func(i, j int) bool { return clone.OutputPortCapabilities[i] < clone.OutputPortCapabilities[j] })
	return clone
}

func (d ControlAdapterFactoryDescriptorV2) DigestV2() (core.Digest, error) {
	clone := d.canonicalV2()
	clone.Ref.Digest = ""
	clone.DescriptorDigest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.control-adapter-factory-v2", ControlAdapterFactoryContractVersionV2, "ControlAdapterFactoryDescriptorV2", clone)
}

func SealControlAdapterFactoryDescriptorV2(d ControlAdapterFactoryDescriptorV2) (ControlAdapterFactoryDescriptorV2, error) {
	if d.ContractVersion != "" && d.ContractVersion != ControlAdapterFactoryContractVersionV2 {
		return ControlAdapterFactoryDescriptorV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "control adapter factory contract version drifted")
	}
	d.ContractVersion = ControlAdapterFactoryContractVersionV2
	d = d.canonicalV2()
	providedRef, providedDescriptor := d.Ref.Digest, d.DescriptorDigest
	d.Ref.Digest = ""
	d.DescriptorDigest = ""
	digest, err := d.DigestV2()
	if err != nil {
		return ControlAdapterFactoryDescriptorV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedDescriptor != "" && providedDescriptor != digest) {
		return ControlAdapterFactoryDescriptorV2{}, NewError(ErrorConflict, "control_adapter_descriptor_drift", "control adapter descriptor supplied a wrong non-zero digest")
	}
	d.Ref.Digest = digest
	d.DescriptorDigest = digest
	if err := d.Validate(); err != nil {
		return ControlAdapterFactoryDescriptorV2{}, err
	}
	return d, nil
}

func (d ControlAdapterFactoryDescriptorV2) Validate() error {
	if d.ContractVersion != ControlAdapterFactoryContractVersionV2 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "control adapter factory contract version is unsupported")
	}
	if err := d.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(d.ComponentID)); err != nil {
		return err
	}
	if err := d.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if _, err := core.ParseSemanticVersion(d.ComponentContract); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(d.Capability)); err != nil {
		return err
	}
	if err := d.Binding.Validate(); err != nil {
		return err
	}
	if d.Binding.ComponentID != d.ComponentID {
		return NewError(ErrorConflict, "control_adapter_binding_component_drift", "control adapter Binding does not belong to its component")
	}
	if err := d.Generation.Validate(); err != nil {
		return err
	}
	if err := d.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	if d.EffectClass != ControlAdapterEffectNoneV2 {
		return NewError(ErrorPrecondition, "control_adapter_effect_forbidden", "control adapter construction must declare EffectClass=none")
	}
	if len(d.ResourceHandles) == 0 || len(d.ResourceHandles) > MaxControlAdapterResourcesV2 || len(d.OutputPortCapabilities) == 0 || len(d.OutputPortCapabilities) > MaxControlAdapterOutputsV2 {
		return NewError(ErrorInvalidArgument, "control_adapter_descriptor_incomplete", "control adapter resources or outputs are incomplete")
	}
	for i, handle := range d.ResourceHandles {
		if err := handle.Validate(); err != nil {
			return err
		}
		if i > 0 {
			previous := d.ResourceHandles[i-1]
			if ownerKeyV2(previous.Owner) > ownerKeyV2(handle.Owner) || (previous.Owner == handle.Owner && previous.ID >= handle.ID) {
				return NewError(ErrorConflict, "control_adapter_resource_duplicate", "control adapter resources must be sorted and unique")
			}
		}
	}
	for i, capability := range d.OutputPortCapabilities {
		if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(capability)); err != nil {
			return err
		}
		if i > 0 && d.OutputPortCapabilities[i-1] >= capability {
			return NewError(ErrorConflict, "control_adapter_output_duplicate", "control adapter output capabilities must be sorted and unique")
		}
	}
	expected, err := d.DigestV2()
	if err != nil {
		return err
	}
	if expected != d.DescriptorDigest || expected != d.Ref.Digest {
		return NewError(ErrorConflict, "control_adapter_descriptor_drift", "control adapter descriptor digest drifted")
	}
	return nil
}

type ControlAdapterConformanceV2 struct {
	ContractVersion       string                         `json:"contract_version"`
	ConformanceID         string                         `json:"conformance_id"`
	Revision              core.Revision                  `json:"revision"`
	DescriptorRef         ControlAdapterFactoryRefV2     `json:"descriptor_ref"`
	CertificationCurrent  runtimeports.OwnerCurrentRefV1 `json:"certification_current"`
	StaticImportEvidence  runtimeports.OwnerCurrentRefV1 `json:"static_import_evidence"`
	NoRawProviderEvidence runtimeports.OwnerCurrentRefV1 `json:"no_raw_provider_evidence"`
	ZeroEffectEvidence    runtimeports.OwnerCurrentRefV1 `json:"zero_effect_evidence"`
	CheckedUnixNano       int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                          `json:"expires_unix_nano"`
	Digest                core.Digest                    `json:"digest"`
}

func (c ControlAdapterConformanceV2) DigestV2() (core.Digest, error) {
	clone := c
	clone.Digest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.control-adapter-conformance-v2", ControlAdapterFactoryContractVersionV2, "ControlAdapterConformanceV2", clone)
}

func SealControlAdapterConformanceV2(c ControlAdapterConformanceV2) (ControlAdapterConformanceV2, error) {
	if c.ContractVersion != "" && c.ContractVersion != ControlAdapterFactoryContractVersionV2 {
		return ControlAdapterConformanceV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "control adapter conformance version drifted")
	}
	c.ContractVersion = ControlAdapterFactoryContractVersionV2
	provided := c.Digest
	c.Digest = ""
	digest, err := c.DigestV2()
	if err != nil {
		return ControlAdapterConformanceV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlAdapterConformanceV2{}, NewError(ErrorConflict, "control_adapter_conformance_drift", "control adapter conformance supplied a wrong non-zero digest")
	}
	c.Digest = digest
	if err := c.Validate(); err != nil {
		return ControlAdapterConformanceV2{}, err
	}
	return c, nil
}

func (c ControlAdapterConformanceV2) Validate() error {
	if c.ContractVersion != ControlAdapterFactoryContractVersionV2 || c.Revision == 0 || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "control_adapter_conformance_incomplete", "control adapter conformance is incomplete")
	}
	if err := ValidateIdentifierV1("control adapter conformance id", c.ConformanceID); err != nil {
		return err
	}
	if err := c.DescriptorRef.Validate(); err != nil {
		return err
	}
	minimum := int64(0)
	for _, ref := range []runtimeports.OwnerCurrentRefV1{c.CertificationCurrent, c.StaticImportEvidence, c.NoRawProviderEvidence, c.ZeroEffectEvidence} {
		if err := ref.Validate(); err != nil {
			return err
		}
		if minimum == 0 || ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	if c.ExpiresUnixNano > minimum {
		return NewError(ErrorPrecondition, "control_adapter_conformance_ttl_drift", "control adapter conformance exceeds an evidence current")
	}
	expected, err := c.DigestV2()
	if err != nil {
		return err
	}
	if expected != c.Digest {
		return NewError(ErrorConflict, "control_adapter_conformance_drift", "control adapter conformance digest drifted")
	}
	return nil
}

func (c ControlAdapterConformanceV2) ValidateCurrent(expected ControlAdapterFactoryRefV2, now time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.DescriptorRef != expected {
		return NewError(ErrorConflict, "control_adapter_conformance_descriptor_drift", "control adapter conformance names another descriptor")
	}
	if now.IsZero() || now.UnixNano() < c.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "control adapter conformance clock regressed")
	}
	if now.UnixNano() >= c.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "control_adapter_conformance_expired", "control adapter conformance expired")
	}
	return nil
}

type ControlAdapterConstructRequestV2 struct {
	ContractVersion           string                            `json:"contract_version"`
	HostID                    string                            `json:"host_id"`
	StartID                   string                            `json:"start_id"`
	AttemptID                 string                            `json:"attempt_id"`
	Descriptor                ControlAdapterFactoryDescriptorV2 `json:"descriptor"`
	Conformance               ControlAdapterConformanceV2       `json:"conformance"`
	ResourceBindings          runtimeports.ResourceBindingSetV1 `json:"resource_bindings"`
	RequestedNotAfterUnixNano int64                             `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                       `json:"request_digest"`
}

func (r ControlAdapterConstructRequestV2) DigestV2() (core.Digest, error) {
	clone := r
	clone.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.control-adapter-construct-v2", ControlAdapterFactoryContractVersionV2, "ControlAdapterConstructRequestV2", clone)
}

func SealControlAdapterConstructRequestV2(r ControlAdapterConstructRequestV2) (ControlAdapterConstructRequestV2, error) {
	if r.ContractVersion != "" && r.ContractVersion != ControlAdapterFactoryContractVersionV2 {
		return ControlAdapterConstructRequestV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "control adapter construct request version drifted")
	}
	r.ContractVersion = ControlAdapterFactoryContractVersionV2
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ControlAdapterConstructRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlAdapterConstructRequestV2{}, NewError(ErrorConflict, "control_adapter_construct_request_drift", "control adapter construct request supplied a wrong non-zero digest")
	}
	r.RequestDigest = digest
	if err := r.Validate(); err != nil {
		return ControlAdapterConstructRequestV2{}, err
	}
	return r, nil
}

func (r ControlAdapterConstructRequestV2) Validate() error {
	if r.ContractVersion != ControlAdapterFactoryContractVersionV2 || r.RequestedNotAfterUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "control_adapter_construct_request_incomplete", "control adapter construct request is incomplete")
	}
	for field, value := range map[string]string{"host id": r.HostID, "start id": r.StartID, "attempt id": r.AttemptID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if err := r.Descriptor.Validate(); err != nil {
		return err
	}
	if err := r.Conformance.Validate(); err != nil {
		return err
	}
	if r.Conformance.DescriptorRef != r.Descriptor.Ref {
		return NewError(ErrorConflict, "control_adapter_conformance_descriptor_drift", "construct request conformance does not bind its descriptor")
	}
	if err := r.ResourceBindings.Validate(); err != nil {
		return err
	}
	if r.ResourceBindings.Ref != r.Descriptor.ResourceBindingSet || len(r.ResourceBindings.Bindings) != len(r.Descriptor.ResourceHandles) {
		return NewError(ErrorConflict, "control_adapter_resource_set_drift", "construct request resources do not match the descriptor")
	}
	handles := map[runtimeports.ResourceHandleRefV1]struct{}{}
	for _, binding := range r.ResourceBindings.Bindings {
		handles[binding.Handle] = struct{}{}
	}
	for _, handle := range r.Descriptor.ResourceHandles {
		if _, ok := handles[handle]; !ok {
			return NewError(ErrorConflict, "control_adapter_resource_handle_drift", "construct request lacks an exact descriptor resource")
		}
	}
	if r.RequestedNotAfterUnixNano > r.Conformance.ExpiresUnixNano || r.RequestedNotAfterUnixNano > r.ResourceBindings.ExpiresUnixNano || r.RequestedNotAfterUnixNano > r.Descriptor.Binding.ExpiresUnixNano || r.RequestedNotAfterUnixNano > r.Descriptor.Generation.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "control_adapter_construct_ttl_drift", "construct request exceeds an exact current input")
	}
	expected, err := r.DigestV2()
	if err != nil {
		return err
	}
	if expected != r.RequestDigest {
		return NewError(ErrorConflict, "control_adapter_construct_request_drift", "control adapter construct request digest drifted")
	}
	return nil
}

func (r ControlAdapterConstructRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.ResourceBindings.CheckedUnixNano || now.UnixNano() < r.Conformance.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "control adapter construct clock regressed")
	}
	if now.UnixNano() >= r.RequestedNotAfterUnixNano {
		return NewError(ErrorPrecondition, "control_adapter_construct_expired", "control adapter construct request expired")
	}
	return nil
}

type ControlAdapterInstanceV2 struct {
	ContractVersion string                     `json:"contract_version"`
	InstanceRef     ExactRefV1                 `json:"instance_ref"`
	AttemptID       string                     `json:"attempt_id"`
	RequestDigest   core.Digest                `json:"request_digest"`
	DescriptorRef   ControlAdapterFactoryRefV2 `json:"descriptor_ref"`
	CheckedUnixNano int64                      `json:"checked_unix_nano"`
	ExpiresUnixNano int64                      `json:"expires_unix_nano"`
	Digest          core.Digest                `json:"digest"`
}

func (i ControlAdapterInstanceV2) DigestV2() (core.Digest, error) {
	clone := i
	clone.Digest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.control-adapter-instance-v2", ControlAdapterFactoryContractVersionV2, "ControlAdapterInstanceV2", clone)
}

func SealControlAdapterInstanceV2(i ControlAdapterInstanceV2) (ControlAdapterInstanceV2, error) {
	if i.ContractVersion != "" && i.ContractVersion != ControlAdapterFactoryContractVersionV2 {
		return ControlAdapterInstanceV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "control adapter instance version drifted")
	}
	i.ContractVersion = ControlAdapterFactoryContractVersionV2
	provided := i.Digest
	i.Digest = ""
	digest, err := i.DigestV2()
	if err != nil {
		return ControlAdapterInstanceV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlAdapterInstanceV2{}, NewError(ErrorConflict, "control_adapter_instance_drift", "control adapter instance supplied a wrong non-zero digest")
	}
	i.Digest = digest
	if err := i.Validate(); err != nil {
		return ControlAdapterInstanceV2{}, err
	}
	return i, nil
}

func (i ControlAdapterInstanceV2) Validate() error {
	if i.ContractVersion != ControlAdapterFactoryContractVersionV2 || i.CheckedUnixNano <= 0 || i.ExpiresUnixNano <= i.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "control_adapter_instance_incomplete", "control adapter instance is incomplete")
	}
	if err := i.InstanceRef.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("control adapter attempt id", i.AttemptID); err != nil {
		return err
	}
	if err := i.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := i.DescriptorRef.Validate(); err != nil {
		return err
	}
	expected, err := i.DigestV2()
	if err != nil {
		return err
	}
	if expected != i.Digest {
		return NewError(ErrorConflict, "control_adapter_instance_drift", "control adapter instance digest drifted")
	}
	return nil
}

func (i ControlAdapterInstanceV2) ValidateCurrent(request ControlAdapterConstructRequestV2, now time.Time) error {
	if err := i.Validate(); err != nil {
		return err
	}
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if i.AttemptID != request.AttemptID || i.RequestDigest != request.RequestDigest || i.DescriptorRef != request.Descriptor.Ref || i.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return NewError(ErrorConflict, "control_adapter_instance_request_drift", "control adapter instance is not the exact request result")
	}
	if now.UnixNano() < i.CheckedUnixNano || now.UnixNano() >= i.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "control_adapter_instance_not_current", "control adapter instance is not current")
	}
	return nil
}

func ownerKeyV2(owner core.OwnerRef) string {
	return owner.Domain + "\x00" + string(owner.ID)
}
