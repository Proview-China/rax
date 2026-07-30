package contract

import (
	"slices"
	"sort"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	HostDeploymentCurrentContractVersionV2 = "praxis.agent-host/deployment-current/v2"
	HostDeploymentCurrentObjectKindV2      = "praxis.agent-host/HostDeploymentCurrentV2"
	HostDeploymentPublishRequestKindV2     = "praxis.agent-host/PublishHostDeploymentCurrentRequestV2"

	HostServiceBindingCurrentContractVersionV2 = "praxis.agent-host/service-binding-current/v2"
	HostServiceBindingCurrentObjectKindV2      = "praxis.agent-host/HostServiceBindingCurrentV2"

	MaxHostDeploymentResourceHandlesV2 = 256
	MaxHostDeploymentServiceBindingsV2 = 256
)

// HostDeploymentCurrentRefV2 is the exact Host-owned current coordinate. It
// binds only the Builder-owned exact Selection Ref and never copies PackageRef,
// PublicationRef or ClosureDigest.
type HostDeploymentCurrentRefV2 struct {
	HostID              string                                            `json:"host_id"`
	DeploymentID        string                                            `json:"deployment_id"`
	Revision            uint64                                            `json:"revision"`
	BootstrapDigest     DigestV1                                          `json:"bootstrap_digest"`
	PackageSelectionRef buildercontract.AgentPackageSelectionCurrentRefV1 `json:"package_selection_ref"`
	ExpiresUnixNano     int64                                             `json:"expires_unix_nano"`
	Digest              DigestV1                                          `json:"digest"`
}

func (ref HostDeploymentCurrentRefV2) IsZero() bool {
	return ref == (HostDeploymentCurrentRefV2{})
}

func (ref HostDeploymentCurrentRefV2) Validate() error {
	if err := ValidateIdentifierV1("host id", ref.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("deployment id", ref.DeploymentID); err != nil {
		return err
	}
	if ref.Revision == 0 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_deployment_v2_ref_incomplete", "Host deployment V2 current Ref is incomplete")
	}
	if err := ref.BootstrapDigest.Validate(); err != nil {
		return err
	}
	if err := ref.PackageSelectionRef.Validate(); err != nil {
		return err
	}
	return ref.Digest.Validate()
}

// HostServiceBindingCurrentV2 is the narrow, read-only current projection
// consumed by the Host deployment Owner. It does not grant service authority.
type HostServiceBindingCurrentV2 struct {
	ContractVersion  string                  `json:"contract_version"`
	ObjectKind       string                  `json:"object_kind"`
	Ref              HostServiceBindingRefV1 `json:"ref"`
	CheckedUnixNano  int64                   `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                   `json:"expires_unix_nano"`
	ProjectionDigest DigestV1                `json:"projection_digest"`
}

func hostServiceBindingCurrentDigestV2(value HostServiceBindingCurrentV2) (DigestV1, error) {
	value.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                      `json:"domain"`
		Type   string                      `json:"type"`
		Body   HostServiceBindingCurrentV2 `json:"body"`
	}{
		Domain: "praxis.agent-host.service-binding-current-v2",
		Type:   HostServiceBindingCurrentObjectKindV2,
		Body:   value,
	})
}

func SealHostServiceBindingCurrentV2(value HostServiceBindingCurrentV2) (HostServiceBindingCurrentV2, error) {
	if value.ContractVersion != "" && value.ContractVersion != HostServiceBindingCurrentContractVersionV2 {
		return HostServiceBindingCurrentV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "Host service binding current V2 contract version drifted")
	}
	if value.ObjectKind != "" && value.ObjectKind != HostServiceBindingCurrentObjectKindV2 {
		return HostServiceBindingCurrentV2{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "Host service binding current V2 object kind drifted")
	}
	value.ContractVersion = HostServiceBindingCurrentContractVersionV2
	value.ObjectKind = HostServiceBindingCurrentObjectKindV2
	provided := value.ProjectionDigest
	value.ProjectionDigest = ""
	digest, err := hostServiceBindingCurrentDigestV2(value)
	if err != nil {
		return HostServiceBindingCurrentV2{}, err
	}
	if provided != "" && provided != digest {
		return HostServiceBindingCurrentV2{}, NewError(ErrorConflict, "host_service_binding_current_digest_drift", "Host service binding current V2 supplied a wrong digest")
	}
	value.ProjectionDigest = digest
	return value, value.ValidateHistoricalV2()
}

func (value HostServiceBindingCurrentV2) ValidateHistoricalV2() error {
	if value.ContractVersion != HostServiceBindingCurrentContractVersionV2 ||
		value.ObjectKind != HostServiceBindingCurrentObjectKindV2 ||
		value.CheckedUnixNano <= 0 ||
		value.ExpiresUnixNano <= value.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_service_binding_current_incomplete", "Host service binding current V2 is incomplete")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if value.Ref.ExpiresUnixNano != value.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_service_binding_expiry_drift", "Host service binding current V2 expiry differs from its exact Ref")
	}
	want, err := hostServiceBindingCurrentDigestV2(value)
	if err != nil || want != value.ProjectionDigest {
		return NewError(ErrorConflict, "host_service_binding_current_digest_drift", "Host service binding current V2 digest drifted")
	}
	return nil
}

func (value HostServiceBindingCurrentV2) ValidateCurrentV2(expected HostServiceBindingRefV1, now time.Time) error {
	if err := value.ValidateHistoricalV2(); err != nil {
		return err
	}
	if value.Ref != expected {
		return NewError(ErrorConflict, "host_service_binding_current_ref_drift", "Host service binding current V2 exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "Host service binding current V2 clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "host_service_binding_expired", "Host service binding current V2 expired")
	}
	return nil
}

// PublishHostDeploymentCurrentRequestV2 carries only exact public inputs.
// PackageRef, PublicationRef and ClosureDigest are always freshly derived from
// the Builder-owned package selection and verified closure.
type PublishHostDeploymentCurrentRequestV2 struct {
	ContractVersion           string                                            `json:"contract_version"`
	ObjectKind                string                                            `json:"object_kind"`
	Bootstrap                 HostBootstrapConfigV1                             `json:"bootstrap"`
	DeploymentID              string                                            `json:"deployment_id"`
	ExpectedCurrent           HostDeploymentCurrentRefV2                        `json:"expected_current,omitempty"`
	PackageSelectionRef       buildercontract.AgentPackageSelectionCurrentRefV1 `json:"package_selection_ref"`
	ResourceHandles           []runtimeports.ResourceHandleRefV1                `json:"resource_handles"`
	ServiceBindings           []HostServiceBindingRefV1                         `json:"service_bindings"`
	RequestedUnixNano         int64                                             `json:"requested_unix_nano"`
	RequestedNotAfterUnixNano int64                                             `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1                                          `json:"request_digest"`
}

func (request PublishHostDeploymentCurrentRequestV2) canonicalV2() PublishHostDeploymentCurrentRequestV2 {
	request.ResourceHandles = canonicalResourceHandlesV2(request.ResourceHandles)
	request.ServiceBindings = canonicalServiceBindingsV2(request.ServiceBindings)
	return request
}

func ClonePublishHostDeploymentCurrentRequestV2(request PublishHostDeploymentCurrentRequestV2) PublishHostDeploymentCurrentRequestV2 {
	request.Bootstrap.StatePlaneBindingIDs = cloneStringsV2(request.Bootstrap.StatePlaneBindingIDs)
	request.Bootstrap.RuntimeServiceBindingIDs = cloneStringsV2(request.Bootstrap.RuntimeServiceBindingIDs)
	request.Bootstrap.ApplicationServiceBindingIDs = cloneStringsV2(request.Bootstrap.ApplicationServiceBindingIDs)
	request.Bootstrap.HarnessServiceBindingIDs = cloneStringsV2(request.Bootstrap.HarnessServiceBindingIDs)
	request.Bootstrap.EnabledControlAPISurfaces = cloneStringsV2(request.Bootstrap.EnabledControlAPISurfaces)
	resources := request.ResourceHandles
	request.ResourceHandles = make([]runtimeports.ResourceHandleRefV1, len(resources))
	copy(request.ResourceHandles, resources)
	services := request.ServiceBindings
	request.ServiceBindings = make([]HostServiceBindingRefV1, len(services))
	copy(request.ServiceBindings, services)
	return request
}

func cloneStringsV2(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func publishHostDeploymentRequestDigestV2(request PublishHostDeploymentCurrentRequestV2) (DigestV1, error) {
	request = request.canonicalV2()
	request.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                                `json:"domain"`
		Type   string                                `json:"type"`
		Body   PublishHostDeploymentCurrentRequestV2 `json:"body"`
	}{
		Domain: "praxis.agent-host.deployment-current-v2",
		Type:   HostDeploymentPublishRequestKindV2,
		Body:   request,
	})
}

func SealPublishHostDeploymentCurrentRequestV2(request PublishHostDeploymentCurrentRequestV2) (PublishHostDeploymentCurrentRequestV2, error) {
	if request.ContractVersion != "" && request.ContractVersion != HostDeploymentCurrentContractVersionV2 {
		return PublishHostDeploymentCurrentRequestV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "Host deployment publish V2 contract version drifted")
	}
	if request.ObjectKind != "" && request.ObjectKind != HostDeploymentPublishRequestKindV2 {
		return PublishHostDeploymentCurrentRequestV2{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "Host deployment publish V2 object kind drifted")
	}
	request.ContractVersion = HostDeploymentCurrentContractVersionV2
	request.ObjectKind = HostDeploymentPublishRequestKindV2
	request = request.canonicalV2()
	provided := request.RequestDigest
	request.RequestDigest = ""
	digest, err := publishHostDeploymentRequestDigestV2(request)
	if err != nil {
		return PublishHostDeploymentCurrentRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return PublishHostDeploymentCurrentRequestV2{}, NewError(ErrorConflict, "host_deployment_request_digest_drift", "Host deployment publish V2 supplied a wrong request digest")
	}
	request.RequestDigest = digest
	return request, request.ValidateHistoricalV2()
}

func (request PublishHostDeploymentCurrentRequestV2) ValidateHistoricalV2() error {
	if request.ContractVersion != HostDeploymentCurrentContractVersionV2 ||
		request.ObjectKind != HostDeploymentPublishRequestKindV2 {
		return NewError(ErrorInvalidArgument, "host_deployment_request_discriminator_invalid", "Host deployment publish V2 discriminator is invalid")
	}
	if err := request.Bootstrap.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("deployment id", request.DeploymentID); err != nil {
		return err
	}
	if !request.ExpectedCurrent.IsZero() {
		if err := request.ExpectedCurrent.Validate(); err != nil {
			return err
		}
		if request.ExpectedCurrent.HostID != request.Bootstrap.HostID ||
			request.ExpectedCurrent.DeploymentID != request.DeploymentID ||
			request.ExpectedCurrent.BootstrapDigest != request.Bootstrap.ContentDigest {
			return NewError(ErrorConflict, "host_deployment_expected_current_drift", "Host deployment publish V2 expected current names another deployment")
		}
	}
	if err := request.PackageSelectionRef.Validate(); err != nil {
		return err
	}
	if request.RequestedUnixNano <= 0 ||
		request.RequestedNotAfterUnixNano <= request.RequestedUnixNano {
		return NewError(ErrorInvalidArgument, "host_deployment_request_window_invalid", "Host deployment publish V2 request window is invalid")
	}
	if request.RequestedNotAfterUnixNano > request.Bootstrap.NotAfterUnixNano {
		return NewError(ErrorPrecondition, "host_deployment_request_ttl_exceeds_bootstrap", "Host deployment publish V2 request TTL exceeds bootstrap")
	}
	if len(request.ResourceHandles) > MaxHostDeploymentResourceHandlesV2 ||
		len(request.ServiceBindings) > MaxHostDeploymentServiceBindingsV2 {
		return NewError(ErrorInvalidArgument, "host_deployment_binding_count_invalid", "Host deployment publish V2 binding count is invalid")
	}
	canonical := request.canonicalV2()
	if request.ResourceHandles == nil ||
		request.ServiceBindings == nil ||
		!slices.Equal(request.ResourceHandles, canonical.ResourceHandles) ||
		!slices.Equal(request.ServiceBindings, canonical.ServiceBindings) {
		return NewError(ErrorConflict, "host_deployment_request_not_canonical", "Host deployment publish V2 bindings are not canonical")
	}
	if err := validateHostDeploymentBindingsV2(request.Bootstrap, request.ResourceHandles, request.ServiceBindings); err != nil {
		return err
	}
	want, err := publishHostDeploymentRequestDigestV2(request)
	if err != nil || want != request.RequestDigest {
		return NewError(ErrorConflict, "host_deployment_request_digest_drift", "Host deployment publish V2 request digest drifted")
	}
	return nil
}

func (request PublishHostDeploymentCurrentRequestV2) ValidateCurrentV2(now time.Time) error {
	if err := request.ValidateHistoricalV2(); err != nil {
		return err
	}
	if err := request.Bootstrap.ValidateCurrentV1(now); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < request.RequestedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "Host deployment publish V2 clock regressed")
	}
	if now.UnixNano() >= request.RequestedNotAfterUnixNano {
		return NewError(ErrorPrecondition, "host_deployment_request_expired", "Host deployment publish V2 request expired")
	}
	return nil
}

// HostDeploymentCurrentV2 stores one Builder-owned exact selection Ref and
// Host-owned resource/service references. It deliberately does not mirror
// PackageRef, PublicationRef or ClosureDigest.
type HostDeploymentCurrentV2 struct {
	ContractVersion  string                             `json:"contract_version"`
	ObjectKind       string                             `json:"object_kind"`
	Ref              HostDeploymentCurrentRefV2         `json:"ref"`
	ResourceHandles  []runtimeports.ResourceHandleRefV1 `json:"resource_handles"`
	ServiceBindings  []HostServiceBindingRefV1          `json:"service_bindings"`
	CheckedUnixNano  int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                              `json:"expires_unix_nano"`
	ProjectionDigest DigestV1                           `json:"projection_digest"`
}

func (current HostDeploymentCurrentV2) canonicalV2() HostDeploymentCurrentV2 {
	current.ResourceHandles = canonicalResourceHandlesV2(current.ResourceHandles)
	current.ServiceBindings = canonicalServiceBindingsV2(current.ServiceBindings)
	return current
}

func hostDeploymentCurrentDigestV2(current HostDeploymentCurrentV2) (DigestV1, error) {
	current = current.canonicalV2()
	current.Ref.Digest = ""
	current.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   HostDeploymentCurrentV2 `json:"body"`
	}{
		Domain: "praxis.agent-host.deployment-current-v2",
		Type:   HostDeploymentCurrentObjectKindV2,
		Body:   current,
	})
}

func SealHostDeploymentCurrentV2(current HostDeploymentCurrentV2) (HostDeploymentCurrentV2, error) {
	if current.ContractVersion != "" && current.ContractVersion != HostDeploymentCurrentContractVersionV2 {
		return HostDeploymentCurrentV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "Host deployment current V2 contract version drifted")
	}
	if current.ObjectKind != "" && current.ObjectKind != HostDeploymentCurrentObjectKindV2 {
		return HostDeploymentCurrentV2{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "Host deployment current V2 object kind drifted")
	}
	current.ContractVersion = HostDeploymentCurrentContractVersionV2
	current.ObjectKind = HostDeploymentCurrentObjectKindV2
	current = current.canonicalV2()
	providedRef := current.Ref.Digest
	providedProjection := current.ProjectionDigest
	current.Ref.Digest = ""
	current.ProjectionDigest = ""
	digest, err := hostDeploymentCurrentDigestV2(current)
	if err != nil {
		return HostDeploymentCurrentV2{}, err
	}
	if (providedRef != "" && providedRef != digest) ||
		(providedProjection != "" && providedProjection != digest) {
		return HostDeploymentCurrentV2{}, NewError(ErrorConflict, "host_deployment_v2_digest_drift", "Host deployment current V2 supplied a wrong digest")
	}
	current.Ref.Digest = digest
	current.ProjectionDigest = digest
	return current, current.ValidateHistoricalV2()
}

func (current HostDeploymentCurrentV2) ValidateHistoricalV2() error {
	if current.ContractVersion != HostDeploymentCurrentContractVersionV2 ||
		current.ObjectKind != HostDeploymentCurrentObjectKindV2 ||
		current.CheckedUnixNano <= 0 ||
		current.ExpiresUnixNano <= current.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_deployment_v2_incomplete", "Host deployment current V2 is incomplete")
	}
	if err := current.Ref.Validate(); err != nil {
		return err
	}
	if current.Ref.ExpiresUnixNano != current.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_deployment_v2_expiry_drift", "Host deployment current V2 Ref expiry drifted")
	}
	if len(current.ResourceHandles) > MaxHostDeploymentResourceHandlesV2 ||
		len(current.ServiceBindings) > MaxHostDeploymentServiceBindingsV2 {
		return NewError(ErrorInvalidArgument, "host_deployment_v2_binding_count_invalid", "Host deployment current V2 binding count is invalid")
	}
	canonical := current.canonicalV2()
	if current.ResourceHandles == nil ||
		current.ServiceBindings == nil ||
		!slices.Equal(current.ResourceHandles, canonical.ResourceHandles) ||
		!slices.Equal(current.ServiceBindings, canonical.ServiceBindings) {
		return NewError(ErrorConflict, "host_deployment_v2_not_canonical", "Host deployment current V2 bindings are not canonical")
	}
	seenResources := map[string]struct{}{}
	minimum := current.Ref.PackageSelectionRef.ExpiresUnixNano
	for _, ref := range current.ResourceHandles {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Owner.Domain) + "\x00" + string(ref.Owner.ID) + "\x00" + ref.ID
		if _, exists := seenResources[key]; exists {
			return NewError(ErrorConflict, "duplicate_resource_handle", "Host deployment current V2 duplicates a resource handle")
		}
		seenResources[key] = struct{}{}
		minimum = minInt64V2(minimum, ref.ExpiresUnixNano)
	}
	seenServices := map[string]struct{}{}
	for _, ref := range current.ServiceBindings {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Role) + "\x00" + ref.ConfiguredID
		if _, exists := seenServices[key]; exists {
			return NewError(ErrorConflict, "duplicate_service_binding", "Host deployment current V2 duplicates a service binding")
		}
		seenServices[key] = struct{}{}
		minimum = minInt64V2(minimum, ref.ExpiresUnixNano)
	}
	if current.ExpiresUnixNano > minimum {
		return NewError(ErrorConflict, "host_deployment_v2_ttl_widened", "Host deployment current V2 TTL exceeds a stored exact input")
	}
	want, err := hostDeploymentCurrentDigestV2(current)
	if err != nil || want != current.Ref.Digest || want != current.ProjectionDigest {
		return NewError(ErrorConflict, "host_deployment_v2_digest_drift", "Host deployment current V2 digest drifted")
	}
	return nil
}

func (current HostDeploymentCurrentV2) ValidateCurrentV2(expected HostDeploymentCurrentRefV2, now time.Time) error {
	if err := current.ValidateHistoricalV2(); err != nil {
		return err
	}
	if current.Ref != expected {
		return NewError(ErrorConflict, "host_deployment_v2_ref_drift", "Host deployment current V2 exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < current.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "Host deployment current V2 clock regressed")
	}
	if now.UnixNano() >= current.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "host_deployment_v2_expired", "Host deployment current V2 expired")
	}
	return nil
}

func CloneHostDeploymentCurrentV2(current HostDeploymentCurrentV2) HostDeploymentCurrentV2 {
	resources := current.ResourceHandles
	current.ResourceHandles = make([]runtimeports.ResourceHandleRefV1, len(current.ResourceHandles))
	copy(current.ResourceHandles, resources)
	services := current.ServiceBindings
	current.ServiceBindings = make([]HostServiceBindingRefV1, len(current.ServiceBindings))
	copy(current.ServiceBindings, services)
	return current
}

func canonicalResourceHandlesV2(values []runtimeports.ResourceHandleRefV1) []runtimeports.ResourceHandleRefV1 {
	result := make([]runtimeports.ResourceHandleRefV1, len(values))
	copy(result, values)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Owner.Domain != right.Owner.Domain {
			return left.Owner.Domain < right.Owner.Domain
		}
		if left.Owner.ID != right.Owner.ID {
			return left.Owner.ID < right.Owner.ID
		}
		return left.ID < right.ID
	})
	return result
}

func canonicalServiceBindingsV2(values []HostServiceBindingRefV1) []HostServiceBindingRefV1 {
	result := make([]HostServiceBindingRefV1, len(values))
	copy(result, values)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Role != result[j].Role {
			return result[i].Role < result[j].Role
		}
		return result[i].ConfiguredID < result[j].ConfiguredID
	})
	return result
}

func validateHostDeploymentBindingsV2(
	bootstrap HostBootstrapConfigV1,
	resources []runtimeports.ResourceHandleRefV1,
	services []HostServiceBindingRefV1,
) error {
	expectedResources := map[string]struct{}{}
	for _, id := range bootstrap.StatePlaneBindingIDs {
		expectedResources[id] = struct{}{}
	}
	for _, ref := range resources {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, exists := expectedResources[ref.ID]; !exists {
			return NewError(ErrorConflict, "host_deployment_resource_drift", "Host deployment current V2 resource is not declared by bootstrap")
		}
		delete(expectedResources, ref.ID)
	}
	if len(expectedResources) != 0 {
		return NewError(ErrorConflict, "host_deployment_resource_missing", "Host deployment current V2 omitted a bootstrap resource")
	}

	expectedServices := bootstrapServiceBindingsV1(bootstrap)
	for _, ref := range services {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Role) + "\x00" + ref.ConfiguredID
		if _, exists := expectedServices[key]; !exists {
			return NewError(ErrorConflict, "host_deployment_service_drift", "Host deployment current V2 service is not declared by bootstrap")
		}
		delete(expectedServices, key)
	}
	if len(expectedServices) != 0 {
		return NewError(ErrorConflict, "host_deployment_service_missing", "Host deployment current V2 omitted a bootstrap service")
	}
	return nil
}

func minInt64V2(left, right int64) int64 {
	if right < left {
		return right
	}
	return left
}
