package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	BindingAdmissionContractVersionV1 = "praxis.runtime.binding-admission/v1"
	bindingAdmissionCanonicalDomainV1 = "praxis.runtime.binding-admission"
	MaxBindingAdmissionReleasesV1     = 256
)

// PreBindingComponentReleaseV1 intentionally contains only release-time and
// deployment-time facts. There is no constructed-component, Activation,
// Generation Association or post-activation current field in this nominal.
type PreBindingComponentReleaseV1 struct {
	ComponentID         ComponentIDV2     `json:"component_id"`
	Release             OwnerCurrentRefV1 `json:"release"`
	Certification       OwnerCurrentRefV1 `json:"certification_current"`
	DeploymentReadiness OwnerCurrentRefV1 `json:"deployment_readiness_current"`
}

func (r PreBindingComponentReleaseV1) Validate() error {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	for _, ref := range []OwnerCurrentRefV1{r.Release, r.Certification, r.DeploymentReadiness} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type BindingAdmissionRequestV1 struct {
	ContractVersion           string                         `json:"contract_version"`
	AttemptID                 string                         `json:"attempt_id"`
	DefinitionCurrent         OwnerCurrentRefV1              `json:"definition_current"`
	PlanCurrent               OwnerCurrentRefV1              `json:"plan_current"`
	AssemblyCurrent           OwnerCurrentRefV1              `json:"assembly_current"`
	CatalogCurrent            OwnerCurrentRefV1              `json:"catalog_current"`
	ResolutionCurrent         OwnerCurrentRefV1              `json:"resolution_current"`
	Releases                  []PreBindingComponentReleaseV1 `json:"releases"`
	ResourceBindingSet        ResourceBindingSetRefV1        `json:"resource_binding_set"`
	AuthorityCurrent          OwnerCurrentRefV1              `json:"authority_current"`
	PolicyCurrent             OwnerCurrentRefV1              `json:"policy_current"`
	ExpectedBindingSetID      string                         `json:"expected_binding_set_id"`
	RequestedNotAfterUnixNano int64                          `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                    `json:"request_digest"`
}

func (r BindingAdmissionRequestV1) Validate() error {
	if r.ContractVersion != BindingAdmissionContractVersionV1 || invalidH4IDV1(r.AttemptID) || invalidH4IDV1(r.ExpectedBindingSetID) || r.RequestedNotAfterUnixNano <= 0 || len(r.Releases) == 0 || len(r.Releases) > MaxBindingAdmissionReleasesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission request is incomplete")
	}
	for _, ref := range []OwnerCurrentRefV1{r.DefinitionCurrent, r.PlanCurrent, r.AssemblyCurrent, r.CatalogCurrent, r.ResolutionCurrent, r.AuthorityCurrent, r.PolicyCurrent} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := r.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	minimum := r.ResourceBindingSet.ExpiresUnixNano
	for _, ref := range []OwnerCurrentRefV1{r.DefinitionCurrent, r.PlanCurrent, r.AssemblyCurrent, r.CatalogCurrent, r.ResolutionCurrent, r.AuthorityCurrent, r.PolicyCurrent} {
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	var previous ComponentIDV2
	for index, release := range r.Releases {
		if err := release.Validate(); err != nil {
			return err
		}
		if index > 0 && release.ComponentID <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Binding admission releases must be sorted and unique")
		}
		previous = release.ComponentID
		for _, ref := range []OwnerCurrentRefV1{release.Release, release.Certification, release.DeploymentReadiness} {
			if ref.ExpiresUnixNano < minimum {
				minimum = ref.ExpiresUnixNano
			}
		}
	}
	if r.RequestedNotAfterUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission requested TTL exceeds a pre-binding current")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission request digest drifted")
	}
	return nil
}

func (r BindingAdmissionRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Binding admission current validation requires time")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission request expired")
	}
	return nil
}

func (r BindingAdmissionRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.RequestDigest = ""
	if copy.Releases == nil {
		copy.Releases = []PreBindingComponentReleaseV1{}
	}
	return core.CanonicalJSONDigest(bindingAdmissionCanonicalDomainV1, BindingAdmissionContractVersionV1, "BindingAdmissionRequestV1", copy)
}

func SealBindingAdmissionRequestV1(r BindingAdmissionRequestV1) (BindingAdmissionRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != BindingAdmissionContractVersionV1 {
		return BindingAdmissionRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission contract version drifted")
	}
	r.ContractVersion = BindingAdmissionContractVersionV1
	r.Releases = append([]PreBindingComponentReleaseV1{}, r.Releases...)
	sort.Slice(r.Releases, func(i, j int) bool { return r.Releases[i].ComponentID < r.Releases[j].ComponentID })
	providedDigest := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return BindingAdmissionRequestV1{}, err
	}
	if providedDigest != "" && providedDigest != digest {
		return BindingAdmissionRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type BindingAdmissionInspectRequestV1 struct {
	AttemptID     string      `json:"attempt_id"`
	RequestDigest core.Digest `json:"request_digest"`
}

func (r BindingAdmissionInspectRequestV1) Validate() error {
	if invalidH4IDV1(r.AttemptID) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission inspect Attempt ID is required")
	}
	return r.RequestDigest.Validate()
}

type BindingAdmissionBindingRefV1 struct {
	ComponentID     ComponentIDV2 `json:"component_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r BindingAdmissionBindingRefV1) Validate() error {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	if invalidH4IDV1(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Binding Ref is incomplete")
	}
	return r.Digest.Validate()
}

type BindingAdmissionBindingSetRefV1 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r BindingAdmissionBindingSetRefV1) Validate() error {
	if invalidH4IDV1(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission BindingSet Ref is incomplete")
	}
	return r.Digest.Validate()
}

type BindingAdmissionResultV1 struct {
	ContractVersion    string                          `json:"contract_version"`
	AttemptID          string                          `json:"attempt_id"`
	RequestDigest      core.Digest                     `json:"request_digest"`
	BindingSet         BindingAdmissionBindingSetRefV1 `json:"binding_set"`
	Bindings           []BindingAdmissionBindingRefV1  `json:"bindings"`
	ResourceBindingSet ResourceBindingSetRefV1         `json:"resource_binding_set"`
	CheckedUnixNano    int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                           `json:"expires_unix_nano"`
	ResultDigest       core.Digest                     `json:"result_digest"`
}

func (r BindingAdmissionResultV1) Validate() error {
	if r.ContractVersion != BindingAdmissionContractVersionV1 || invalidH4IDV1(r.AttemptID) || r.RequestDigest.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || len(r.Bindings) == 0 || len(r.Bindings) > MaxBindingAdmissionReleasesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission result is incomplete")
	}
	if err := r.BindingSet.Validate(); err != nil {
		return err
	}
	if err := r.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	if r.BindingSet.ExpiresUnixNano != r.ExpiresUnixNano || r.ExpiresUnixNano > r.ResourceBindingSet.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission result TTL drifted")
	}
	var previous ComponentIDV2
	for index, binding := range r.Bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if index > 0 && binding.ComponentID <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Binding admission result bindings must be sorted and unique")
		}
		if binding.ExpiresUnixNano < r.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission result exceeds a Binding TTL")
		}
		previous = binding.ComponentID
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.ResultDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission result digest drifted")
	}
	return nil
}

func (r BindingAdmissionResultV1) ValidateCurrent(request BindingAdmissionRequestV1, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if r.AttemptID != request.AttemptID || r.RequestDigest != request.RequestDigest || r.BindingSet.ID != request.ExpectedBindingSetID || r.ResourceBindingSet != request.ResourceBindingSet || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano || len(r.Bindings) != len(request.Releases) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission result is not the exact request successor")
	}
	for index := range r.Bindings {
		if r.Bindings[index].ComponentID != request.Releases[index].ComponentID {
			return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "Binding admission result component set drifted")
		}
	}
	if now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Binding admission result clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Binding admission result expired")
	}
	return nil
}

func (r BindingAdmissionResultV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.ResultDigest = ""
	if copy.Bindings == nil {
		copy.Bindings = []BindingAdmissionBindingRefV1{}
	}
	return core.CanonicalJSONDigest(bindingAdmissionCanonicalDomainV1, BindingAdmissionContractVersionV1, "BindingAdmissionResultV1", copy)
}

func SealBindingAdmissionResultV1(r BindingAdmissionResultV1) (BindingAdmissionResultV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != BindingAdmissionContractVersionV1 {
		return BindingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission result contract version drifted")
	}
	r.ContractVersion = BindingAdmissionContractVersionV1
	r.Bindings = append([]BindingAdmissionBindingRefV1{}, r.Bindings...)
	sort.Slice(r.Bindings, func(i, j int) bool { return r.Bindings[i].ComponentID < r.Bindings[j].ComponentID })
	r.BindingSet.ExpiresUnixNano = r.ExpiresUnixNano
	providedDigest := r.ResultDigest
	r.ResultDigest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return BindingAdmissionResultV1{}, err
	}
	if providedDigest != "" && providedDigest != digest {
		return BindingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Binding admission result supplied a wrong digest")
	}
	r.ResultDigest = digest
	return r, r.Validate()
}

type BindingAdmissionGovernancePortV1 interface {
	StartOrInspectBindingAdmissionV1(context.Context, BindingAdmissionRequestV1) (BindingAdmissionResultV1, error)
	InspectBindingAdmissionV1(context.Context, BindingAdmissionInspectRequestV1) (BindingAdmissionResultV1, error)
}
