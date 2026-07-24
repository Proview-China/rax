package contract

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const SystemReadyContractVersionV2 = "praxis.agent-host/system-ready/v2"

type ComponentProductionCurrentV2 struct {
	Domain               runtimeports.NamespacedNameV2             `json:"domain"`
	ReleaseCurrent       runtimeports.OwnerCurrentRefV1            `json:"release_current"`
	ConstructedComponent ExactRefV1                                `json:"constructed_component"`
	Binding              runtimeports.BindingAdmissionBindingRefV1 `json:"binding"`
	GenerationCurrent    runtimeports.OwnerCurrentRefV1            `json:"generation_current"`
	ActivationCurrent    runtimeports.OwnerCurrentRefV1            `json:"activation_current"`
	ProductionCurrent    runtimeports.OwnerCurrentRefV1            `json:"production_current"`
}

func (c ComponentProductionCurrentV2) Validate() error {
	if err := runtimeports.ValidateNamespacedNameV2(c.Domain); err != nil {
		return err
	}
	if err := c.ReleaseCurrent.Validate(); err != nil {
		return err
	}
	if err := c.ConstructedComponent.Validate(); err != nil {
		return err
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if string(c.Domain) != string(c.Binding.ComponentID) {
		return NewError(ErrorConflict, "component_production_binding_drift", "component production domain and Runtime Binding component drifted")
	}
	if err := c.GenerationCurrent.Validate(); err != nil {
		return err
	}
	if err := c.ActivationCurrent.Validate(); err != nil {
		return err
	}
	return c.ProductionCurrent.Validate()
}

func (c ComponentProductionCurrentV2) minimumExpiryV2() int64 {
	minimum := c.ReleaseCurrent.ExpiresUnixNano
	for _, value := range []int64{c.Binding.ExpiresUnixNano, c.GenerationCurrent.ExpiresUnixNano, c.ActivationCurrent.ExpiresUnixNano, c.ProductionCurrent.ExpiresUnixNano} {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

type SystemReadyFactRefV2 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r SystemReadyFactRefV2) Validate() error {
	if err := ValidateIdentifierV1("system ready fact id", r.ID); err != nil {
		return err
	}
	if r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "system_ready_fact_ref_incomplete", "SystemReady Fact Ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return nil
}

type SystemReadyFactV2 struct {
	ContractVersion          string                         `json:"contract_version"`
	Ref                      SystemReadyFactRefV2           `json:"ref"`
	HostID                   string                         `json:"host_id"`
	StartID                  string                         `json:"start_id"`
	HostStartClaim           HostStartClaimRefV1            `json:"host_start_claim"`
	DefinitionCurrent        runtimeports.OwnerCurrentRefV1 `json:"definition_current"`
	PlanCurrent              runtimeports.OwnerCurrentRefV1 `json:"plan_current"`
	AssemblyCurrent          runtimeports.OwnerCurrentRefV1 `json:"assembly_current"`
	BindingSetCurrent        runtimeports.OwnerCurrentRefV1 `json:"binding_set_current"`
	ActivationCurrent        runtimeports.OwnerCurrentRefV1 `json:"activation_current"`
	GenerationBindingCurrent runtimeports.OwnerCurrentRefV1 `json:"generation_binding_current"`
	ApplicationStartCurrent  runtimeports.OwnerCurrentRefV1 `json:"application_start_current"`
	SandboxLeaseCurrent      runtimeports.OwnerCurrentRefV1 `json:"sandbox_lease_current"`
	SandboxActiveCurrent     runtimeports.OwnerCurrentRefV1 `json:"sandbox_active_current"`
	ExecutionReadyCurrent    runtimeports.OwnerCurrentRefV1 `json:"execution_ready_current"`
	SupervisionPolicyCurrent runtimeports.OwnerCurrentRefV1 `json:"supervision_policy_current"`
	Components               []ComponentProductionCurrentV2 `json:"components"`
	MinimumReadyWindowNanos  int64                          `json:"minimum_ready_window_nanos"`
	CheckedUnixNano          int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                          `json:"expires_unix_nano"`
	Digest                   core.Digest                    `json:"digest"`
}

func (f SystemReadyFactV2) canonicalV2() SystemReadyFactV2 {
	clone := f
	clone.Components = append([]ComponentProductionCurrentV2{}, f.Components...)
	sort.Slice(clone.Components, func(i, j int) bool { return clone.Components[i].Domain < clone.Components[j].Domain })
	return clone
}

func (f SystemReadyFactV2) ClosureDigestV2() (core.Digest, error) {
	clone := f.canonicalV2()
	clone.ContractVersion = SystemReadyContractVersionV2
	clone.Ref = SystemReadyFactRefV2{}
	clone.Digest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-closure-v2", SystemReadyContractVersionV2, "SystemReadyClosureV2", clone)
}

func (f SystemReadyFactV2) DigestV2() (core.Digest, error) {
	clone := f.canonicalV2()
	clone.Ref.Digest = ""
	clone.Digest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-v2", SystemReadyContractVersionV2, "SystemReadyFactV2", clone)
}

func SealSystemReadyFactV2(f SystemReadyFactV2) (SystemReadyFactV2, error) {
	if f.ContractVersion != "" && f.ContractVersion != SystemReadyContractVersionV2 {
		return SystemReadyFactV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "SystemReady Fact version drifted")
	}
	f.ContractVersion = SystemReadyContractVersionV2
	f = f.canonicalV2()
	closure, err := f.ClosureDigestV2()
	if err != nil {
		return SystemReadyFactV2{}, err
	}
	expectedID := DeriveSystemReadyFactIDV2(f.HostID, f.StartID, closure)
	if f.Ref.ID != "" && f.Ref.ID != expectedID {
		return SystemReadyFactV2{}, NewError(ErrorConflict, "system_ready_fact_id_drift", "SystemReady Fact supplied a wrong deterministic ID")
	}
	f.Ref.ID = expectedID
	if f.Ref.Revision == 0 {
		f.Ref.Revision = 1
	}
	providedRef, providedDigest := f.Ref.Digest, f.Digest
	f.Ref.Digest, f.Digest = "", ""
	digest, err := f.DigestV2()
	if err != nil {
		return SystemReadyFactV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedDigest != "" && providedDigest != digest) {
		return SystemReadyFactV2{}, NewError(ErrorConflict, "system_ready_fact_drift", "SystemReady Fact supplied a wrong non-zero digest")
	}
	f.Ref.Digest, f.Digest = digest, digest
	if err := f.Validate(); err != nil {
		return SystemReadyFactV2{}, err
	}
	return f, nil
}

func (f SystemReadyFactV2) Validate() error {
	if f.ContractVersion != SystemReadyContractVersionV2 || f.CheckedUnixNano <= 0 || f.ExpiresUnixNano <= f.CheckedUnixNano || f.MinimumReadyWindowNanos <= 0 || len(f.Components) == 0 {
		return NewError(ErrorInvalidArgument, "system_ready_fact_incomplete", "SystemReady Fact is incomplete")
	}
	if err := f.Ref.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("host id", f.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", f.StartID); err != nil {
		return err
	}
	if err := f.HostStartClaim.Validate(); err != nil {
		return err
	}
	closure, err := f.ClosureDigestV2()
	if err != nil {
		return err
	}
	if f.Ref.ID != DeriveSystemReadyFactIDV2(f.HostID, f.StartID, closure) || f.Ref.Revision != 1 {
		return NewError(ErrorConflict, "system_ready_fact_id_drift", "SystemReady Fact ID is not the deterministic Host/Start identity")
	}
	if f.HostStartClaim.HostID != f.HostID || f.HostStartClaim.StartID != f.StartID {
		return NewError(ErrorConflict, "system_ready_claim_subject_drift", "SystemReady Fact names another HostStart Claim subject")
	}
	minimum := f.HostStartClaim.ExpiresUnixNano
	for _, ref := range []runtimeports.OwnerCurrentRefV1{f.DefinitionCurrent, f.PlanCurrent, f.AssemblyCurrent, f.BindingSetCurrent, f.ActivationCurrent, f.GenerationBindingCurrent, f.ApplicationStartCurrent, f.SandboxLeaseCurrent, f.SandboxActiveCurrent, f.ExecutionReadyCurrent, f.SupervisionPolicyCurrent} {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	for index, component := range f.Components {
		if err := component.Validate(); err != nil {
			return err
		}
		if index > 0 && f.Components[index-1].Domain >= component.Domain {
			return NewError(ErrorConflict, "system_ready_component_duplicate", "SystemReady components must be sorted and unique")
		}
		if component.minimumExpiryV2() < minimum {
			minimum = component.minimumExpiryV2()
		}
	}
	if f.Ref.ExpiresUnixNano != f.ExpiresUnixNano || f.ExpiresUnixNano > minimum || f.ExpiresUnixNano-f.CheckedUnixNano < f.MinimumReadyWindowNanos {
		return NewError(ErrorPrecondition, "system_ready_window_invalid", "SystemReady lifetime exceeds an input or misses the supervised minimum window")
	}
	expected, err := f.DigestV2()
	if err != nil {
		return err
	}
	if expected != f.Digest || expected != f.Ref.Digest {
		return NewError(ErrorConflict, "system_ready_fact_drift", "SystemReady Fact digest drifted")
	}
	return nil
}

func (f SystemReadyFactV2) ValidateCurrent(now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < f.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "SystemReady Fact clock regressed")
	}
	if now.UnixNano() >= f.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "system_ready_fact_expired", "SystemReady Fact expired")
	}
	return nil
}

type SystemReadyCurrentStateV2 string

const (
	SystemReadyCurrentReadyV2  SystemReadyCurrentStateV2 = "ready"
	SystemReadyCurrentFencedV2 SystemReadyCurrentStateV2 = "fenced"
)

type SystemReadyCurrentRefV2 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Epoch           core.Epoch    `json:"epoch"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r SystemReadyCurrentRefV2) Validate() error {
	if err := ValidateIdentifierV1("system ready current id", r.ID); err != nil {
		return err
	}
	if r.Revision == 0 || r.Epoch == 0 || r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "system_ready_current_ref_incomplete", "SystemReady Current Ref is incomplete")
	}
	return r.Digest.Validate()
}

type SystemReadyCurrentV2 struct {
	ContractVersion  string                    `json:"contract_version"`
	Ref              SystemReadyCurrentRefV2   `json:"ref"`
	HostID           string                    `json:"host_id"`
	StartID          string                    `json:"start_id"`
	FactRef          SystemReadyFactRefV2      `json:"fact_ref"`
	State            SystemReadyCurrentStateV2 `json:"state"`
	CheckedUnixNano  int64                     `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                     `json:"expires_unix_nano"`
	ProjectionDigest core.Digest               `json:"projection_digest"`
}

func (c SystemReadyCurrentV2) DigestV2() (core.Digest, error) {
	clone := c
	clone.Ref.Digest, clone.ProjectionDigest = "", ""
	return core.CanonicalJSONDigest("praxis.agent-host.system-ready-current-v2", SystemReadyContractVersionV2, "SystemReadyCurrentV2", clone)
}

func SealSystemReadyCurrentV2(c SystemReadyCurrentV2) (SystemReadyCurrentV2, error) {
	if c.ContractVersion != "" && c.ContractVersion != SystemReadyContractVersionV2 {
		return SystemReadyCurrentV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "SystemReady Current version drifted")
	}
	c.ContractVersion = SystemReadyContractVersionV2
	c.Ref.ExpiresUnixNano = c.ExpiresUnixNano
	providedRef, providedProjection := c.Ref.Digest, c.ProjectionDigest
	c.Ref.Digest, c.ProjectionDigest = "", ""
	digest, err := c.DigestV2()
	if err != nil {
		return SystemReadyCurrentV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return SystemReadyCurrentV2{}, NewError(ErrorConflict, "system_ready_current_drift", "SystemReady Current supplied a wrong non-zero digest")
	}
	c.Ref.Digest, c.ProjectionDigest = digest, digest
	if err := c.Validate(); err != nil {
		return SystemReadyCurrentV2{}, err
	}
	return c, nil
}

func (c SystemReadyCurrentV2) Validate() error {
	if c.ContractVersion != SystemReadyContractVersionV2 || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "system_ready_current_incomplete", "SystemReady Current is incomplete")
	}
	if err := c.Ref.Validate(); err != nil {
		return err
	}
	if err := c.FactRef.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("host id", c.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", c.StartID); err != nil {
		return err
	}
	if c.Ref.ID != DeriveSystemReadyCurrentIDV2(c.HostID, c.StartID) {
		return NewError(ErrorConflict, "system_ready_current_id_drift", "SystemReady Current ID is not the deterministic Host/Start identity")
	}
	if c.State != SystemReadyCurrentReadyV2 && c.State != SystemReadyCurrentFencedV2 {
		return NewError(ErrorInvalidArgument, "system_ready_current_state_invalid", "SystemReady Current state is unsupported")
	}
	if c.State == SystemReadyCurrentReadyV2 && c.ExpiresUnixNano > c.FactRef.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "system_ready_current_ttl_drift", "SystemReady Current exceeds its immutable Fact")
	}
	expected, err := c.DigestV2()
	if err != nil {
		return err
	}
	if expected != c.Ref.Digest || expected != c.ProjectionDigest {
		return NewError(ErrorConflict, "system_ready_current_drift", "SystemReady Current digest drifted")
	}
	return nil
}

func DeriveSystemReadyFactIDV2(hostID, startID string, closure core.Digest) string {
	digest, _ := core.CanonicalJSONDigest("praxis.agent-host.system-ready-id-v2", "2.0.0", "SystemReadyFactIDV2", struct {
		HostID  string      `json:"host_id"`
		StartID string      `json:"start_id"`
		Closure core.Digest `json:"closure"`
	}{hostID, startID, closure})
	return "ready/" + strings.TrimPrefix(string(digest), "sha256:")
}

func DeriveSystemReadyCurrentIDV2(hostID, startID string) string {
	digest, _ := core.CanonicalJSONDigest("praxis.agent-host.system-ready-current-id-v2", "2.0.0", "SystemReadyCurrentIDV2", struct {
		HostID  string `json:"host_id"`
		StartID string `json:"start_id"`
	}{hostID, startID})
	return "availability/" + strings.TrimPrefix(string(digest), "sha256:")
}

func (c SystemReadyCurrentV2) ValidateCurrent(expected SystemReadyCurrentRefV2, now time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Ref != expected {
		return NewError(ErrorConflict, "system_ready_current_ref_drift", "SystemReady Current exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < c.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "SystemReady Current clock regressed")
	}
	if c.State != SystemReadyCurrentReadyV2 || now.UnixNano() >= c.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "system_ready_not_available", "SystemReady Current is fenced or expired")
	}
	return nil
}

func ValidateSystemReadyCurrentSuccessorV2(current, next SystemReadyCurrentV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.HostID != next.HostID || current.StartID != next.StartID || current.Ref.ID != next.Ref.ID || next.Ref.Revision != current.Ref.Revision+1 {
		return NewError(ErrorConflict, "system_ready_current_identity_drift", "SystemReady Current successor identity drifted")
	}
	if current.State == SystemReadyCurrentFencedV2 {
		return NewError(ErrorPrecondition, "system_ready_fenced", "fenced SystemReady Current is terminal")
	}
	if next.CheckedUnixNano < current.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "SystemReady Current successor clock regressed")
	}
	if next.State == SystemReadyCurrentFencedV2 {
		if next.Ref.Epoch != current.Ref.Epoch || next.FactRef != current.FactRef {
			return NewError(ErrorConflict, "system_ready_fence_drift", "SystemReady fence must close the exact current epoch and Fact")
		}
	} else if next.State == SystemReadyCurrentReadyV2 {
		if next.Ref.Epoch != current.Ref.Epoch {
			return NewError(ErrorConflict, "system_ready_epoch_drift", "SystemReady renewal must preserve the live availability epoch")
		}
	} else {
		return NewError(ErrorInvalidArgument, "system_ready_current_state_invalid", "SystemReady successor state is unsupported")
	}
	return nil
}

func (c SystemReadyCurrentV2) ToAgentExecutionAvailabilityV1(owner core.OwnerRef) (runtimeports.AgentExecutionAvailabilityProjectionV1, error) {
	if err := c.Validate(); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	state := runtimeports.AgentExecutionAvailabilityReadyV1
	if c.State == SystemReadyCurrentFencedV2 {
		state = runtimeports.AgentExecutionAvailabilityFencedV1
	}
	return runtimeports.SealAgentExecutionAvailabilityProjectionV1(runtimeports.AgentExecutionAvailabilityProjectionV1{Ref: runtimeports.AgentExecutionAvailabilityRefV1{Owner: owner, ID: c.Ref.ID, Revision: c.Ref.Revision, Epoch: c.Ref.Epoch}, HostID: c.HostID, StartID: c.StartID, SystemReady: runtimeports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "2.0.0", ID: c.FactRef.ID, Revision: c.FactRef.Revision, Digest: c.FactRef.Digest, ExpiresUnixNano: c.FactRef.ExpiresUnixNano}, State: state, CheckedUnixNano: c.CheckedUnixNano, ExpiresUnixNano: c.ExpiresUnixNano})
}
