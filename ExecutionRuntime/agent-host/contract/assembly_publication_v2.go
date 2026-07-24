package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const AssemblyPublicationAdapterContractVersionV2 = "praxis.agent-host/assembly-publication-adapter/v2"

type AssemblyPublicationRequestV2 struct {
	ContractVersion          string                                                   `json:"contract_version"`
	HostID                   string                                                   `json:"host_id"`
	StartID                  string                                                   `json:"start_id"`
	AttemptID                string                                                   `json:"attempt_id"`
	Artifacts                CompiledAssemblyArtifactsV2                              `json:"artifacts"`
	ExpectedCurrent          assemblycontract.AssemblyPublicationCurrentExpectationV2 `json:"expected_current"`
	RequestedExpiresUnixNano int64                                                    `json:"requested_expires_unix_nano"`
}

func (r AssemblyPublicationRequestV2) ValidateAt(now time.Time) error {
	if r.ContractVersion != AssemblyPublicationAdapterContractVersionV2 {
		return NewError(ErrorInvalidArgument, "assembly_publication_contract_invalid", "Assembly publication Adapter contract is unsupported")
	}
	for field, value := range map[string]string{"host id": r.HostID, "start id": r.StartID, "attempt id": r.AttemptID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if err := r.Artifacts.ValidateAt(now); err != nil {
		return err
	}
	if err := r.ExpectedCurrent.Validate(); err != nil {
		return ownerContractErrorV2(err, "assembly_publication_expected_invalid")
	}
	if r.RequestedExpiresUnixNano <= now.UnixNano() || r.RequestedExpiresUnixNano > r.Artifacts.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "assembly_publication_window_invalid", "Assembly publication requested window exceeds its compiled current inputs")
	}
	return nil
}

func (r AssemblyPublicationRequestV2) DigestV2() (DigestV1, error) {
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   AssemblyPublicationRequestV2 `json:"body"`
	}{Domain: "praxis.agent-host.assembly-publication-adapter", Type: "AssemblyPublicationRequestV2", Body: r})
}

type AssemblyPublicationResultV2 struct {
	ContractVersion string                         `json:"contract_version"`
	OwnerCurrent    runtimeports.OwnerCurrentRefV1 `json:"owner_current"`
	Publication     ExactRefV1                     `json:"publication"`
	Generation      ExactRefV1                     `json:"generation"`
	Manifest        ExactRefV1                     `json:"manifest"`
	Graph           ExactRefV1                     `json:"graph"`
	Handoff         ExactRefV1                     `json:"handoff"`
	Recovered       bool                           `json:"recovered"`
}

func (r AssemblyPublicationResultV2) ValidateAt(now time.Time) error {
	if r.ContractVersion != AssemblyPublicationAdapterContractVersionV2 || now.IsZero() {
		return NewError(ErrorInvalidArgument, "assembly_publication_result_invalid", "Assembly publication result is incomplete")
	}
	if err := r.OwnerCurrent.Validate(); err != nil {
		return ownerContractErrorV2(err, "assembly_owner_current_invalid")
	}
	for _, ref := range []ExactRefV1{r.Publication, r.Generation, r.Manifest, r.Graph, r.Handoff} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if now.UnixNano() >= r.OwnerCurrent.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "assembly_owner_current_expired", "Assembly publication owner current expired")
	}
	return nil
}

func ownerContractErrorV2(err error, reason string) error {
	if err == nil {
		return nil
	}
	return NewError(ErrorInvalidArgument, reason, "referenced owner public contract rejected the value")
}

func AssemblyPublicationOwnerCurrentV2(owner core.OwnerRef, current assemblycontract.AssemblyPublicationCurrentV2) (runtimeports.OwnerCurrentRefV1, error) {
	value := runtimeports.OwnerCurrentRefV1{Owner: owner, ContractVersion: assemblycontract.PublicationContractVersionV2, ID: current.ScopeRef, Revision: current.Revision, Digest: current.Digest, ExpiresUnixNano: current.ExpiresUnixNano}
	if err := value.Validate(); err != nil {
		return runtimeports.OwnerCurrentRefV1{}, ownerContractErrorV2(err, "assembly_owner_current_invalid")
	}
	return value, nil
}
