package contract

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolInputContractCurrentContractVersionV1 = "praxis.tool.input-contract-current/v1"

const (
	ToolInputSchemaCurrentKindV1     runtimeports.NamespacedNameV2 = "praxis.tool/input-schema-current"
	ToolInputLimitPolicyV1           runtimeports.NamespacedNameV2 = "praxis.tool/input-payload-v1"
	MaxToolInputContractCurrentTTLV1                               = 15 * time.Second
)

const toolInputContractCanonicalDomainV1 = "praxis.tool"

type ToolInputContractCurrentRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ToolInputContractCurrentRefV1) Validate() error {
	if ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("Tool Input Contract current Ref is invalid")
	}
	return r.Digest.Validate()
}

type ToolInputSchemaCurrentRefV1 struct {
	Kind            runtimeports.NamespacedNameV2  `json:"kind"`
	ID              string                         `json:"id"`
	Revision        core.Revision                  `json:"revision"`
	Digest          core.Digest                    `json:"digest"`
	InputSchema     runtimeports.SchemaRefV2       `json:"input_schema"`
	Authority       ToolRegistryObjectCurrentRefV1 `json:"authority"`
	RegistryOwner   core.OwnerRef                  `json:"registry_owner"`
	CheckedUnixNano int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano int64                          `json:"expires_unix_nano"`
}

func (r ToolInputSchemaCurrentRefV1) Validate() error {
	if r.Kind != ToolInputSchemaCurrentKindV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.InputSchema.Validate() != nil || r.Authority.Validate() != nil || r.Authority.Kind != ToolRegistryDescriptorCurrentKindV1 || r.RegistryOwner.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return invalid("Tool Input Schema current Ref is invalid")
	}
	if r.Digest != r.InputSchema.ContentDigest {
		return conflict("Tool Input Schema current digest differs from Schema")
	}
	id, err := DeriveToolInputSchemaCurrentIDV1(r.InputSchema, r.Authority)
	if err != nil || id != r.ID {
		return conflict("Tool Input Schema current ID drifted")
	}
	return nil
}

func (r ToolInputSchemaCurrentRefV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Input Schema current clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Input Schema current expired")
	}
	return nil
}

func DeriveToolInputSchemaCurrentIDV1(schema runtimeports.SchemaRefV2, authority ToolRegistryObjectCurrentRefV1) (string, error) {
	if err := schema.Validate(); err != nil {
		return "", err
	}
	if err := authority.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputSchemaCurrentIdentityV1", struct {
		InputSchema runtimeports.SchemaRefV2       `json:"input_schema"`
		Authority   ToolRegistryObjectCurrentRefV1 `json:"authority"`
	}{InputSchema: schema, Authority: authority})
	if err != nil {
		return "", err
	}
	return "input-schema-current-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func SealToolInputSchemaCurrentRefV1(r ToolInputSchemaCurrentRefV1) (ToolInputSchemaCurrentRefV1, error) {
	r.Kind = ToolInputSchemaCurrentKindV1
	id, err := DeriveToolInputSchemaCurrentIDV1(r.InputSchema, r.Authority)
	if err != nil {
		return ToolInputSchemaCurrentRefV1{}, err
	}
	if r.ID != "" && r.ID != id {
		return ToolInputSchemaCurrentRefV1{}, conflict("supplied Tool Input Schema current ID drifted")
	}
	r.ID, r.Revision, r.Digest = id, 1, r.InputSchema.ContentDigest
	return r, r.Validate()
}

type ToolInputLimitPolicySubjectV1 struct {
	Surface             ObjectRef                `json:"surface"`
	SurfaceEntryOrdinal uint32                   `json:"surface_entry_ordinal"`
	SurfaceEntry        ToolSurfaceEntry         `json:"surface_entry"`
	Capability          ObjectRef                `json:"capability"`
	Tool                ObjectRef                `json:"tool"`
	InputSchema         runtimeports.SchemaRefV2 `json:"input_schema"`
	MaxInlineBytes      uint64                   `json:"max_inline_bytes"`
	InlineRequired      bool                     `json:"inline_required"`
	RefForbidden        bool                     `json:"ref_forbidden"`
}

func DeriveToolInputLimitPolicyV1(surface ObjectRef, ordinal uint32, entry ToolSurfaceEntry, capability, tool ObjectRef, schema runtimeports.SchemaRefV2) (runtimeports.OpaqueLimitPolicyRefV2, error) {
	if surface.Validate() != nil || entry.Validate() != nil || capability.Validate() != nil || tool.Validate() != nil || schema.Validate() != nil || entry.Order != ordinal || entry.Capability != capability || entry.Tool != tool || entry.InputSchema != schema {
		return runtimeports.OpaqueLimitPolicyRefV2{}, invalid("Tool Input Limit Policy sources are invalid")
	}
	subject := ToolInputLimitPolicySubjectV1{
		Surface: surface, SurfaceEntryOrdinal: ordinal, SurfaceEntry: cloneToolSurfaceEntryV1(entry), Capability: capability, Tool: tool, InputSchema: schema,
		MaxInlineBytes: runtimeports.MaxOpaqueInlineBytes, InlineRequired: true, RefForbidden: true,
	}
	digest, err := core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputLimitPolicySubjectV1", subject)
	if err != nil {
		return runtimeports.OpaqueLimitPolicyRefV2{}, err
	}
	return runtimeports.OpaqueLimitPolicyRefV2{Policy: ToolInputLimitPolicyV1, Digest: digest}, nil
}

type ToolInputContractBindingSubjectV1 struct {
	ApplicationRequestID       string                              `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                       `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                         `json:"application_request_digest"`
	PendingAction              PendingActionExactRefV2             `json:"pending_action"`
	OperationScopeDigest       core.Digest                         `json:"operation_scope_digest"`
	ProviderBinding            runtimeports.ProviderBindingRefV2   `json:"provider_binding"`
	ExpectedOwner              runtimeports.EffectOwnerRefV2       `json:"expected_owner"`
	SurfaceOwner               core.OwnerRef                       `json:"surface_owner"`
	CapabilityRegistryOwner    core.OwnerRef                       `json:"capability_registry_owner"`
	ToolRegistryOwner          core.OwnerRef                       `json:"tool_registry_owner"`
	Surface                    ObjectRef                           `json:"surface"`
	SurfaceEntryOrdinal        uint32                              `json:"surface_entry_ordinal"`
	SurfaceEntry               ToolSurfaceEntry                    `json:"surface_entry"`
	Capability                 ObjectRef                           `json:"capability"`
	Tool                       ObjectRef                           `json:"tool"`
	ToolArtifactDigest         core.Digest                         `json:"tool_artifact_digest"`
	InputSchema                runtimeports.SchemaRefV2            `json:"input_schema"`
	LimitPolicy                runtimeports.OpaqueLimitPolicyRefV2 `json:"limit_policy"`
	Digest                     core.Digest                         `json:"digest"`
}

func (s ToolInputContractBindingSubjectV1) Validate() error {
	if strings.TrimSpace(s.ApplicationRequestID) == "" || s.ApplicationRequestRevision == 0 || s.ApplicationRequestDigest.Validate() != nil || s.PendingAction.Validate() != nil || s.PendingAction.Revision != 1 || s.OperationScopeDigest.Validate() != nil || s.ProviderBinding.Validate() != nil || validateEffectOwner(s.ExpectedOwner) != nil || s.SurfaceOwner.Validate() != nil || s.CapabilityRegistryOwner.Validate() != nil || s.ToolRegistryOwner.Validate() != nil || s.Surface.Validate() != nil || s.SurfaceEntry.Validate() != nil || s.Capability.Validate() != nil || s.Tool.Validate() != nil || s.ToolArtifactDigest.Validate() != nil || s.InputSchema.Validate() != nil || runtimeports.ValidateNamespacedNameV2(s.LimitPolicy.Policy) != nil || s.LimitPolicy.Digest.Validate() != nil {
		return invalid("Tool Input Contract Binding Subject is invalid")
	}
	if s.SurfaceEntry.Order != s.SurfaceEntryOrdinal || s.SurfaceEntry.Capability != s.Capability || s.SurfaceEntry.Tool != s.Tool || s.SurfaceEntry.InputSchema != s.InputSchema || !s.SurfaceEntry.Allowed || s.SurfaceEntry.Visibility != SurfaceVisible || !containsToolExecuteV1(s.SurfaceEntry.EffectKinds) {
		return conflict("Tool Input Contract Surface Entry drifted")
	}
	if s.ProviderBinding.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3) || s.ProviderBinding.ArtifactDigest != s.ToolArtifactDigest || s.ExpectedOwner.Role != runtimeports.OwnerSettlement || s.ExpectedOwner.ComponentID != s.ProviderBinding.ComponentID || s.ExpectedOwner.ManifestDigest != s.ProviderBinding.ManifestDigest {
		return conflict("Tool Input Contract Provider or ExpectedOwner drifted")
	}
	policy, err := DeriveToolInputLimitPolicyV1(s.Surface, s.SurfaceEntryOrdinal, s.SurfaceEntry, s.Capability, s.Tool, s.InputSchema)
	if err != nil || policy != s.LimitPolicy {
		return conflict("Tool Input Contract Limit Policy drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Tool Input Contract Binding Subject digest drifted")
	}
	return nil
}

func (s ToolInputContractBindingSubjectV1) ComputeDigest() (core.Digest, error) {
	s = cloneToolInputContractBindingSubjectV1(s)
	s.Digest = ""
	return core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputContractBindingSubjectV1", s)
}

func SealToolInputContractBindingSubjectV1(s ToolInputContractBindingSubjectV1) (ToolInputContractBindingSubjectV1, error) {
	s = cloneToolInputContractBindingSubjectV1(s)
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return ToolInputContractBindingSubjectV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolInputContractBindingSubjectV1{}, conflict("supplied Tool Input Contract Binding Subject digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type ToolInputContractLookupSubjectV1 struct {
	ApplicationRequestID       string                            `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                     `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                       `json:"application_request_digest"`
	PendingAction              PendingActionExactRefV2           `json:"pending_action"`
	OperationScopeDigest       core.Digest                       `json:"operation_scope_digest"`
	ProviderBinding            runtimeports.ProviderBindingRefV2 `json:"provider_binding"`
	ExpectedOwner              runtimeports.EffectOwnerRefV2     `json:"expected_owner"`
	Surface                    ObjectRef                         `json:"surface"`
	CallName                   string                            `json:"call_name"`
	Capability                 ObjectRef                         `json:"capability"`
	Tool                       ObjectRef                         `json:"tool"`
	InputSchema                runtimeports.SchemaRefV2          `json:"input_schema"`
	Digest                     core.Digest                       `json:"digest"`
}

func (s ToolInputContractLookupSubjectV1) Validate() error {
	if strings.TrimSpace(s.ApplicationRequestID) == "" || s.ApplicationRequestRevision == 0 || s.ApplicationRequestDigest.Validate() != nil || s.PendingAction.Validate() != nil || s.PendingAction.Revision != 1 || s.OperationScopeDigest.Validate() != nil || s.ProviderBinding.Validate() != nil || validateEffectOwner(s.ExpectedOwner) != nil || s.Surface.Validate() != nil || strings.TrimSpace(s.CallName) == "" || s.Capability.Validate() != nil || s.Tool.Validate() != nil || s.InputSchema.Validate() != nil {
		return invalid("Tool Input Contract Lookup Subject is invalid")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Tool Input Contract Lookup Subject digest drifted")
	}
	return nil
}

func (s ToolInputContractLookupSubjectV1) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputContractLookupSubjectV1", s)
}

func SealToolInputContractLookupSubjectV1(s ToolInputContractLookupSubjectV1) (ToolInputContractLookupSubjectV1, error) {
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return ToolInputContractLookupSubjectV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolInputContractLookupSubjectV1{}, conflict("supplied Tool Input Contract Lookup Subject digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

type ToolInputContractIssuanceSubjectV1 struct {
	ContractVersion          string                           `json:"contract_version"`
	LookupSubject            ToolInputContractLookupSubjectV1 `json:"lookup_subject"`
	RequestedExpiresUnixNano int64                            `json:"requested_expires_unix_nano"`
	Digest                   core.Digest                      `json:"digest"`
}

func (s ToolInputContractIssuanceSubjectV1) Validate() error {
	if s.ContractVersion != ToolInputContractCurrentContractVersionV1 || s.LookupSubject.Validate() != nil || s.RequestedExpiresUnixNano < 0 {
		return invalid("Tool Input Contract Issuance Subject is invalid")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("Tool Input Contract Issuance Subject digest drifted")
	}
	return nil
}

func (s ToolInputContractIssuanceSubjectV1) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputContractIssuanceSubjectV1", s)
}

func SealToolInputContractIssuanceSubjectV1(s ToolInputContractIssuanceSubjectV1) (ToolInputContractIssuanceSubjectV1, error) {
	s.ContractVersion = ToolInputContractCurrentContractVersionV1
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return ToolInputContractIssuanceSubjectV1{}, err
	}
	if provided != "" && provided != digest {
		return ToolInputContractIssuanceSubjectV1{}, conflict("supplied Tool Input Contract Issuance Subject digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

func ToolInputContractIssuanceFromResolveRequestV1(r ToolInputContractResolveRequestV1) (ToolInputContractIssuanceSubjectV1, error) {
	lookup, err := SealToolInputContractLookupSubjectV1(ToolInputContractLookupSubjectV1{
		ApplicationRequestID: r.ApplicationRequestID, ApplicationRequestRevision: r.ApplicationRequestRevision, ApplicationRequestDigest: r.ApplicationRequestDigest,
		PendingAction: r.PendingAction, OperationScopeDigest: r.OperationScopeDigest, ProviderBinding: r.ProviderBinding, ExpectedOwner: r.ExpectedOwner,
		Surface: r.Surface, CallName: r.CallName, Capability: r.Capability, Tool: r.Tool, InputSchema: r.InputSchema,
	})
	if err != nil {
		return ToolInputContractIssuanceSubjectV1{}, err
	}
	return SealToolInputContractIssuanceSubjectV1(ToolInputContractIssuanceSubjectV1{LookupSubject: lookup, RequestedExpiresUnixNano: r.RequestedExpiresUnixNano})
}

type ToolInputContractResolveRequestV1 struct {
	ApplicationRequestID       string                            `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                     `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                       `json:"application_request_digest"`
	PendingAction              PendingActionExactRefV2           `json:"pending_action"`
	OperationScopeDigest       core.Digest                       `json:"operation_scope_digest"`
	ProviderBinding            runtimeports.ProviderBindingRefV2 `json:"provider_binding"`
	ExpectedOwner              runtimeports.EffectOwnerRefV2     `json:"expected_owner"`
	Surface                    ObjectRef                         `json:"surface"`
	CallName                   string                            `json:"call_name"`
	Capability                 ObjectRef                         `json:"capability"`
	Tool                       ObjectRef                         `json:"tool"`
	InputSchema                runtimeports.SchemaRefV2          `json:"input_schema"`
	RequestedExpiresUnixNano   int64                             `json:"requested_expires_unix_nano"`
}

func (r ToolInputContractResolveRequestV1) Validate(now time.Time) error {
	if strings.TrimSpace(r.ApplicationRequestID) == "" || r.ApplicationRequestRevision == 0 || r.ApplicationRequestDigest.Validate() != nil || r.PendingAction.Validate() != nil || r.PendingAction.Revision != 1 || r.OperationScopeDigest.Validate() != nil || r.ProviderBinding.Validate() != nil || validateEffectOwner(r.ExpectedOwner) != nil || r.Surface.Validate() != nil || strings.TrimSpace(r.CallName) == "" || r.Capability.Validate() != nil || r.Tool.Validate() != nil || r.InputSchema.Validate() != nil || r.RequestedExpiresUnixNano < 0 || now.IsZero() {
		return invalid("Tool Input Contract Resolve request is invalid")
	}
	if r.RequestedExpiresUnixNano > 0 && !now.Before(time.Unix(0, r.RequestedExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Input Contract requested window expired")
	}
	return nil
}

type ToolInputContractInspectByIssuanceRequestV1 struct {
	ResolveRequest ToolInputContractResolveRequestV1 `json:"resolve_request"`
}

type ToolInputContractInspectExactRequestV1 struct {
	ResolveRequest ToolInputContractResolveRequestV1 `json:"resolve_request"`
	Expected       ToolInputContractCurrentRefV1     `json:"expected"`
}

type ToolInputContractCurrentProjectionV1 struct {
	ContractVersion          string                                 `json:"contract_version"`
	Ref                      ToolInputContractCurrentRefV1          `json:"ref"`
	IssuanceSubject          ToolInputContractIssuanceSubjectV1     `json:"issuance_subject"`
	BindingSubject           ToolInputContractBindingSubjectV1      `json:"binding_subject"`
	SurfaceCurrent           ToolSurfaceManifestCurrentProjectionV1 `json:"surface_current"`
	CapabilityCurrent        ToolRegistryObjectCurrentProjectionV1  `json:"capability_current"`
	ToolCurrent              ToolRegistryObjectCurrentProjectionV1  `json:"tool_current"`
	InputSchemaCurrent       ToolInputSchemaCurrentRefV1            `json:"input_schema_current"`
	RequestedExpiresUnixNano int64                                  `json:"requested_expires_unix_nano"`
	CheckedUnixNano          int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                  `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                            `json:"projection_digest"`
}

func (p ToolInputContractCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ToolInputContractCurrentContractVersionV1 || p.Ref.Validate() != nil || p.IssuanceSubject.Validate() != nil || p.BindingSubject.Validate() != nil || p.SurfaceCurrent.Validate() != nil || p.CapabilityCurrent.Validate() != nil || p.ToolCurrent.Validate() != nil || p.InputSchemaCurrent.Validate() != nil || p.RequestedExpiresUnixNano < 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return invalid("Tool Input Contract current Projection is invalid")
	}
	lookup := p.IssuanceSubject.LookupSubject
	if p.RequestedExpiresUnixNano != p.IssuanceSubject.RequestedExpiresUnixNano || lookup.ApplicationRequestID != p.BindingSubject.ApplicationRequestID || lookup.ApplicationRequestRevision != p.BindingSubject.ApplicationRequestRevision || lookup.ApplicationRequestDigest != p.BindingSubject.ApplicationRequestDigest || lookup.PendingAction != p.BindingSubject.PendingAction || lookup.OperationScopeDigest != p.BindingSubject.OperationScopeDigest || lookup.ProviderBinding != p.BindingSubject.ProviderBinding || lookup.ExpectedOwner != p.BindingSubject.ExpectedOwner || lookup.Surface != p.BindingSubject.Surface || lookup.CallName != p.BindingSubject.SurfaceEntry.ModelName || lookup.Capability != p.BindingSubject.Capability || lookup.Tool != p.BindingSubject.Tool || lookup.InputSchema != p.BindingSubject.InputSchema || p.SurfaceCurrent.Ref.ID != p.BindingSubject.Surface.ID || p.SurfaceCurrent.Ref.Revision != p.BindingSubject.Surface.Revision || p.SurfaceCurrent.Ref.Digest != p.BindingSubject.Surface.Digest || p.SurfaceCurrent.Owner != p.BindingSubject.SurfaceOwner || p.CapabilityCurrent.Object != p.BindingSubject.Capability || p.CapabilityCurrent.RegistryOwner != p.BindingSubject.CapabilityRegistryOwner || p.ToolCurrent.Object != p.BindingSubject.Tool || p.ToolCurrent.RegistryOwner != p.BindingSubject.ToolRegistryOwner || p.InputSchemaCurrent.InputSchema != p.BindingSubject.InputSchema || p.InputSchemaCurrent.Authority != p.ToolCurrent.Ref || p.InputSchemaCurrent.RegistryOwner != p.BindingSubject.ToolRegistryOwner || p.InputSchemaCurrent.CheckedUnixNano != p.CheckedUnixNano || p.InputSchemaCurrent.ExpiresUnixNano != p.ExpiresUnixNano {
		return conflict("Tool Input Contract repeated fields drifted")
	}
	if int(p.BindingSubject.SurfaceEntryOrdinal) >= len(p.SurfaceCurrent.Manifest.Entries) || !reflect.DeepEqual(p.SurfaceCurrent.Manifest.Entries[p.BindingSubject.SurfaceEntryOrdinal], p.BindingSubject.SurfaceEntry) {
		return conflict("Tool Input Contract Surface Entry differs from Manifest")
	}
	if p.RequestedExpiresUnixNano > 0 && p.ExpiresUnixNano > p.RequestedExpiresUnixNano {
		return conflict("Tool Input Contract exceeds requested expiry")
	}
	for _, upper := range []int64{p.SurfaceCurrent.ExpiresUnixNano, p.CapabilityCurrent.ExpiresUnixNano, p.ToolCurrent.ExpiresUnixNano, p.CheckedUnixNano + int64(MaxToolInputContractCurrentTTLV1)} {
		if p.ExpiresUnixNano > upper {
			return conflict("Tool Input Contract exceeds an Owner current upper bound")
		}
	}
	id, err := DeriveToolInputContractCurrentIDV1(p.IssuanceSubject)
	if err != nil || p.Ref.ID != id || p.Ref.Revision != 1 || p.Ref.Digest != p.ProjectionDigest {
		return conflict("Tool Input Contract Ref or identity drifted")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return conflict("Tool Input Contract Projection digest drifted")
	}
	return nil
}

func (p ToolInputContractCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Input Contract clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Input Contract expired")
	}
	return nil
}

func (p ToolInputContractCurrentProjectionV1) ValidateAgainst(request ToolInputContractResolveRequestV1, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	if err := p.ValidateCurrent(now); err != nil {
		return err
	}
	s := p.BindingSubject
	if s.ApplicationRequestID != request.ApplicationRequestID || s.ApplicationRequestRevision != request.ApplicationRequestRevision || s.ApplicationRequestDigest != request.ApplicationRequestDigest || s.PendingAction != request.PendingAction || s.OperationScopeDigest != request.OperationScopeDigest || s.ProviderBinding != request.ProviderBinding || s.ExpectedOwner != request.ExpectedOwner || s.Surface != request.Surface || s.SurfaceEntry.ModelName != request.CallName || s.Capability != request.Capability || s.Tool != request.Tool || s.InputSchema != request.InputSchema || p.RequestedExpiresUnixNano != request.RequestedExpiresUnixNano {
		return conflict("Tool Input Contract does not match Resolve request")
	}
	return nil
}

func (p ToolInputContractCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p = CloneToolInputContractCurrentProjectionV1(p)
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(toolInputContractCanonicalDomainV1, ToolInputContractCurrentContractVersionV1, "ToolInputContractCurrentProjectionV1", p)
}

func SealToolInputContractCurrentV1(p ToolInputContractCurrentProjectionV1) (ToolInputContractCurrentProjectionV1, error) {
	p = CloneToolInputContractCurrentProjectionV1(p)
	p.ContractVersion = ToolInputContractCurrentContractVersionV1
	if p.IssuanceSubject.Validate() != nil || p.BindingSubject.Validate() != nil {
		return ToolInputContractCurrentProjectionV1{}, invalid("Tool Input Contract subjects are invalid")
	}
	id, err := DeriveToolInputContractCurrentIDV1(p.IssuanceSubject)
	if err != nil {
		return ToolInputContractCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ToolInputContractCurrentProjectionV1{}, conflict("supplied Tool Input Contract ID drifted")
	}
	p.Ref.ID, p.Ref.Revision = id, 1
	providedRef, providedProjection := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return ToolInputContractCurrentProjectionV1{}, err
	}
	for _, provided := range []core.Digest{providedRef, providedProjection} {
		if provided != "" && provided != digest {
			return ToolInputContractCurrentProjectionV1{}, conflict("supplied Tool Input Contract Projection digest drifted")
		}
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

func DeriveToolInputContractCurrentIDV1(issuance ToolInputContractIssuanceSubjectV1) (string, error) {
	if err := issuance.Validate(); err != nil {
		return "", err
	}
	digest, err := issuance.ComputeDigest()
	if err != nil {
		return "", err
	}
	return "tool-input-contract-v1-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

type ToolInputContractCurrentReaderV1 interface {
	ResolveToolInputContractCurrentV1(context.Context, ToolInputContractResolveRequestV1) (ToolInputContractCurrentProjectionV1, error)
	InspectToolInputContractCurrentByIssuanceV1(context.Context, ToolInputContractInspectByIssuanceRequestV1) (ToolInputContractCurrentProjectionV1, error)
	InspectExactToolInputContractCurrentV1(context.Context, ToolInputContractInspectExactRequestV1) (ToolInputContractCurrentProjectionV1, error)
}

type ToolInputContractLeaseStoreV1 interface {
	CreateToolInputContractCurrentOnceV1(context.Context, ToolInputContractCurrentProjectionV1) (ToolInputContractCurrentProjectionV1, error)
	InspectToolInputContractCurrentByIssuanceIDV1(context.Context, string) (ToolInputContractCurrentProjectionV1, error)
	InspectExactToolInputContractCurrentV1(context.Context, ToolInputContractCurrentRefV1) (ToolInputContractCurrentProjectionV1, error)
}

func CloneToolInputContractCurrentProjectionV1(p ToolInputContractCurrentProjectionV1) ToolInputContractCurrentProjectionV1 {
	p.BindingSubject = cloneToolInputContractBindingSubjectV1(p.BindingSubject)
	p.SurfaceCurrent.Manifest.Entries = append([]ToolSurfaceEntry(nil), p.SurfaceCurrent.Manifest.Entries...)
	for index := range p.SurfaceCurrent.Manifest.Entries {
		p.SurfaceCurrent.Manifest.Entries[index] = cloneToolSurfaceEntryV1(p.SurfaceCurrent.Manifest.Entries[index])
	}
	p.SurfaceCurrent.Manifest.Residuals = append([]Residual(nil), p.SurfaceCurrent.Manifest.Residuals...)
	return p
}

func cloneToolInputContractBindingSubjectV1(s ToolInputContractBindingSubjectV1) ToolInputContractBindingSubjectV1 {
	s.SurfaceEntry = cloneToolSurfaceEntryV1(s.SurfaceEntry)
	return s
}

func cloneToolSurfaceEntryV1(entry ToolSurfaceEntry) ToolSurfaceEntry {
	entry.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), entry.EffectKinds...)
	return entry
}

func containsToolExecuteV1(values []runtimeports.NamespacedNameV2) bool {
	for _, value := range values {
		if value == runtimeports.NamespacedNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3) {
			return true
		}
	}
	return false
}
