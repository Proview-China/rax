package contract

import (
	"sort"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const HostLifecycleContractVersionV2 = "praxis.agent-host/lifecycle/v2"

// StartRequestV2 is the additive declarative HostV2 entry point. The opaque
// HostConfig remains HostV1-compatible, while the exact source Ref prevents a
// caller from changing the Definition behind the stable configured source.
type StartRequestV2 struct {
	ContractVersion           string       `json:"contract_version"`
	StartID                   string       `json:"start_id"`
	Config                    HostConfigV1 `json:"config"`
	DefinitionSourceCurrent   ExactRefV1   `json:"definition_source_current"`
	RequestedAtUnixNano       int64        `json:"requested_at_unix_nano"`
	RequestedNotAfterUnixNano int64        `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1     `json:"request_digest"`
}

func (r StartRequestV2) canonicalV2() StartRequestV2 {
	r.Config = r.Config.CanonicalV1()
	return r
}

func (r StartRequestV2) DigestV2() (DigestV1, error) {
	clone := r.canonicalV2()
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string         `json:"domain"`
		Type   string         `json:"type"`
		Body   StartRequestV2 `json:"body"`
	}{Domain: "praxis.agent-host.lifecycle-v2", Type: "StartRequestV2", Body: clone})
}

func SealStartRequestV2(r StartRequestV2) (StartRequestV2, error) {
	if r.ContractVersion != "" && r.ContractVersion != HostLifecycleContractVersionV2 {
		return StartRequestV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "HostV2 Start request version drifted")
	}
	r.ContractVersion = HostLifecycleContractVersionV2
	r = r.canonicalV2()
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return StartRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return StartRequestV2{}, NewError(ErrorConflict, "host_v2_start_request_drift", "HostV2 Start request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

func (r StartRequestV2) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV2 || r.RequestedAtUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedAtUnixNano {
		return NewError(ErrorInvalidArgument, "host_v2_start_request_incomplete", "HostV2 Start request is incomplete")
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if err := r.Config.Validate(); err != nil {
		return err
	}
	if err := r.DefinitionSourceCurrent.Validate(); err != nil {
		return err
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.RequestDigest {
		return NewError(ErrorConflict, "host_v2_start_request_drift", "HostV2 Start request digest drifted")
	}
	return nil
}

func (r StartRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedAtUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "HostV2 Start request was checked before its creation watermark")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return NewError(ErrorPrecondition, "host_v2_start_request_expired", "HostV2 Start request expired")
	}
	return nil
}

func (r StartRequestV2) ClaimV1() (HostStartClaimV1, error) {
	if err := r.Validate(); err != nil {
		return HostStartClaimV1{}, err
	}
	configDigest, err := r.Config.DigestV1()
	if err != nil {
		return HostStartClaimV1{}, err
	}
	return SealHostStartClaimV1(HostStartClaimV1{
		ContractVersion:     HostStartClaimContractVersionV1,
		HostContractVersion: ContractVersionV2,
		HostID:              r.Config.HostID,
		StartID:             r.StartID,
		ConfigDigest:        configDigest,
		DefinitionSourceRef: r.DefinitionSourceCurrent,
		RequestedOperation:  HostStartOperationStartV1,
		CreatedUnixNano:     r.RequestedAtUnixNano,
		ExpiresUnixNano:     r.RequestedNotAfterUnixNano,
	})
}

// HostV2StartOutputsV2 contains only public Owner results. It is a recovered
// orchestration view, never a second authoritative store.
type HostV2StartOutputsV2 struct {
	Definition            DecodedDefinitionV1                            `json:"definition"`
	Resolved              ResolvedAssemblyV1                             `json:"resolved"`
	Compiled              CompiledAssemblyArtifactsV2                    `json:"compiled"`
	Assembly              AssemblyPublicationResultV2                    `json:"assembly"`
	Binding               runtimeports.BindingAdmissionResultV1          `json:"binding"`
	Controls              []ControlAdapterInstanceV2                     `json:"controls"`
	Activation            applicationcontract.AgentActivationResultV1    `json:"activation"`
	GenerationAssociation runtimeports.GenerationBindingAssociationRefV1 `json:"generation_association"`
	Ready                 SystemReadyGatewayResultV2                     `json:"ready"`
}

func (o HostV2StartOutputsV2) ValidateAt(now time.Time) error {
	if err := o.Definition.Validate(); err != nil {
		return err
	}
	if err := o.Resolved.Validate(); err != nil {
		return err
	}
	if err := o.Compiled.ValidateAt(now); err != nil {
		return err
	}
	if err := o.Assembly.ValidateAt(now); err != nil {
		return err
	}
	if err := o.Binding.Validate(); err != nil {
		return err
	}
	if len(o.Controls) == 0 {
		return NewError(ErrorPrecondition, "host_v2_control_set_missing", "HostV2 requires the complete control adapter set")
	}
	for index, control := range o.Controls {
		if err := control.Validate(); err != nil {
			return err
		}
		if index > 0 && o.Controls[index-1].DescriptorRef.FactoryID >= control.DescriptorRef.FactoryID {
			return NewError(ErrorConflict, "host_v2_control_set_not_canonical", "HostV2 control adapters must be sorted and unique by exact factory")
		}
	}
	if err := o.Activation.Validate(); err != nil {
		return err
	}
	if err := o.GenerationAssociation.Validate(); err != nil {
		return err
	}
	return o.Ready.Validate()
}

type StartResultV2 struct {
	ContractVersion         string               `json:"contract_version"`
	HostID                  string               `json:"host_id"`
	StartID                 string               `json:"start_id"`
	RequestDigest           DigestV1             `json:"request_digest"`
	RequestNotAfterUnixNano int64                `json:"request_not_after_unix_nano"`
	Journal                 ExactRefV1           `json:"journal"`
	Outputs                 HostV2StartOutputsV2 `json:"outputs"`
	CheckedUnixNano         int64                `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                `json:"expires_unix_nano"`
	ResultDigest            DigestV1             `json:"result_digest"`
}

func (r StartResultV2) DigestV2() (DigestV1, error) {
	clone := r
	clone.Outputs.Controls = append([]ControlAdapterInstanceV2{}, r.Outputs.Controls...)
	sort.Slice(clone.Outputs.Controls, func(i, j int) bool {
		return clone.Outputs.Controls[i].DescriptorRef.FactoryID < clone.Outputs.Controls[j].DescriptorRef.FactoryID
	})
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   StartResultV2 `json:"body"`
	}{Domain: "praxis.agent-host.lifecycle-v2", Type: "StartResultV2", Body: clone})
}

func SealStartResultV2(r StartResultV2) (StartResultV2, error) {
	r.ContractVersion = HostLifecycleContractVersionV2
	r.Outputs.Controls = append([]ControlAdapterInstanceV2{}, r.Outputs.Controls...)
	sort.Slice(r.Outputs.Controls, func(i, j int) bool {
		return r.Outputs.Controls[i].DescriptorRef.FactoryID < r.Outputs.Controls[j].DescriptorRef.FactoryID
	})
	provided := r.ResultDigest
	r.ResultDigest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return StartResultV2{}, err
	}
	if provided != "" && provided != digest {
		return StartResultV2{}, NewError(ErrorConflict, "host_v2_start_result_drift", "HostV2 Start result supplied a wrong digest")
	}
	r.ResultDigest = digest
	return r, r.Validate()
}

func (r StartResultV2) Validate() error {
	if r.ContractVersion != HostLifecycleContractVersionV2 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_v2_start_result_incomplete", "HostV2 Start result is incomplete")
	}
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := r.Journal.Validate(); err != nil {
		return err
	}
	if err := r.Outputs.ValidateAt(time.Unix(0, r.CheckedUnixNano)); err != nil {
		return err
	}
	minimum := r.RequestNotAfterUnixNano
	if minimum <= r.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "host_v2_start_result_request_window_invalid", "HostV2 Start result request window is invalid")
	}
	values := []int64{r.Outputs.Compiled.ExpiresUnixNano, r.Outputs.Assembly.OwnerCurrent.ExpiresUnixNano, r.Outputs.Binding.ExpiresUnixNano, r.Outputs.Activation.ExpiresUnixNano}
	values = append(values, r.Outputs.Ready.Fact.ExpiresUnixNano)
	for _, control := range r.Outputs.Controls {
		values = append(values, control.ExpiresUnixNano)
	}
	for _, value := range values {
		if value < minimum {
			minimum = value
		}
	}
	if r.ExpiresUnixNano != minimum {
		return NewError(ErrorConflict, "host_v2_start_result_expiry_drift", "HostV2 Start result expiry is not the exact minimum Owner window")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.ResultDigest {
		return NewError(ErrorConflict, "host_v2_start_result_drift", "HostV2 Start result digest drifted")
	}
	return nil
}

func (r StartResultV2) ValidateFor(request StartRequestV2, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.HostID != request.Config.HostID || r.StartID != request.StartID || r.RequestDigest != request.RequestDigest || r.RequestNotAfterUnixNano != request.RequestedNotAfterUnixNano || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano || now.UnixNano() < r.CheckedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return NewError(ErrorConflict, "host_v2_start_result_request_drift", "HostV2 Start result is not current for the exact request")
	}
	return nil
}

type InspectRequestV2 struct {
	HostID        string     `json:"host_id"`
	StartID       string     `json:"start_id"`
	StartClaimRef ExactRefV1 `json:"start_claim_ref"`
	RequestDigest DigestV1   `json:"request_digest"`
}

func (r InspectRequestV2) Validate() error {
	if err := ValidateIdentifierV1("host id", r.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", r.StartID); err != nil {
		return err
	}
	if err := r.StartClaimRef.Validate(); err != nil {
		return err
	}
	return r.RequestDigest.Validate()
}

type InspectResultV2 struct {
	ContractVersion string         `json:"contract_version"`
	Journal         HostJournalV2  `json:"journal"`
	Ready           *StartResultV2 `json:"ready,omitempty"`
	Phase           HostPhaseV2    `json:"phase"`
}

func HostV2OwnerCurrentCoordinate(ref runtimeports.OwnerCurrentRefV1) HostOperationCoordinateV2 {
	return HostOperationCoordinateV2{ContractKind: ref.ContractVersion, OwnerID: string(ref.Owner.ID), ID: ref.ID, Revision: uint64(ref.Revision), Digest: DigestV1(ref.Digest), Current: true, ExpiresUnixNano: ref.ExpiresUnixNano}
}

func HostV2ExactCoordinate(kind, owner string, ref ExactRefV1) HostOperationCoordinateV2 {
	return HostOperationCoordinateV2{ContractKind: kind, OwnerID: owner, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
}

func HostV2RuntimeRefCoordinate(kind, owner, id string, revision core.Revision, digest core.Digest) HostOperationCoordinateV2 {
	return HostOperationCoordinateV2{ContractKind: kind, OwnerID: owner, ID: id, Revision: uint64(revision), Digest: DigestV1(digest)}
}
