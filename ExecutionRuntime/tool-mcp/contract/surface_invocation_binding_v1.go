package contract

import (
	"context"
	"strings"
	"time"

	modelcontract "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolSurfaceInvocationBindingContractVersionV1 = "praxis.tool.surface-invocation-binding/v1"

const ToolSurfaceInvocationBindingAckKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/surface-invocation-binding-ack-v1"

const toolSurfaceInvocationBindingCanonicalDomainV1 = "praxis.tool"

type ToolSurfaceInvocationCoordinateV1 struct {
	InvocationID     string      `json:"invocation_id"`
	InvocationDigest core.Digest `json:"invocation_digest"`
}

func (c ToolSurfaceInvocationCoordinateV1) Validate() error {
	if strings.TrimSpace(c.InvocationID) == "" || len(c.InvocationID) > MaxStringBytes {
		return invalid("Tool Surface Invocation coordinate ID is invalid")
	}
	return c.InvocationDigest.Validate()
}

type ToolSurfaceInvocationBindingSubjectV1 struct {
	Invocation                         ToolSurfaceInvocationCoordinateV1                        `json:"invocation"`
	PreparedFactRef                    modelcontract.PreparedModelInvocationRefV1               `json:"prepared_fact_ref"`
	PreparedHistoricalFact             modelcontract.PreparedModelInvocationFactV1              `json:"prepared_historical_fact"`
	PreparedCurrentRef                 modelcontract.PreparedModelInvocationCurrentRefV1        `json:"prepared_current_ref"`
	PreparedCurrent                    modelcontract.PreparedModelInvocationCurrentProjectionV1 `json:"prepared_current"`
	SurfaceCurrent                     ToolSurfaceManifestCurrentProjectionV1                   `json:"surface_current"`
	AssemblyCurrentRef                 runtimeports.ModelPreDispatchAssemblyCurrentRefV1        `json:"assembly_current_ref"`
	AssemblyRegistrySnapshot           runtimeports.RegistrySnapshotRefV1                       `json:"assembly_registry_snapshot"`
	AssemblyCurrent                    runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1 `json:"assembly_current"`
	ToolExpectedInjectionDigest        core.Digest                                              `json:"tool_expected_injection_digest"`
	ModelActualToolSurfaceDigest       core.Digest                                              `json:"model_actual_tool_surface_digest"`
	ModelActualProviderInjectionDigest core.Digest                                              `json:"model_actual_provider_injection_digest"`
	ProfileDigest                      core.Digest                                              `json:"profile_digest"`
	RequestedNotAfterUnixNano          int64                                                    `json:"requested_not_after_unix_nano"`
	Digest                             core.Digest                                              `json:"digest"`
}

func (s ToolSurfaceInvocationBindingSubjectV1) Validate() error {
	if err := validateToolSurfaceInvocationInputsV1(s.Invocation, s.PreparedFactRef, s.PreparedHistoricalFact, s.PreparedCurrentRef, s.PreparedCurrent, s.SurfaceCurrent, s.AssemblyCurrentRef, s.AssemblyRegistrySnapshot, s.AssemblyCurrent, s.RequestedNotAfterUnixNano); err != nil {
		return err
	}
	recomputed, err := ComputeExpectedInjectionDigest(s.SurfaceCurrent.Manifest.Entries)
	if err != nil || recomputed != s.ToolExpectedInjectionDigest || s.ToolExpectedInjectionDigest != s.ModelActualToolSurfaceDigest || s.ModelActualToolSurfaceDigest != s.PreparedHistoricalFact.ActualToolSurfaceDigest || s.ModelActualToolSurfaceDigest != s.PreparedCurrent.ActualToolSurfaceDigest {
		return conflict("Tool Surface Invocation expected injection closure drifted")
	}
	if s.ModelActualProviderInjectionDigest != s.PreparedHistoricalFact.ActualProviderInjectionDigest || s.ModelActualProviderInjectionDigest != s.PreparedCurrent.ActualProviderInjectionDigest {
		return conflict("Tool Surface Invocation provider injection digest drifted")
	}
	if s.ProfileDigest != s.PreparedHistoricalFact.ProfileDigest || s.ProfileDigest != s.AssemblyCurrent.ProfileDigest {
		return conflict("Tool Surface Invocation Profile digest drifted")
	}
	for _, digest := range []core.Digest{s.ToolExpectedInjectionDigest, s.ModelActualToolSurfaceDigest, s.ModelActualProviderInjectionDigest, s.ProfileDigest, s.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Tool Surface Invocation Binding Subject digest drifted")
	}
	return nil
}

func (s ToolSurfaceInvocationBindingSubjectV1) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationBindingSubjectV1", s)
}

type ToolSurfaceInvocationBindingRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolSurfaceInvocationBindingRefV1) Validate() error {
	if r.Owner.Validate() != nil || r.ContractVersion != ToolSurfaceInvocationBindingContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("Tool Surface Invocation Binding Ref is invalid")
	}
	return r.Digest.Validate()
}

func (r ToolSurfaceInvocationBindingRefV1) ModelRefV1() modelcontract.PreparedModelInvocationSurfaceBindingRefV1 {
	return modelcontract.PreparedModelInvocationSurfaceBindingRefV1{Owner: r.Owner, ContractVersion: r.ContractVersion, ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

type ToolSurfaceInvocationBindingV1 struct {
	ContractVersion  string                                `json:"contract_version"`
	Ref              ToolSurfaceInvocationBindingRefV1     `json:"ref"`
	Subject          ToolSurfaceInvocationBindingSubjectV1 `json:"subject"`
	CreatedUnixNano  int64                                 `json:"created_unix_nano"`
	NotAfterUnixNano int64                                 `json:"not_after_unix_nano"`
	Digest           core.Digest                           `json:"digest"`
}

func (b ToolSurfaceInvocationBindingV1) Validate() error {
	if b.ContractVersion != ToolSurfaceInvocationBindingContractVersionV1 || b.Ref.ContractVersion != b.ContractVersion || b.CreatedUnixNano <= 0 || b.CreatedUnixNano >= b.NotAfterUnixNano {
		return invalid("Tool Surface Invocation Binding identity or lifetime is invalid")
	}
	if err := b.Ref.Validate(); err != nil {
		return err
	}
	if err := b.Subject.Validate(); err != nil {
		return err
	}
	expectedID, err := DeriveToolSurfaceInvocationBindingIDV1(b.Subject.Invocation)
	if err != nil || expectedID != b.Ref.ID {
		return conflict("Tool Surface Invocation Binding ID drifted")
	}
	if b.Digest != b.Ref.Digest {
		return conflict("Tool Surface Invocation Binding Ref and top-level digest drifted")
	}
	for _, checked := range []int64{b.Subject.PreparedCurrent.CheckedUnixNano, b.Subject.SurfaceCurrent.CheckedUnixNano, b.Subject.AssemblyCurrent.CheckedUnixNano} {
		if b.CreatedUnixNano < checked {
			return conflict("Tool Surface Invocation Binding was created before an Owner current projection")
		}
	}
	digest, err := b.ComputeDigest()
	if err != nil || digest != b.Digest {
		return conflict("Tool Surface Invocation Binding digest drifted")
	}
	if b.NotAfterUnixNano > toolSurfaceInvocationNotAfterV1(b.Subject, time.Time{}) {
		return conflict("Tool Surface Invocation Binding exceeds an Owner current upper bound")
	}
	return nil
}

func (b ToolSurfaceInvocationBindingV1) ValidateCurrent(now time.Time) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < b.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding current clock regressed")
	}
	if !now.Before(time.Unix(0, b.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Invocation Binding expired")
	}
	return nil
}

func (b ToolSurfaceInvocationBindingV1) ComputeDigest() (core.Digest, error) {
	b.Ref.Digest = ""
	b.Digest = ""
	return core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationBindingV1", b)
}

func SealToolSurfaceInvocationBindingV1(b ToolSurfaceInvocationBindingV1) (ToolSurfaceInvocationBindingV1, error) {
	if b.ContractVersion != "" && b.ContractVersion != ToolSurfaceInvocationBindingContractVersionV1 {
		return ToolSurfaceInvocationBindingV1{}, invalid("Tool Surface Invocation Binding contract version drifted")
	}
	b.ContractVersion = ToolSurfaceInvocationBindingContractVersionV1
	if err := b.Subject.Validate(); err != nil {
		return ToolSurfaceInvocationBindingV1{}, err
	}
	if b.Ref.Owner.Validate() != nil {
		return ToolSurfaceInvocationBindingV1{}, invalid("Tool Surface Invocation Binding Owner is required")
	}
	b.Ref.ContractVersion = b.ContractVersion
	expectedID, err := DeriveToolSurfaceInvocationBindingIDV1(b.Subject.Invocation)
	if err != nil {
		return ToolSurfaceInvocationBindingV1{}, err
	}
	if b.Ref.ID != "" && b.Ref.ID != expectedID {
		return ToolSurfaceInvocationBindingV1{}, conflict("supplied Tool Surface Invocation Binding ID drifted")
	}
	b.Ref.ID = expectedID
	if b.Ref.Revision != 0 && b.Ref.Revision != 1 {
		return ToolSurfaceInvocationBindingV1{}, conflict("supplied Tool Surface Invocation Binding revision drifted")
	}
	b.Ref.Revision = 1
	providedRefDigest, providedDigest := b.Ref.Digest, b.Digest
	b.Ref.Digest, b.Digest = "", ""
	digest, err := b.ComputeDigest()
	if err != nil {
		return ToolSurfaceInvocationBindingV1{}, err
	}
	for _, provided := range []core.Digest{providedRefDigest, providedDigest} {
		if provided != "" && provided != digest {
			return ToolSurfaceInvocationBindingV1{}, conflict("supplied Tool Surface Invocation Binding digest drifted")
		}
	}
	b.Ref.Digest, b.Digest = digest, digest
	return b, b.Validate()
}

type ToolSurfaceInvocationBindingAckRefV1 struct {
	Kind       runtimeports.NamespacedNameV2     `json:"kind"`
	ID         string                            `json:"id"`
	Revision   core.Revision                     `json:"revision"`
	Digest     core.Digest                       `json:"digest"`
	BindingRef ToolSurfaceInvocationBindingRefV1 `json:"binding_ref"`
}

func (r ToolSurfaceInvocationBindingAckRefV1) Validate() error {
	if r.Kind != ToolSurfaceInvocationBindingAckKindV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.BindingRef.Validate() != nil {
		return invalid("Tool Surface Invocation Binding Ack Ref is invalid")
	}
	return r.Digest.Validate()
}

type ToolSurfaceInvocationBindingAckV1 struct {
	ContractVersion    string                                            `json:"contract_version"`
	Ref                ToolSurfaceInvocationBindingAckRefV1              `json:"ref"`
	BindingRef         ToolSurfaceInvocationBindingRefV1                 `json:"binding_ref"`
	Invocation         ToolSurfaceInvocationCoordinateV1                 `json:"invocation"`
	PreparedFactRef    modelcontract.PreparedModelInvocationRefV1        `json:"prepared_fact_ref"`
	PreparedCurrentRef modelcontract.PreparedModelInvocationCurrentRefV1 `json:"prepared_current_ref"`
	CheckedUnixNano    int64                                             `json:"checked_unix_nano"`
	NotAfterUnixNano   int64                                             `json:"not_after_unix_nano"`
	Digest             core.Digest                                       `json:"digest"`
}

func (a ToolSurfaceInvocationBindingAckV1) Validate() error {
	if a.ContractVersion != ToolSurfaceInvocationBindingContractVersionV1 || a.Ref.Validate() != nil || a.BindingRef.Validate() != nil || a.Invocation.Validate() != nil || a.PreparedFactRef.Validate() != nil || a.PreparedCurrentRef.Validate() != nil || a.CheckedUnixNano <= 0 || a.CheckedUnixNano >= a.NotAfterUnixNano {
		return invalid("Tool Surface Invocation Binding Ack is invalid")
	}
	if a.Ref.BindingRef != a.BindingRef || a.Ref.Digest != a.Digest {
		return conflict("Tool Surface Invocation Binding Ack duplicate fields drifted")
	}
	expectedID, err := DeriveToolSurfaceInvocationBindingAckIDV1(a.BindingRef, a.PreparedFactRef, a.PreparedCurrentRef)
	if err != nil || expectedID != a.Ref.ID {
		return conflict("Tool Surface Invocation Binding Ack ID drifted")
	}
	digest, err := a.ComputeDigest()
	if err != nil || digest != a.Digest {
		return conflict("Tool Surface Invocation Binding Ack digest drifted")
	}
	return nil
}

func (a ToolSurfaceInvocationBindingAckV1) ComputeDigest() (core.Digest, error) {
	a.Ref.Digest = ""
	a.Digest = ""
	return core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationBindingAckV1", a)
}

func (a ToolSurfaceInvocationBindingAckV1) ValidateAgainst(b ToolSurfaceInvocationBindingV1, now time.Time) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if a.BindingRef != b.Ref || a.Invocation != b.Subject.Invocation || a.PreparedFactRef != b.Subject.PreparedFactRef || a.PreparedCurrentRef != b.Subject.PreparedCurrentRef || a.NotAfterUnixNano != b.NotAfterUnixNano || a.CheckedUnixNano < b.CreatedUnixNano {
		return conflict("Tool Surface Invocation Binding Ack does not close its Binding")
	}
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding Ack validation requires time")
	}
	if now.UnixNano() < a.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding Ack clock regressed")
	}
	if !now.Before(time.Unix(0, a.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Invocation Binding Ack expired")
	}
	return nil
}

func SealToolSurfaceInvocationBindingAckV1(a ToolSurfaceInvocationBindingAckV1) (ToolSurfaceInvocationBindingAckV1, error) {
	if a.ContractVersion != "" && a.ContractVersion != ToolSurfaceInvocationBindingContractVersionV1 {
		return ToolSurfaceInvocationBindingAckV1{}, invalid("Tool Surface Invocation Binding Ack contract version drifted")
	}
	a.ContractVersion = ToolSurfaceInvocationBindingContractVersionV1
	a.Ref.Kind = ToolSurfaceInvocationBindingAckKindV1
	a.Ref.BindingRef = a.BindingRef
	expectedID, err := DeriveToolSurfaceInvocationBindingAckIDV1(a.BindingRef, a.PreparedFactRef, a.PreparedCurrentRef)
	if err != nil {
		return ToolSurfaceInvocationBindingAckV1{}, err
	}
	if a.Ref.ID != "" && a.Ref.ID != expectedID {
		return ToolSurfaceInvocationBindingAckV1{}, conflict("supplied Tool Surface Invocation Binding Ack ID drifted")
	}
	a.Ref.ID = expectedID
	if a.Ref.Revision != 0 && a.Ref.Revision != 1 {
		return ToolSurfaceInvocationBindingAckV1{}, conflict("supplied Tool Surface Invocation Binding Ack revision drifted")
	}
	a.Ref.Revision = 1
	providedRefDigest, providedDigest := a.Ref.Digest, a.Digest
	a.Ref.Digest, a.Digest = "", ""
	digest, err := a.ComputeDigest()
	if err != nil {
		return ToolSurfaceInvocationBindingAckV1{}, err
	}
	for _, provided := range []core.Digest{providedRefDigest, providedDigest} {
		if provided != "" && provided != digest {
			return ToolSurfaceInvocationBindingAckV1{}, conflict("supplied Tool Surface Invocation Binding Ack digest drifted")
		}
	}
	a.Ref.Digest, a.Digest = digest, digest
	return a, a.Validate()
}

type ToolSurfaceInvocationBindingEnsureRequestV1 struct {
	Invocation                ToolSurfaceInvocationCoordinateV1                        `json:"invocation"`
	PreparedFactRef           modelcontract.PreparedModelInvocationRefV1               `json:"prepared_fact_ref"`
	PreparedHistoricalFact    modelcontract.PreparedModelInvocationFactV1              `json:"prepared_historical_fact"`
	PreparedCurrentRef        modelcontract.PreparedModelInvocationCurrentRefV1        `json:"prepared_current_ref"`
	PreparedCurrent           modelcontract.PreparedModelInvocationCurrentProjectionV1 `json:"prepared_current"`
	SurfaceCurrent            ToolSurfaceManifestCurrentProjectionV1                   `json:"surface_current"`
	AssemblyCurrentRef        runtimeports.ModelPreDispatchAssemblyCurrentRefV1        `json:"assembly_current_ref"`
	AssemblyRegistrySnapshot  runtimeports.RegistrySnapshotRefV1                       `json:"assembly_registry_snapshot"`
	AssemblyCurrent           runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1 `json:"assembly_current"`
	RequestedNotAfterUnixNano int64                                                    `json:"requested_not_after_unix_nano"`
}

func (r ToolSurfaceInvocationBindingEnsureRequestV1) Validate() error {
	return validateToolSurfaceInvocationInputsV1(r.Invocation, r.PreparedFactRef, r.PreparedHistoricalFact, r.PreparedCurrentRef, r.PreparedCurrent, r.SurfaceCurrent, r.AssemblyCurrentRef, r.AssemblyRegistrySnapshot, r.AssemblyCurrent, r.RequestedNotAfterUnixNano)
}

func (r ToolSurfaceInvocationBindingEnsureRequestV1) ComputeDigest() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationBindingEnsureRequestV1", r)
}

func (r ToolSurfaceInvocationBindingEnsureRequestV1) ValidateAgainst(b ToolSurfaceInvocationBindingV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := b.Validate(); err != nil {
		return err
	}
	subject, err := SealToolSurfaceInvocationBindingSubjectV1(r)
	if err != nil {
		return err
	}
	if subject.Digest != b.Subject.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Surface Invocation Binding request differs from winner")
	}
	return nil
}

func SealToolSurfaceInvocationBindingSubjectV1(r ToolSurfaceInvocationBindingEnsureRequestV1) (ToolSurfaceInvocationBindingSubjectV1, error) {
	if err := r.Validate(); err != nil {
		return ToolSurfaceInvocationBindingSubjectV1{}, err
	}
	expected, err := ComputeExpectedInjectionDigest(r.SurfaceCurrent.Manifest.Entries)
	if err != nil {
		return ToolSurfaceInvocationBindingSubjectV1{}, err
	}
	s := ToolSurfaceInvocationBindingSubjectV1{
		Invocation: r.Invocation, PreparedFactRef: r.PreparedFactRef, PreparedHistoricalFact: r.PreparedHistoricalFact,
		PreparedCurrentRef: r.PreparedCurrentRef, PreparedCurrent: r.PreparedCurrent, SurfaceCurrent: r.SurfaceCurrent,
		AssemblyCurrentRef: r.AssemblyCurrentRef, AssemblyRegistrySnapshot: r.AssemblyRegistrySnapshot, AssemblyCurrent: r.AssemblyCurrent,
		ToolExpectedInjectionDigest: expected, ModelActualToolSurfaceDigest: r.PreparedHistoricalFact.ActualToolSurfaceDigest,
		ModelActualProviderInjectionDigest: r.PreparedHistoricalFact.ActualProviderInjectionDigest,
		ProfileDigest:                      r.PreparedHistoricalFact.ProfileDigest, RequestedNotAfterUnixNano: r.RequestedNotAfterUnixNano,
	}
	s.Digest, err = s.ComputeDigest()
	if err != nil {
		return ToolSurfaceInvocationBindingSubjectV1{}, err
	}
	return s, s.Validate()
}

type ToolSurfaceInvocationBindingWriterV1 interface {
	EnsureToolSurfaceInvocationBindingV1(context.Context, ToolSurfaceInvocationBindingEnsureRequestV1) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
}

type ToolSurfaceInvocationBindingReaderV1 interface {
	InspectToolSurfaceInvocationBindingByInvocationV1(context.Context, ToolSurfaceInvocationCoordinateV1) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
	InspectExactToolSurfaceInvocationBindingV1(context.Context, ToolSurfaceInvocationBindingRefV1) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
}

type ToolSurfaceInvocationBindingRepositoryV1 interface {
	ToolSurfaceInvocationBindingWriterV1
	ToolSurfaceInvocationBindingReaderV1
}

func DeriveToolSurfaceInvocationBindingIDV1(c ToolSurfaceInvocationCoordinateV1) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationCoordinateV1", c)
	if err != nil {
		return "", err
	}
	return "tool-surface-invocation-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DeriveToolSurfaceInvocationBindingAckIDV1(binding ToolSurfaceInvocationBindingRefV1, prepared modelcontract.PreparedModelInvocationRefV1, current modelcontract.PreparedModelInvocationCurrentRefV1) (string, error) {
	if binding.Validate() != nil || prepared.Validate() != nil || current.Validate() != nil {
		return "", invalid("Tool Surface Invocation Binding Ack identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest(toolSurfaceInvocationBindingCanonicalDomainV1, ToolSurfaceInvocationBindingContractVersionV1, "ToolSurfaceInvocationBindingAckIdentityV1", struct {
		Binding  ToolSurfaceInvocationBindingRefV1                 `json:"binding"`
		Prepared modelcontract.PreparedModelInvocationRefV1        `json:"prepared"`
		Current  modelcontract.PreparedModelInvocationCurrentRefV1 `json:"current"`
	}{binding, prepared, current})
	if err != nil {
		return "", err
	}
	return "tool-surface-invocation-ack-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func validateToolSurfaceInvocationInputsV1(invocation ToolSurfaceInvocationCoordinateV1, preparedRef modelcontract.PreparedModelInvocationRefV1, fact modelcontract.PreparedModelInvocationFactV1, currentRef modelcontract.PreparedModelInvocationCurrentRefV1, current modelcontract.PreparedModelInvocationCurrentProjectionV1, surface ToolSurfaceManifestCurrentProjectionV1, assemblyRef runtimeports.ModelPreDispatchAssemblyCurrentRefV1, registry runtimeports.RegistrySnapshotRefV1, assembly runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, requested int64) error {
	if invocation.Validate() != nil || preparedRef.Validate() != nil || fact.Validate() != nil || currentRef.Validate() != nil || current.Validate() != nil || surface.Validate() != nil || assemblyRef.Validate() != nil || registry.Validate() != nil || assembly.Validate() != nil || requested <= 0 {
		return invalid("Tool Surface Invocation Binding inputs are invalid")
	}
	if fact.Ref() != preparedRef || currentRef != current.Ref() || currentRef.Prepared != preparedRef || current.Prepared != preparedRef || current.ValidateAgainstFact(fact) != nil {
		return conflict("Prepared Historical and Current closure drifted")
	}
	if invocation.InvocationID != fact.InvocationID || invocation.InvocationDigest != fact.InvocationDigest {
		return conflict("Tool Surface Invocation coordinate drifted from Prepared Fact")
	}
	if assemblyRef != assembly.Ref || assembly.RegistrySnapshot != registry || fact.RegistrySnapshotRef != registry || current.RegistrySnapshotRef != registry {
		return conflict("Tool Surface Invocation Registry or Assembly exact Ref drifted")
	}
	if registry.Digest != surface.Manifest.RegistrySnapshotDigest {
		return conflict("Tool Surface Manifest Registry snapshot digest drifted")
	}
	if assembly.ToolSurface.ID != surface.Ref.ID || assembly.ToolSurface.Revision != surface.Ref.Revision || assembly.ToolSurface.Digest != surface.Ref.Digest {
		return conflict("Assembly ToolSurface does not bind Tool Surface current")
	}
	if assembly.ProfileDigest != fact.ProfileDigest || fact.ActualToolSurfaceDigest != current.ActualToolSurfaceDigest || fact.ActualProviderInjectionDigest != current.ActualProviderInjectionDigest {
		return conflict("Prepared or Assembly semantic digest drifted")
	}
	expected, err := ComputeExpectedInjectionDigest(surface.Manifest.Entries)
	if err != nil || expected != surface.Manifest.ExpectedInjectionDigest || expected != fact.ActualToolSurfaceDigest {
		return conflict("Tool expected injection digest differs from Model actual Tool Surface")
	}
	return nil
}

func toolSurfaceInvocationNotAfterV1(s ToolSurfaceInvocationBindingSubjectV1, callerDeadline time.Time) int64 {
	values := []int64{s.PreparedHistoricalFact.NotAfterUnixNano, s.PreparedCurrent.ExpiresUnixNano, s.SurfaceCurrent.ExpiresUnixNano, s.AssemblyCurrent.ExpiresUnixNano, s.RequestedNotAfterUnixNano}
	if !callerDeadline.IsZero() {
		values = append(values, callerDeadline.UnixNano())
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func ToolSurfaceInvocationBindingNotAfterV1(s ToolSurfaceInvocationBindingSubjectV1, callerDeadline time.Time) int64 {
	return toolSurfaceInvocationNotAfterV1(s, callerDeadline)
}
