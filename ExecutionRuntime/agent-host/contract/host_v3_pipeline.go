package contract

import (
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const HostV3OwnerPipelineContractVersionV1 = "praxis.agent-host/owner-pipeline/v1"

// HostV3OwnerStartProjectionV1 is an internal composition result. It does not
// create a second Current: every field is an exact Ref returned by its Owner.
type HostV3OwnerStartProjectionV1 struct {
	ContractVersion  string                                       `json:"contract_version"`
	HostID           string                                       `json:"host_id"`
	StartID          string                                       `json:"start_id"`
	RequestDigest    DigestV1                                     `json:"request_digest"`
	Journal          ExactRefV1                                   `json:"journal"`
	CleanupClosure   ExactRefV1                                   `json:"cleanup_closure"`
	Ready            SystemReadyCurrentRefV2                      `json:"ready"`
	Availability     runtimeports.AgentExecutionAvailabilityRefV1 `json:"availability"`
	CheckedUnixNano  int64                                        `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                        `json:"expires_unix_nano"`
	ProjectionDigest DigestV1                                     `json:"projection_digest"`
}

func (p HostV3OwnerStartProjectionV1) digestV1() (DigestV1, error) {
	p.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   HostV3OwnerStartProjectionV1 `json:"body"`
	}{"praxis.agent-host.owner-pipeline", "HostV3OwnerStartProjectionV1", p})
}

func SealHostV3OwnerStartProjectionV1(p HostV3OwnerStartProjectionV1) (HostV3OwnerStartProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != HostV3OwnerPipelineContractVersionV1 {
		return HostV3OwnerStartProjectionV1{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Owner pipeline projection version drifted")
	}
	p.ContractVersion = HostV3OwnerPipelineContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.digestV1()
	if err != nil {
		return HostV3OwnerStartProjectionV1{}, err
	}
	if provided != "" && provided != d {
		return HostV3OwnerStartProjectionV1{}, NewError(ErrorConflict, "host_v3_owner_projection_drift", "HostV3 Owner projection supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}

func (p HostV3OwnerStartProjectionV1) Validate() error {
	if p.ContractVersion != HostV3OwnerPipelineContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_v3_owner_projection_incomplete", "HostV3 Owner projection is incomplete")
	}
	if err := ValidateIdentifierV1("host id", p.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", p.StartID); err != nil {
		return err
	}
	if err := p.RequestDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []ExactRefV1{p.Journal, p.CleanupClosure} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if p.CleanupClosure.Kind != HostCleanupClosureRefKindV2 {
		return NewError(ErrorConflict, "host_v3_cleanup_closure_kind_drift", "HostV3 Owner projection does not name a Cleanup Closure")
	}
	if err := p.Ready.Validate(); err != nil {
		return err
	}
	if err := p.Availability.Validate(); err != nil {
		return err
	}
	if p.Ready.ID != p.Availability.ID || p.Ready.Revision != p.Availability.Revision || p.Ready.Epoch != p.Availability.Epoch || p.Ready.ExpiresUnixNano != p.Availability.ExpiresUnixNano {
		return NewError(ErrorConflict, "host_v3_availability_projection_drift", "HostV3 Ready and Availability coordinates drifted")
	}
	minimum := p.Ready.ExpiresUnixNano
	if p.Availability.ExpiresUnixNano < minimum {
		minimum = p.Availability.ExpiresUnixNano
	}
	if p.ExpiresUnixNano != minimum {
		return NewError(ErrorConflict, "host_v3_owner_projection_expiry_drift", "HostV3 Owner projection exceeds an exact Owner window")
	}
	d, err := p.digestV1()
	if err != nil || d != p.ProjectionDigest {
		return NewError(ErrorConflict, "host_v3_owner_projection_drift", "HostV3 Owner projection digest drifted")
	}
	return nil
}

func (p HostV3OwnerStartProjectionV1) ValidateFor(request StartRequestV3, claim HostStartClaimRefV1, journal ExactRefV1, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.HostID != request.Config.HostID || p.StartID != request.StartID || p.RequestDigest != request.RequestDigest || p.Journal != journal || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || claim.HostID != p.HostID || claim.StartID != p.StartID {
		return NewError(ErrorConflict, "host_v3_owner_projection_request_drift", "HostV3 Owner projection does not bind the exact Start")
	}
	return nil
}

func (p HostV3OwnerStartProjectionV1) ValidateForOrigin(binding HostStartClaimInputBindingV3, journal ExactRefV1, now time.Time) error {
	if err := binding.ValidateV3(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.HostID != binding.Input.HostID || p.StartID != binding.Input.StartID || p.Journal != journal || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return NewError(ErrorConflict, "host_v3_owner_projection_origin_drift", "HostV3 Owner projection does not bind the exact persisted origin")
	}
	return nil
}

type HostV3OwnerStopProjectionV1 struct {
	ContractVersion  string         `json:"contract_version"`
	RequestDigest    DigestV1       `json:"request_digest"`
	Journal          ExactRefV1     `json:"journal"`
	CleanupClosure   ExactRefV1     `json:"cleanup_closure"`
	CleanupResult    ExactRefV1     `json:"cleanup_result"`
	State            CleanupStateV1 `json:"state"`
	CheckedUnixNano  int64          `json:"checked_unix_nano"`
	ProjectionDigest DigestV1       `json:"projection_digest"`
}

func (p HostV3OwnerStopProjectionV1) digestV1() (DigestV1, error) {
	p.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                      `json:"domain"`
		Type   string                      `json:"type"`
		Body   HostV3OwnerStopProjectionV1 `json:"body"`
	}{"praxis.agent-host.owner-pipeline", "HostV3OwnerStopProjectionV1", p})
}
func SealHostV3OwnerStopProjectionV1(p HostV3OwnerStopProjectionV1) (HostV3OwnerStopProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != HostV3OwnerPipelineContractVersionV1 {
		return HostV3OwnerStopProjectionV1{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV3 Owner Stop projection version drifted")
	}
	p.ContractVersion = HostV3OwnerPipelineContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, err := p.digestV1()
	if err != nil {
		return HostV3OwnerStopProjectionV1{}, err
	}
	if provided != "" && provided != d {
		return HostV3OwnerStopProjectionV1{}, NewError(ErrorConflict, "host_v3_owner_stop_projection_drift", "HostV3 Owner Stop projection supplied a wrong digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}
func (p HostV3OwnerStopProjectionV1) Validate() error {
	if p.ContractVersion != HostV3OwnerPipelineContractVersionV1 || p.CheckedUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "host_v3_owner_stop_projection_incomplete", "HostV3 Owner Stop projection is incomplete")
	}
	if err := p.RequestDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []ExactRefV1{p.Journal, p.CleanupClosure, p.CleanupResult} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if p.CleanupClosure.Kind != HostCleanupClosureRefKindV2 {
		return NewError(ErrorConflict, "host_v3_cleanup_closure_kind_drift", "HostV3 Stop does not bind a Cleanup Closure")
	}
	if p.State != CleanupClosedV1 && p.State != CleanupResidualV1 && p.State != CleanupIndeterminateV1 {
		return NewError(ErrorInvalidArgument, "host_v3_stop_state_invalid", "HostV3 Owner Stop state is invalid")
	}
	d, err := p.digestV1()
	if err != nil || d != p.ProjectionDigest {
		return NewError(ErrorConflict, "host_v3_owner_stop_projection_drift", "HostV3 Owner Stop projection digest drifted")
	}
	return nil
}
func (p HostV3OwnerStopProjectionV1) ValidateFor(r StopRequestV3) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.RequestDigest != r.RequestDigest || p.CleanupClosure != r.CleanupClosure {
		return NewError(ErrorConflict, "host_v3_owner_stop_projection_request_drift", "HostV3 Owner Stop projection does not bind the exact request")
	}
	return nil
}
