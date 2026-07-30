package modelinvoker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	InvocationMaterialSourceLineageContractVersionV2 = "praxis.model-invoker.invocation-material-source-lineage/v2"

	InvocationMaterialContextFrameKindV2          = "praxis.context/frame"
	InvocationMaterialContextMaterialKindV2       = "praxis.context/model-input-material"
	InvocationMaterialToolInjectionMaterialKindV2 = "praxis.tool/model-tool-injection-material"
	InvocationMaterialToolSurfaceKindV2           = "praxis.tool/surface-manifest-current"
)

// InvocationMaterialSourceLineageV2 preserves four different Owner
// coordinates. The derived digests are deliberately not substituted for any
// exact source Ref digest.
type InvocationMaterialSourceLineageV2 struct {
	ContractVersion          string                             `json:"contract_version"`
	ContextFrame             InvocationMaterialExactSourceRefV1 `json:"context_frame"`
	ContextMaterial          InvocationMaterialExactSourceRefV1 `json:"context_material"`
	ToolInjectionMaterial    InvocationMaterialExactSourceRefV1 `json:"tool_injection_material"`
	ToolSurface              InvocationMaterialExactSourceRefV1 `json:"tool_surface"`
	ContextMappedInputDigest core.Digest                        `json:"context_mapped_input_digest"`
	ExpectedInjectionDigest  core.Digest                        `json:"expected_injection_digest"`
	CompiledToolsDigest      core.Digest                        `json:"compiled_tools_digest"`
	RequestToolsDigest       core.Digest                        `json:"request_tools_digest"`
	Digest                   core.Digest                        `json:"digest"`
}

func (l InvocationMaterialSourceLineageV2) Validate() error {
	if l.ContractVersion != InvocationMaterialSourceLineageContractVersionV2 {
		return governedInvalidV1("invocation material source lineage version is invalid")
	}
	refs := []InvocationMaterialExactSourceRefV1{
		l.ContextFrame,
		l.ContextMaterial,
		l.ToolInjectionMaterial,
		l.ToolSurface,
	}
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return governedInvalidV1("invocation material source lineage exact Ref is invalid")
		}
	}
	if l.ContextFrame.Kind != InvocationMaterialContextFrameKindV2 ||
		l.ContextMaterial.Kind != InvocationMaterialContextMaterialKindV2 ||
		l.ToolInjectionMaterial.Kind != InvocationMaterialToolInjectionMaterialKindV2 ||
		l.ToolSurface.Kind != InvocationMaterialToolSurfaceKindV2 {
		return governedConflictV1("invocation material source lineage role Kind drifted")
	}
	if l.ContextFrame.Owner != l.ContextMaterial.Owner {
		return governedConflictV1("invocation material Context source pair Owner drifted")
	}
	if l.ToolInjectionMaterial.Owner != l.ToolSurface.Owner {
		return governedConflictV1("invocation material Tool source pair Owner drifted")
	}
	for left := range refs {
		for right := left + 1; right < len(refs); right++ {
			if refs[left] == refs[right] {
				return governedConflictV1("invocation material source roles were collapsed")
			}
		}
	}
	for _, digest := range []core.Digest{
		l.ContextMappedInputDigest,
		l.ExpectedInjectionDigest,
		l.CompiledToolsDigest,
		l.RequestToolsDigest,
		l.Digest,
	} {
		if err := digest.Validate(); err != nil {
			return governedInvalidV1("invocation material source lineage digest is invalid")
		}
	}
	expected, err := invocationMaterialSourceLineageDigestV2(l)
	if err != nil || expected != l.Digest {
		return governedConflictV1("invocation material source lineage digest drifted")
	}
	return nil
}

func SealInvocationMaterialSourceLineageV2(l InvocationMaterialSourceLineageV2) (InvocationMaterialSourceLineageV2, error) {
	if l.ContractVersion != "" && l.ContractVersion != InvocationMaterialSourceLineageContractVersionV2 {
		return InvocationMaterialSourceLineageV2{}, governedInvalidV1("invocation material source lineage version drifted")
	}
	l.ContractVersion = InvocationMaterialSourceLineageContractVersionV2
	provided := l.Digest
	l.Digest = ""
	digest, err := invocationMaterialSourceLineageDigestV2(l)
	if err != nil {
		return InvocationMaterialSourceLineageV2{}, err
	}
	if provided != "" && provided != digest {
		return InvocationMaterialSourceLineageV2{}, governedConflictV1("supplied invocation material source lineage digest drifted")
	}
	l.Digest = digest
	return l, l.Validate()
}

func invocationMaterialSourceLineageDigestV2(l InvocationMaterialSourceLineageV2) (core.Digest, error) {
	l.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material-source-lineage",
		"v2",
		"InvocationMaterialSourceLineageV2",
		l,
	)
}

type InvocationMaterialContextPairProjectionV2 struct {
	ContextFrame             InvocationMaterialExactSourceRefV1 `json:"context_frame"`
	ContextMaterial          InvocationMaterialExactSourceRefV1 `json:"context_material"`
	ContextMappedInputDigest core.Digest                        `json:"context_mapped_input_digest"`
	CheckedUnixNano          int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                              `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                        `json:"projection_digest"`
}

func (p InvocationMaterialContextPairProjectionV2) ValidateCurrentV2(
	frame InvocationMaterialExactSourceRefV1,
	material InvocationMaterialExactSourceRefV1,
	mappedInput core.Digest,
	now time.Time,
) error {
	if frame.Validate() != nil || material.Validate() != nil || mappedInput.Validate() != nil ||
		p.ContextFrame.Validate() != nil || p.ContextMaterial.Validate() != nil ||
		p.ContextMappedInputDigest.Validate() != nil || p.ContextFrame != frame ||
		p.ContextMaterial != material || p.ContextMappedInputDigest != mappedInput ||
		p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano ||
		now.IsZero() || now.UnixNano() < p.CheckedUnixNano ||
		!now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return governedConflictV1("invocation material Context source pair is not current")
	}
	if p.ContextFrame == p.ContextMaterial {
		return governedConflictV1("invocation material Context source roles were collapsed")
	}
	if p.ContextFrame.Kind != InvocationMaterialContextFrameKindV2 ||
		p.ContextMaterial.Kind != InvocationMaterialContextMaterialKindV2 ||
		p.ContextFrame.Owner != p.ContextMaterial.Owner {
		return governedConflictV1("invocation material Context source pair role drifted")
	}
	expected, err := invocationMaterialContextPairProjectionDigestV2(p)
	if err != nil || p.ProjectionDigest.Validate() != nil || expected != p.ProjectionDigest {
		return governedConflictV1("invocation material Context source projection digest drifted")
	}
	return nil
}

func SealInvocationMaterialContextPairProjectionV2(p InvocationMaterialContextPairProjectionV2) (InvocationMaterialContextPairProjectionV2, error) {
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := invocationMaterialContextPairProjectionDigestV2(p)
	if err != nil {
		return InvocationMaterialContextPairProjectionV2{}, err
	}
	if provided != "" && provided != digest {
		return InvocationMaterialContextPairProjectionV2{}, governedConflictV1("supplied Context source projection digest drifted")
	}
	p.ProjectionDigest = digest
	if err := p.ValidateCurrentV2(p.ContextFrame, p.ContextMaterial, p.ContextMappedInputDigest, time.Unix(0, p.CheckedUnixNano)); err != nil {
		return InvocationMaterialContextPairProjectionV2{}, err
	}
	return p, nil
}

func invocationMaterialContextPairProjectionDigestV2(p InvocationMaterialContextPairProjectionV2) (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material-context-pair",
		"v2",
		"InvocationMaterialContextPairProjectionV2",
		p,
	)
}

type InvocationMaterialToolPairProjectionV2 struct {
	ToolInjectionMaterial   InvocationMaterialExactSourceRefV1 `json:"tool_injection_material"`
	ToolSurface             InvocationMaterialExactSourceRefV1 `json:"tool_surface"`
	ExpectedInjectionDigest core.Digest                        `json:"expected_injection_digest"`
	CompiledToolsDigest     core.Digest                        `json:"compiled_tools_digest"`
	RequestToolsDigest      core.Digest                        `json:"request_tools_digest"`
	CheckedUnixNano         int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                              `json:"expires_unix_nano"`
	ProjectionDigest        core.Digest                        `json:"projection_digest"`
}

func (p InvocationMaterialToolPairProjectionV2) ValidateCurrentV2(
	injection InvocationMaterialExactSourceRefV1,
	surface InvocationMaterialExactSourceRefV1,
	expectedInjection core.Digest,
	compiledTools core.Digest,
	requestTools core.Digest,
	now time.Time,
) error {
	if injection.Validate() != nil || surface.Validate() != nil ||
		expectedInjection.Validate() != nil || compiledTools.Validate() != nil ||
		requestTools.Validate() != nil || p.ToolInjectionMaterial.Validate() != nil ||
		p.ToolSurface.Validate() != nil || p.ExpectedInjectionDigest.Validate() != nil ||
		p.CompiledToolsDigest.Validate() != nil || p.RequestToolsDigest.Validate() != nil ||
		p.ToolInjectionMaterial != injection || p.ToolSurface != surface ||
		p.ExpectedInjectionDigest != expectedInjection ||
		p.CompiledToolsDigest != compiledTools || p.RequestToolsDigest != requestTools ||
		p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano ||
		now.IsZero() || now.UnixNano() < p.CheckedUnixNano ||
		!now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return governedConflictV1("invocation material Tool source pair is not current")
	}
	if p.ToolInjectionMaterial == p.ToolSurface {
		return governedConflictV1("invocation material Tool source roles were collapsed")
	}
	if p.ToolInjectionMaterial.Kind != InvocationMaterialToolInjectionMaterialKindV2 ||
		p.ToolSurface.Kind != InvocationMaterialToolSurfaceKindV2 ||
		p.ToolInjectionMaterial.Owner != p.ToolSurface.Owner {
		return governedConflictV1("invocation material Tool source pair role drifted")
	}
	expected, err := invocationMaterialToolPairProjectionDigestV2(p)
	if err != nil || p.ProjectionDigest.Validate() != nil || expected != p.ProjectionDigest {
		return governedConflictV1("invocation material Tool source projection digest drifted")
	}
	return nil
}

func SealInvocationMaterialToolPairProjectionV2(p InvocationMaterialToolPairProjectionV2) (InvocationMaterialToolPairProjectionV2, error) {
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := invocationMaterialToolPairProjectionDigestV2(p)
	if err != nil {
		return InvocationMaterialToolPairProjectionV2{}, err
	}
	if provided != "" && provided != digest {
		return InvocationMaterialToolPairProjectionV2{}, governedConflictV1("supplied Tool source projection digest drifted")
	}
	p.ProjectionDigest = digest
	if err := p.ValidateCurrentV2(
		p.ToolInjectionMaterial,
		p.ToolSurface,
		p.ExpectedInjectionDigest,
		p.CompiledToolsDigest,
		p.RequestToolsDigest,
		time.Unix(0, p.CheckedUnixNano),
	); err != nil {
		return InvocationMaterialToolPairProjectionV2{}, err
	}
	return p, nil
}

func invocationMaterialToolPairProjectionDigestV2(p InvocationMaterialToolPairProjectionV2) (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material-tool-pair",
		"v2",
		"InvocationMaterialToolPairProjectionV2",
		p,
	)
}

// InvocationMaterialContextPairExactReaderV2 is a neutral Model-owned port.
// A future cross-Owner adapter must prove the exact Context material-to-request
// lowering, including the RouteCall Instructions/Input bytes, before returning
// this projection. An owner-local or test reader proves no cross-Owner closure
// and is not production authorization.
type InvocationMaterialContextPairExactReaderV2 interface {
	InspectExactInvocationContextPairV2(
		context.Context,
		InvocationMaterialExactSourceRefV1,
		InvocationMaterialExactSourceRefV1,
		core.Digest,
	) (InvocationMaterialContextPairProjectionV2, error)
}

// InvocationMaterialToolPairExactReaderV2 is a neutral Model-owned port. A
// future Tool adapter must prove that its stored compiled tools close the
// returned RequestToolsDigest before returning this projection.
type InvocationMaterialToolPairExactReaderV2 interface {
	InspectExactInvocationToolPairV2(
		context.Context,
		InvocationMaterialExactSourceRefV1,
		InvocationMaterialExactSourceRefV1,
		core.Digest,
	) (InvocationMaterialToolPairProjectionV2, error)
}

type InvocationMaterialExactClosureV2 struct {
	SourceLineage     InvocationMaterialSourceLineageV2  `json:"source_lineage"`
	ProviderInjection InvocationMaterialExactSourceRefV1 `json:"provider_injection"`
	Route             InvocationMaterialExactSourceRefV1 `json:"route"`
	Profile           InvocationMaterialExactSourceRefV1 `json:"profile"`
}

func (c InvocationMaterialExactClosureV2) ValidateAgainstPreparedV2(prepared PreparedModelInvocationFactV1) error {
	if c.SourceLineage.Validate() != nil || c.ProviderInjection.Validate() != nil ||
		c.Route.Validate() != nil || c.Profile.Validate() != nil ||
		prepared.Validate() != nil {
		return governedInvalidV1("invocation material V2 exact closure is invalid")
	}
	if c.SourceLineage.ExpectedInjectionDigest != prepared.ActualToolSurfaceDigest ||
		c.SourceLineage.RequestToolsDigest != prepared.RequestToolsDigest ||
		c.ProviderInjection.Digest != prepared.ActualProviderInjectionDigest ||
		c.Route.Digest != prepared.RouteDigest ||
		c.Profile.Digest != prepared.ProfileDigest {
		return governedConflictV1("invocation material V2 exact closure differs from Prepared")
	}
	return nil
}

type InvocationMaterialAuthorizerConfigV2 struct {
	ContextPair       InvocationMaterialContextPairExactReaderV2
	ToolPair          InvocationMaterialToolPairExactReaderV2
	ProviderInjection InvocationMaterialProviderInjectionExactReaderV1
	Route             InvocationMaterialRouteExactReaderV1
	Profile           InvocationMaterialProfileExactReaderV1
}

type InvocationMaterialAuthorizerV2 struct {
	config InvocationMaterialAuthorizerConfigV2
}

func NewInvocationMaterialAuthorizerV2(config InvocationMaterialAuthorizerConfigV2) (*InvocationMaterialAuthorizerV2, error) {
	for _, dependency := range []any{
		config.ContextPair,
		config.ToolPair,
		config.ProviderInjection,
		config.Route,
		config.Profile,
	} {
		if nilLikeInvocationMaterialV1(dependency) {
			return nil, governedInvalidV1("invocation material V2 Authorizer requires five exact readers")
		}
	}
	return &InvocationMaterialAuthorizerV2{config: config}, nil
}

func DigestGovernedModelTurnContextV2(call RouteCall) (core.Digest, error) {
	canonical, err := canonicalInvocationMaterialCallV1(call)
	if err != nil {
		return "", err
	}
	if _, err := DigestGovernedModelTurnRouteCallV2(canonical); err != nil {
		return "", err
	}
	return DigestGovernedModelTurnContextBodyV2(
		canonical.Request.Instructions,
		canonical.Request.Input,
	)
}

// DigestGovernedModelTurnContextBodyV2 seals the exact Model-owned
// Instructions/Input body without requiring another Owner to synthesize a
// RouteCall. It does not define or authorize any Context Channel lowering.
func DigestGovernedModelTurnContextBodyV2(
	instructions []Instruction,
	input []InputItem,
) (core.Digest, error) {
	body, err := canonicalGovernedModelTurnContextBodyV2(instructions, input)
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-context",
		"v2",
		"GovernedModelTurnContextV2",
		body,
	)
}

func canonicalGovernedModelTurnContextBodyV2(
	instructions []Instruction,
	input []InputItem,
) (struct {
	Instructions []Instruction `json:"instructions"`
	Input        []InputItem   `json:"input"`
}, error) {
	body := struct {
		Instructions []Instruction `json:"instructions"`
		Input        []InputItem   `json:"input"`
	}{
		Instructions: instructions,
		Input:        input,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return body, err
	}
	var canonical struct {
		Instructions []Instruction `json:"instructions"`
		Input        []InputItem   `json:"input"`
	}
	if err := core.DecodeStrictJSON(payload, &canonical); err != nil {
		return body, err
	}
	if len(canonical.Input) == 0 {
		return body, governedInvalidV1("governed model turn requires at least one input item")
	}
	for _, instruction := range canonical.Instructions {
		if instruction.Role != RoleSystem && instruction.Role != RoleDeveloper {
			return body, governedInvalidV1("governed model turn instruction role is invalid")
		}
		if strings.TrimSpace(instruction.Text) == "" {
			return body, governedInvalidV1("governed model turn instruction text is required")
		}
	}
	for _, item := range canonical.Input {
		if err := validateInputItem(item); err != nil {
			return body, governedInvalidV1("governed model turn input item is invalid")
		}
	}
	return canonical, nil
}

func DigestGovernedModelTurnRequestToolsV2(call RouteCall) (core.Digest, error) {
	canonical, err := canonicalInvocationMaterialCallV1(call)
	if err != nil {
		return "", err
	}
	if _, err := DigestGovernedModelTurnRouteCallV2(canonical); err != nil {
		return "", err
	}
	return DigestGovernedModelTurnRequestToolSetV2(canonical.Request.Tools)
}

// DigestGovernedModelTurnRequestToolSetV2 seals the exact Model-owned
// Request.Tools bytes without requiring another Owner to synthesize a
// RouteCall. It deliberately uses the same canonical frame and body as
// DigestGovernedModelTurnRequestToolsV2.
func DigestGovernedModelTurnRequestToolSetV2(tools []Tool) (core.Digest, error) {
	canonical, err := canonicalGovernedModelTurnRequestToolSetV2(tools)
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.governed-model-turn-request-tools",
		"v2",
		"GovernedModelTurnRequestToolsV2",
		canonical,
	)
}

func canonicalGovernedModelTurnRequestToolSetV2(tools []Tool) ([]Tool, error) {
	payload, err := json.Marshal(tools)
	if err != nil {
		return nil, err
	}
	var canonical []Tool
	if err := core.DecodeStrictJSON(payload, &canonical); err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nil, governedInvalidV1("governed model turn requires tools and explicit non-parallel execution")
	}
	names := make(map[string]struct{}, len(canonical))
	for _, tool := range canonical {
		if strings.TrimSpace(tool.Name) == "" || tool.Strict == nil || !*tool.Strict {
			return nil, governedInvalidV1("governed model turn requires strict named tools")
		}
		if _, ok := names[tool.Name]; ok {
			return nil, governedConflictV1("governed model turn tool names must be unique")
		}
		names[tool.Name] = struct{}{}
		if err := validateStrictJSONObjectV1(tool.Parameters); err != nil {
			return nil, err
		}
	}
	return canonical, nil
}

func sameInvocationMaterialAuthorizationClosureV2(left, right InvocationMaterialAuthorizationV2) bool {
	return left.PreparedRef == right.PreparedRef &&
		left.CurrentRef == right.CurrentRef &&
		left.RouteCallDigest == right.RouteCallDigest &&
		reflect.DeepEqual(left.SourceLineage, right.SourceLineage) &&
		left.ProviderInjectionRef == right.ProviderInjectionRef &&
		left.RouteRef == right.RouteRef &&
		left.ProfileRef == right.ProfileRef
}
