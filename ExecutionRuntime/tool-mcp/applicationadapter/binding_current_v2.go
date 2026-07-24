package applicationadapter

import (
	"context"
	"reflect"
	"strconv"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SingleCallToolActionBindingResolveRequestV2 struct {
	ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2              `json:"application_request"`
	SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
	RequestedExpiresUnixNano int64                                                          `json:"requested_expires_unix_nano"`
}

func (r SingleCallToolActionBindingResolveRequestV2) Validate(now time.Time) error {
	if r.RequestedExpiresUnixNano < 0 || r.ApplicationRequest.ValidateCurrent(now) != nil || r.SourceSubject.Validate() != nil || !reflect.DeepEqual(r.SourceSubject, r.ApplicationRequest.Action.PendingSubject) {
		return bindingInvalidV1("BindingV2 Resolve request is invalid")
	}
	if r.RequestedExpiresUnixNano > 0 && !now.Before(time.Unix(0, r.RequestedExpiresUnixNano)) {
		return bindingExpiredV1("BindingV2 requested window expired")
	}
	return nil
}

type SingleCallToolActionBindingIssuanceLookupRequestV2 struct {
	ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2              `json:"application_request"`
	SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
	RequestedExpiresUnixNano int64                                                          `json:"requested_expires_unix_nano"`
}

func (r SingleCallToolActionBindingIssuanceLookupRequestV2) resolveV2() SingleCallToolActionBindingResolveRequestV2 {
	return SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: r.ApplicationRequest, SourceSubject: r.SourceSubject, RequestedExpiresUnixNano: r.RequestedExpiresUnixNano}
}

type SingleCallToolActionBindingInspectExactRequestV2 struct {
	ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2              `json:"application_request"`
	SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
	RequestedExpiresUnixNano int64                                                          `json:"requested_expires_unix_nano"`
	Expected                 toolcontract.SingleCallToolActionBindingCurrentRefV2           `json:"expected"`
}

func (r SingleCallToolActionBindingInspectExactRequestV2) resolveV2() SingleCallToolActionBindingResolveRequestV2 {
	return SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: r.ApplicationRequest, SourceSubject: r.SourceSubject, RequestedExpiresUnixNano: r.RequestedExpiresUnixNano}
}

type SingleCallToolActionCandidateClosureV2 struct {
	ApplicationInput         applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
	ModelProjection          modelinvoker.ToolCallCandidateObservationProjectionV1            `json:"model_projection"`
	SurfaceInvocationBinding toolcontract.ToolSurfaceInvocationBindingV1                      `json:"surface_invocation_binding"`
	Association              runtimeports.GenerationBindingAssociationFactV1                  `json:"association"`
	Generation               runtimeports.GenerationCurrentProjectionV1                       `json:"generation"`
	Route                    runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
	ProviderCurrent          runtimeports.ProviderBindingCurrentProjectionV2                  `json:"provider_current"`
	SurfaceCurrent           toolcontract.ToolSurfaceManifestCurrentProjectionV1              `json:"surface_current"`
	CapabilityCurrent        toolcontract.ToolRegistryObjectCurrentProjectionV1               `json:"capability_current"`
	ToolCurrent              toolcontract.ToolRegistryObjectCurrentProjectionV1               `json:"tool_current"`
	InputContract            toolcontract.ToolInputContractCurrentProjectionV1                `json:"input_contract"`
	Candidate                toolcontract.ActionCandidateV3                                   `json:"candidate"`
	ClosureDigest            core.Digest                                                      `json:"closure_digest"`
}

func (c SingleCallToolActionCandidateClosureV2) Validate() error {
	if c.ApplicationInput.Digest.Validate() != nil || c.ModelProjection.Validate() != nil || c.SurfaceInvocationBinding.Validate() != nil || c.Association.Validate() != nil || c.Generation.Validate() != nil || c.Route.Validate() != nil || c.SurfaceCurrent.Validate() != nil || c.CapabilityCurrent.Validate() != nil || c.ToolCurrent.Validate() != nil || c.InputContract.Validate() != nil || c.Candidate.Validate() != nil {
		return bindingInvalidV1("BindingV2 Candidate Closure is invalid")
	}
	if c.SurfaceInvocationBinding.Subject.SurfaceCurrent.Ref != c.SurfaceCurrent.Ref || c.InputContract.SurfaceCurrent.Ref != c.SurfaceCurrent.Ref || c.InputContract.CapabilityCurrent.Ref != c.CapabilityCurrent.Ref || c.InputContract.ToolCurrent.Ref != c.ToolCurrent.Ref || c.Candidate.InputContractCurrentRef != c.InputContract.Ref || c.Candidate.SurfaceCurrent != c.SurfaceCurrent.Ref || c.Candidate.CapabilityCurrent != c.CapabilityCurrent.Ref || c.Candidate.ToolCurrent != c.ToolCurrent.Ref || c.Route.ProviderBinding != c.ProviderCurrent.Ref {
		return bindingConflictV1("BindingV2 Candidate Closure exact refs drifted")
	}
	if err := c.Candidate.ValidateAgainstInputContract(c.InputContract); err != nil {
		return err
	}
	if err := c.Candidate.ValidateAgainstModelProjection(c.ModelProjection); err != nil {
		return err
	}
	digest, err := c.DigestV2()
	if err != nil || digest != c.ClosureDigest {
		return bindingConflictV1("BindingV2 Candidate Closure digest drifted")
	}
	return nil
}

func (c SingleCallToolActionCandidateClosureV2) DigestV2() (core.Digest, error) {
	c = cloneCandidateClosureV2(c)
	c.ClosureDigest = ""
	return core.CanonicalJSONDigest("praxis.tool", toolcontract.SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionCandidateClosureV2", c)
}

func SealSingleCallToolActionCandidateClosureV2(c SingleCallToolActionCandidateClosureV2) (SingleCallToolActionCandidateClosureV2, error) {
	c = cloneCandidateClosureV2(c)
	provided := c.ClosureDigest
	c.ClosureDigest = ""
	digest, err := c.DigestV2()
	if err != nil {
		return SingleCallToolActionCandidateClosureV2{}, err
	}
	if provided != "" && provided != digest {
		return SingleCallToolActionCandidateClosureV2{}, bindingConflictV1("supplied BindingV2 Candidate Closure digest drifted")
	}
	c.ClosureDigest = digest
	return c, c.Validate()
}

type SingleCallToolActionBindingS2SnapshotV2 struct {
	ApplicationInput         applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
	SurfaceInvocationBinding toolcontract.ToolSurfaceInvocationBindingV1                      `json:"surface_invocation_binding"`
	Association              runtimeports.GenerationBindingAssociationFactV1                  `json:"association"`
	Generation               runtimeports.GenerationCurrentProjectionV1                       `json:"generation"`
	Route                    runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
	ProviderCurrent          runtimeports.ProviderBindingCurrentProjectionV2                  `json:"provider_current"`
	SurfaceCurrent           toolcontract.ToolSurfaceManifestCurrentProjectionV1              `json:"surface_current"`
	CapabilityCurrent        toolcontract.ToolRegistryObjectCurrentProjectionV1               `json:"capability_current"`
	ToolCurrent              toolcontract.ToolRegistryObjectCurrentProjectionV1               `json:"tool_current"`
	CheckedUnixNano          int64                                                            `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                                            `json:"expires_unix_nano"`
	Digest                   core.Digest                                                      `json:"digest"`
}

func (s SingleCallToolActionBindingS2SnapshotV2) Validate() error {
	if s.ApplicationInput.Digest.Validate() != nil || s.SurfaceInvocationBinding.Validate() != nil || s.Association.Validate() != nil || s.Generation.Validate() != nil || s.Route.Validate() != nil || s.SurfaceCurrent.Validate() != nil || s.CapabilityCurrent.Validate() != nil || s.ToolCurrent.Validate() != nil || s.CheckedUnixNano <= 0 || s.ExpiresUnixNano <= s.CheckedUnixNano {
		return bindingInvalidV1("BindingV2 S2 Snapshot is invalid")
	}
	if s.SurfaceInvocationBinding.Subject.SurfaceCurrent.Ref != s.SurfaceCurrent.Ref || s.Route.ProviderBinding != s.ProviderCurrent.Ref {
		return bindingConflictV1("BindingV2 S2 Snapshot exact refs drifted")
	}
	for _, upper := range []int64{s.ApplicationInput.ExpiresUnixNano, s.SurfaceInvocationBinding.NotAfterUnixNano, s.Association.ExpiresUnixNano, s.Generation.ExpiresUnixNano, s.Route.ExpiresUnixNano, s.ProviderCurrent.ExpiresUnixNano, s.SurfaceCurrent.ExpiresUnixNano, s.CapabilityCurrent.ExpiresUnixNano, s.ToolCurrent.ExpiresUnixNano} {
		if s.ExpiresUnixNano > upper {
			return bindingConflictV1("BindingV2 S2 Snapshot exceeds an Owner current upper bound")
		}
	}
	digest, err := s.DigestV2()
	if err != nil || digest != s.Digest {
		return bindingConflictV1("BindingV2 S2 Snapshot digest drifted")
	}
	return nil
}

func (s SingleCallToolActionBindingS2SnapshotV2) DigestV2() (core.Digest, error) {
	s = cloneS2SnapshotV2(s)
	s.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool", toolcontract.SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionBindingS2SnapshotV2", s)
}

func SealSingleCallToolActionBindingS2SnapshotV2(s SingleCallToolActionBindingS2SnapshotV2) (SingleCallToolActionBindingS2SnapshotV2, error) {
	s = cloneS2SnapshotV2(s)
	provided := s.Digest
	s.Digest = ""
	digest, err := s.DigestV2()
	if err != nil {
		return SingleCallToolActionBindingS2SnapshotV2{}, err
	}
	if provided != "" && provided != digest {
		return SingleCallToolActionBindingS2SnapshotV2{}, bindingConflictV1("supplied BindingV2 S2 Snapshot digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type SingleCallToolActionBindingCurrentProjectionV2 struct {
	ContractVersion          string                                                    `json:"contract_version"`
	Ref                      toolcontract.SingleCallToolActionBindingCurrentRefV2      `json:"ref"`
	IssuanceSubject          toolcontract.SingleCallToolActionBindingIssuanceSubjectV2 `json:"issuance_subject"`
	CandidateRef             toolcontract.ObjectRef                                    `json:"candidate_ref"`
	InputContractCurrentRef  toolcontract.ToolInputContractCurrentRefV1                `json:"input_contract_current_ref"`
	CandidateClosure         SingleCallToolActionCandidateClosureV2                    `json:"candidate_closure"`
	S2Snapshot               SingleCallToolActionBindingS2SnapshotV2                   `json:"s2_snapshot"`
	RequestedExpiresUnixNano int64                                                     `json:"requested_expires_unix_nano"`
	CheckedUnixNano          int64                                                     `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                                     `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                                               `json:"projection_digest"`
}

func (p SingleCallToolActionBindingCurrentProjectionV2) Validate() error {
	if p.ContractVersion != toolcontract.SingleCallToolActionBindingCurrentContractVersionV2 || p.Ref.Validate() != nil || p.IssuanceSubject.Validate() != nil || p.CandidateRef.Validate() != nil || p.InputContractCurrentRef.Validate() != nil || p.CandidateClosure.Validate() != nil || p.S2Snapshot.Validate() != nil || p.RequestedExpiresUnixNano < 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return bindingInvalidV1("BindingV2 Current Projection is invalid")
	}
	if p.CandidateRef != p.CandidateClosure.Candidate.ObjectRef() || p.InputContractCurrentRef != p.CandidateClosure.InputContract.Ref || p.RequestedExpiresUnixNano != p.IssuanceSubject.RequestedExpiresUnixNano || p.Ref.Revision != 1 {
		return bindingConflictV1("BindingV2 Current Projection repeated fields drifted")
	}
	id, err := toolcontract.DeriveSingleCallToolActionBindingCurrentIDV2(p.IssuanceSubject)
	if err != nil || id != p.Ref.ID {
		return bindingConflictV1("BindingV2 Current Projection ID drifted")
	}
	for _, upper := range []int64{p.CandidateClosure.ApplicationInput.ExpiresUnixNano, p.CandidateClosure.SurfaceInvocationBinding.NotAfterUnixNano, p.CandidateClosure.Association.ExpiresUnixNano, p.CandidateClosure.Generation.ExpiresUnixNano, p.CandidateClosure.Route.ExpiresUnixNano, p.CandidateClosure.ProviderCurrent.ExpiresUnixNano, p.CandidateClosure.InputContract.ExpiresUnixNano, p.CandidateClosure.SurfaceCurrent.ExpiresUnixNano, p.CandidateClosure.CapabilityCurrent.ExpiresUnixNano, p.CandidateClosure.ToolCurrent.ExpiresUnixNano, p.CandidateClosure.Candidate.RequestedExpiresUnixNano, p.S2Snapshot.ExpiresUnixNano, p.CheckedUnixNano + int64(toolcontract.MaxSingleCallToolActionBindingCurrentTTLV2)} {
		if p.ExpiresUnixNano > upper {
			return bindingConflictV1("BindingV2 Current Projection exceeds an Owner current upper bound")
		}
	}
	if p.RequestedExpiresUnixNano > 0 && p.ExpiresUnixNano > p.RequestedExpiresUnixNano {
		return bindingConflictV1("BindingV2 Current Projection exceeds requested expiry")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return bindingConflictV1("BindingV2 Current Projection digest drifted")
	}
	return nil
}

func (p SingleCallToolActionBindingCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "BindingV2 clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return bindingExpiredV1("BindingV2 expired")
	}
	return nil
}

func (p SingleCallToolActionBindingCurrentProjectionV2) ValidateAgainst(request SingleCallToolActionBindingResolveRequestV2, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	if err := p.ValidateCurrent(now); err != nil {
		return err
	}
	_, issuance, err := sealBindingIssuanceV2(request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p.IssuanceSubject, issuance) || p.CandidateClosure.ApplicationInput.RequestID != request.ApplicationRequest.ID || p.CandidateClosure.ApplicationInput.RequestDigest != request.ApplicationRequest.Digest {
		return bindingConflictV1("BindingV2 Current Projection differs from stable request")
	}
	return nil
}

func (p SingleCallToolActionBindingCurrentProjectionV2) DigestV2() (core.Digest, error) {
	p = CloneSingleCallToolActionBindingCurrentProjectionV2(p)
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.tool", toolcontract.SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionBindingCurrentProjectionV2", p)
}

func SealSingleCallToolActionBindingCurrentProjectionV2(p SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	p = CloneSingleCallToolActionBindingCurrentProjectionV2(p)
	p.ContractVersion = toolcontract.SingleCallToolActionBindingCurrentContractVersionV2
	id, err := toolcontract.DeriveSingleCallToolActionBindingCurrentIDV2(p.IssuanceSubject)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("supplied BindingV2 ID drifted")
	}
	p.Ref.ID, p.Ref.Revision = id, 1
	providedRef, providedProjection := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.DigestV2()
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	for _, provided := range []core.Digest{providedRef, providedProjection} {
		if provided != "" && provided != digest {
			return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("supplied BindingV2 Projection digest drifted")
		}
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type SingleCallToolActionBindingCurrentReaderV2 interface {
	ResolveSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingResolveRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
	InspectSingleCallToolActionBindingCurrentByIssuanceV2(context.Context, SingleCallToolActionBindingIssuanceLookupRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
	InspectExactSingleCallToolActionBindingCurrentV2(context.Context, SingleCallToolActionBindingInspectExactRequestV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
}

type SingleCallToolActionBindingLeaseStoreV2 interface {
	CreateSingleCallToolActionBindingCurrentOnceV2(context.Context, SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
	InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(context.Context, string) (SingleCallToolActionBindingCurrentProjectionV2, error)
	InspectExactSingleCallToolActionBindingCurrentV2(context.Context, toolcontract.SingleCallToolActionBindingCurrentRefV2) (SingleCallToolActionBindingCurrentProjectionV2, error)
}

func sealBindingIssuanceV2(request SingleCallToolActionBindingResolveRequestV2) (toolcontract.SingleCallToolActionBindingSubjectV2, toolcontract.SingleCallToolActionBindingIssuanceSubjectV2, error) {
	sourceDigest, err := request.SourceSubject.DigestV2()
	if err != nil {
		return toolcontract.SingleCallToolActionBindingSubjectV2{}, toolcontract.SingleCallToolActionBindingIssuanceSubjectV2{}, err
	}
	subject, err := toolcontract.SealSingleCallToolActionBindingSubjectV2(toolcontract.SingleCallToolActionBindingSubjectV2{
		ApplicationRequestID: request.ApplicationRequest.ID, ApplicationRequestRevision: request.ApplicationRequest.Revision, ApplicationRequestDigest: request.ApplicationRequest.Digest,
		PendingAction: toolcontract.PendingActionExactRefV2{ID: request.SourceSubject.PendingActionRef, Revision: 1, RequestDigest: request.SourceSubject.PendingActionDigest},
		TenantID:      request.SourceSubject.Run.ExecutionScope.Identity.TenantID, RunID: string(request.SourceSubject.Run.RunID), SessionID: request.SourceSubject.SessionID,
		TurnID: formatTurnV2(request.SourceSubject.Turn), ActionCoordinateDigest: request.ApplicationRequest.Action.Digest,
		ExecutionScope: request.ApplicationRequest.Action.ExecutionScope, ExecutionScopeDigest: request.ApplicationRequest.Action.ExecutionScopeDigest, SourceSubjectDigest: sourceDigest,
	})
	if err != nil {
		return toolcontract.SingleCallToolActionBindingSubjectV2{}, toolcontract.SingleCallToolActionBindingIssuanceSubjectV2{}, err
	}
	issuance, err := toolcontract.SealSingleCallToolActionBindingIssuanceSubjectV2(toolcontract.SingleCallToolActionBindingIssuanceSubjectV2{BindingSubject: subject, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano})
	return subject, issuance, err
}

func formatTurnV2(turn uint32) string { return strconv.FormatUint(uint64(turn), 10) }

func cloneCandidateClosureV2(c SingleCallToolActionCandidateClosureV2) SingleCallToolActionCandidateClosureV2 {
	c.ApplicationInput = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(c.ApplicationInput)
	c.ModelProjection = c.ModelProjection.Clone()
	c.SurfaceInvocationBinding = cloneSurfaceInvocationBindingV2(c.SurfaceInvocationBinding)
	c.Association = cloneAssociationV1(c.Association)
	c.Generation = cloneGenerationV1(c.Generation)
	c.SurfaceCurrent = cloneSurfaceCurrentProjectionV2(c.SurfaceCurrent)
	c.InputContract = toolcontract.CloneToolInputContractCurrentProjectionV1(c.InputContract)
	c.Candidate = toolcontract.CloneActionCandidateV3(c.Candidate)
	return c
}

func cloneS2SnapshotV2(s SingleCallToolActionBindingS2SnapshotV2) SingleCallToolActionBindingS2SnapshotV2 {
	s.ApplicationInput = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(s.ApplicationInput)
	s.SurfaceInvocationBinding = cloneSurfaceInvocationBindingV2(s.SurfaceInvocationBinding)
	s.Association = cloneAssociationV1(s.Association)
	s.Generation = cloneGenerationV1(s.Generation)
	s.SurfaceCurrent = cloneSurfaceCurrentProjectionV2(s.SurfaceCurrent)
	return s
}

func CloneSingleCallToolActionBindingCurrentProjectionV2(p SingleCallToolActionBindingCurrentProjectionV2) SingleCallToolActionBindingCurrentProjectionV2 {
	p.CandidateClosure = cloneCandidateClosureV2(p.CandidateClosure)
	p.S2Snapshot = cloneS2SnapshotV2(p.S2Snapshot)
	return p
}

func cloneSurfaceCurrentProjectionV2(p toolcontract.ToolSurfaceManifestCurrentProjectionV1) toolcontract.ToolSurfaceManifestCurrentProjectionV1 {
	p.Manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), p.Manifest.Entries...)
	for index := range p.Manifest.Entries {
		p.Manifest.Entries[index].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), p.Manifest.Entries[index].EffectKinds...)
	}
	p.Manifest.Residuals = append([]toolcontract.Residual(nil), p.Manifest.Residuals...)
	return p
}

func cloneSurfaceInvocationBindingV2(b toolcontract.ToolSurfaceInvocationBindingV1) toolcontract.ToolSurfaceInvocationBindingV1 {
	b.Subject.SurfaceCurrent = cloneSurfaceCurrentProjectionV2(b.Subject.SurfaceCurrent)
	return b
}
