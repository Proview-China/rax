package modelinvoker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	InvocationMaterialContractVersionV2              = "praxis.model-invoker.invocation-material/v2"
	InvocationMaterialAuthorizationContractVersionV2 = "praxis.model-invoker.invocation-material-authorization/v2"
)

type InvocationMaterialAuthorizationRefV2 struct {
	ContractVersion string                              `json:"contract_version"`
	ID              string                              `json:"id"`
	Revision        core.Revision                       `json:"revision"`
	Digest          core.Digest                         `json:"digest"`
	PreparedRef     PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef      PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	RouteCallDigest core.Digest                         `json:"route_call_digest"`
	SourceLineage   InvocationMaterialSourceLineageV2   `json:"source_lineage"`
	ExpiresUnixNano int64                               `json:"expires_unix_nano"`
}

type InvocationMaterialAuthorizationV2 struct {
	ContractVersion      string                              `json:"contract_version"`
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	Digest               core.Digest                         `json:"digest"`
	PreparedRef          PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef           PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	RouteCallDigest      core.Digest                         `json:"route_call_digest"`
	SourceLineage        InvocationMaterialSourceLineageV2   `json:"source_lineage"`
	ProviderInjectionRef InvocationMaterialExactSourceRefV1  `json:"provider_injection_ref"`
	RouteRef             InvocationMaterialExactSourceRefV1  `json:"route_ref"`
	ProfileRef           InvocationMaterialExactSourceRefV1  `json:"profile_ref"`
	AuthorizedUnixNano   int64                               `json:"authorized_unix_nano"`
	ExpiresUnixNano      int64                               `json:"expires_unix_nano"`
}

func (a InvocationMaterialAuthorizationV2) RefV2() InvocationMaterialAuthorizationRefV2 {
	return InvocationMaterialAuthorizationRefV2{
		ContractVersion: a.ContractVersion,
		ID:              a.ID,
		Revision:        a.Revision,
		Digest:          a.Digest,
		PreparedRef:     a.PreparedRef,
		CurrentRef:      a.CurrentRef,
		RouteCallDigest: a.RouteCallDigest,
		SourceLineage:   a.SourceLineage,
		ExpiresUnixNano: a.ExpiresUnixNano,
	}
}

func (r InvocationMaterialAuthorizationRefV2) Validate() error {
	if r.ContractVersion != InvocationMaterialAuthorizationContractVersionV2 ||
		strings.TrimSpace(r.ID) == "" || r.Revision != 1 ||
		r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil ||
		r.CurrentRef.Validate() != nil || r.CurrentRef.Prepared != r.PreparedRef ||
		r.RouteCallDigest.Validate() != nil || r.SourceLineage.Validate() != nil ||
		r.ExpiresUnixNano <= r.CurrentRef.CheckedUnixNano {
		return governedInvalidV1("invocation material V2 authorization Ref is invalid")
	}
	expectedID, err := invocationMaterialAuthorizationIdentityV2(
		r.PreparedRef,
		r.CurrentRef,
		r.RouteCallDigest,
	)
	if err != nil || expectedID != r.ID {
		return governedConflictV1("invocation material V2 authorization Ref identity drifted")
	}
	return nil
}

func (a InvocationMaterialAuthorizationV2) Validate() error {
	if a.RefV2().Validate() != nil || a.SourceLineage.Validate() != nil ||
		a.ProviderInjectionRef.Validate() != nil || a.RouteRef.Validate() != nil ||
		a.ProfileRef.Validate() != nil || a.AuthorizedUnixNano <= 0 ||
		a.ExpiresUnixNano <= a.AuthorizedUnixNano ||
		a.SourceLineage != a.RefV2().SourceLineage {
		return governedInvalidV1("invocation material V2 authorization is invalid")
	}
	expectedID, err := invocationMaterialAuthorizationIdentityV2(
		a.PreparedRef,
		a.CurrentRef,
		a.RouteCallDigest,
	)
	if err != nil || expectedID != a.ID {
		return governedConflictV1("invocation material V2 authorization identity drifted")
	}
	expected, err := invocationMaterialAuthorizationDigestV2(a)
	if err != nil || expected != a.Digest {
		return governedConflictV1("invocation material V2 authorization digest drifted")
	}
	return nil
}

func (a InvocationMaterialAuthorizationV2) ValidateAgainstV2(
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	routeCallDigest core.Digest,
	contextMappedInputDigest core.Digest,
	requestToolsDigest core.Digest,
	now time.Time,
) error {
	if a.Validate() != nil || prepared.Validate() != nil ||
		current.ValidateAgainstFact(prepared) != nil ||
		current.ValidateCurrent(current.Ref(), now) != nil || now.IsZero() ||
		!now.Before(time.Unix(0, a.ExpiresUnixNano)) ||
		a.ExpiresUnixNano > current.ExpiresUnixNano ||
		a.ExpiresUnixNano > prepared.NotAfterUnixNano ||
		a.PreparedRef != prepared.Ref() || a.CurrentRef != current.Ref() ||
		a.RouteCallDigest != routeCallDigest ||
		a.SourceLineage.ContextMappedInputDigest != contextMappedInputDigest ||
		a.SourceLineage.RequestToolsDigest != requestToolsDigest ||
		requestToolsDigest != prepared.RequestToolsDigest ||
		a.SourceLineage.ExpectedInjectionDigest != prepared.ActualToolSurfaceDigest ||
		a.ProviderInjectionRef.Digest != prepared.ActualProviderInjectionDigest ||
		a.RouteRef.Digest != prepared.RouteDigest ||
		a.ProfileRef.Digest != prepared.ProfileDigest {
		return governedConflictV1("invocation material V2 authorization differs from exact Prepared/current sources")
	}
	return nil
}

func SealInvocationMaterialAuthorizationV2(a InvocationMaterialAuthorizationV2) (InvocationMaterialAuthorizationV2, error) {
	if a.ContractVersion != "" && a.ContractVersion != InvocationMaterialAuthorizationContractVersionV2 {
		return InvocationMaterialAuthorizationV2{}, governedInvalidV1("invocation material V2 authorization version drifted")
	}
	a.ContractVersion = InvocationMaterialAuthorizationContractVersionV2
	a.Revision = 1
	id, err := invocationMaterialAuthorizationIdentityV2(
		a.PreparedRef,
		a.CurrentRef,
		a.RouteCallDigest,
	)
	if err != nil {
		return InvocationMaterialAuthorizationV2{}, err
	}
	if a.ID != "" && a.ID != id {
		return InvocationMaterialAuthorizationV2{}, governedConflictV1("invocation material V2 authorization ID drifted")
	}
	a.ID = id
	provided := a.Digest
	a.Digest = ""
	a.Digest, err = invocationMaterialAuthorizationDigestV2(a)
	if err != nil {
		return InvocationMaterialAuthorizationV2{}, err
	}
	if provided != "" && provided != a.Digest {
		return InvocationMaterialAuthorizationV2{}, governedConflictV1("supplied invocation material V2 authorization digest drifted")
	}
	return a, a.Validate()
}

type invocationMaterialAuthorizationClosureV3 struct {
	authorization InvocationMaterialAuthorizationV2
	contextPair   InvocationMaterialContextPairProjectionV2
	toolPair      InvocationMaterialToolPairProjectionV2
}

func (a *InvocationMaterialAuthorizerV2) authorizeV2(
	ctx context.Context,
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	call RouteCall,
	expected InvocationMaterialExactClosureV2,
	now time.Time,
) (InvocationMaterialAuthorizationV2, error) {
	closure, err := a.authorizeClosureV3(
		ctx,
		prepared,
		current,
		call,
		expected,
		now,
	)
	if err != nil {
		return InvocationMaterialAuthorizationV2{}, err
	}
	return closure.authorization, nil
}

// authorizeClosureV3 derives the authorization and the complete Context/Tool
// projections from the same authoritative reads. Its compatibility wrapper
// above preserves the V2 contract while V3 S1/S2 callers can freeze the exact
// source bodies without a second-read TOCTOU window.
func (a *InvocationMaterialAuthorizerV2) authorizeClosureV3(
	ctx context.Context,
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	call RouteCall,
	expected InvocationMaterialExactClosureV2,
	now time.Time,
) (invocationMaterialAuthorizationClosureV3, error) {
	var closure invocationMaterialAuthorizationClosureV3
	if a == nil || ctx == nil || ctx.Err() != nil || now.IsZero() ||
		expected.ValidateAgainstPreparedV2(prepared) != nil ||
		current.ValidateAgainstFact(prepared) != nil ||
		current.ValidateCurrent(current.Ref(), now) != nil {
		return closure, governedInvalidV1("invocation material V2 Authorizer input is invalid")
	}
	contextDigest, err := DigestGovernedModelTurnContextV2(call)
	if err != nil {
		return closure, err
	}
	requestToolsDigest, err := DigestGovernedModelTurnRequestToolsV2(call)
	if err != nil {
		return closure, err
	}
	if contextDigest != expected.SourceLineage.ContextMappedInputDigest ||
		requestToolsDigest != expected.SourceLineage.RequestToolsDigest {
		return closure, governedConflictV1("RouteCall bytes differ from expected source lineage")
	}
	contextPair, err := a.config.ContextPair.InspectExactInvocationContextPairV2(
		ctx,
		expected.SourceLineage.ContextFrame,
		expected.SourceLineage.ContextMaterial,
		contextDigest,
	)
	if err != nil {
		return closure, err
	}
	if err := contextPair.ValidateCurrentV2(
		expected.SourceLineage.ContextFrame,
		expected.SourceLineage.ContextMaterial,
		contextDigest,
		now,
	); err != nil {
		return closure, err
	}
	toolPair, err := a.config.ToolPair.InspectExactInvocationToolPairV2(
		ctx,
		expected.SourceLineage.ToolInjectionMaterial,
		expected.SourceLineage.ToolSurface,
		requestToolsDigest,
	)
	if err != nil {
		return closure, err
	}
	if err := toolPair.ValidateCurrentV2(
		expected.SourceLineage.ToolInjectionMaterial,
		expected.SourceLineage.ToolSurface,
		expected.SourceLineage.ExpectedInjectionDigest,
		expected.SourceLineage.CompiledToolsDigest,
		requestToolsDigest,
		now,
	); err != nil {
		return closure, err
	}
	provider, err := a.config.ProviderInjection.InspectExactInvocationProviderInjectionV1(ctx, expected.ProviderInjection)
	if err != nil || provider.ValidateCurrentV1(expected.ProviderInjection, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material V2 ProviderInjection source is not current")
		}
		return closure, err
	}
	route, err := a.config.Route.InspectExactInvocationRouteV1(ctx, expected.Route)
	if err != nil || route.ValidateCurrentV1(expected.Route, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material V2 Route source is not current")
		}
		return closure, err
	}
	profile, err := a.config.Profile.InspectExactInvocationProfileV1(ctx, expected.Profile)
	if err != nil || profile.ValidateCurrentV1(expected.Profile, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material V2 Profile source is not current")
		}
		return closure, err
	}
	observed, err := SealInvocationMaterialSourceLineageV2(InvocationMaterialSourceLineageV2{
		ContextFrame:             contextPair.ContextFrame,
		ContextMaterial:          contextPair.ContextMaterial,
		ToolInjectionMaterial:    toolPair.ToolInjectionMaterial,
		ToolSurface:              toolPair.ToolSurface,
		ContextMappedInputDigest: contextPair.ContextMappedInputDigest,
		ExpectedInjectionDigest:  toolPair.ExpectedInjectionDigest,
		CompiledToolsDigest:      toolPair.CompiledToolsDigest,
		RequestToolsDigest:       toolPair.RequestToolsDigest,
	})
	if err != nil || observed != expected.SourceLineage ||
		provider.Ref != expected.ProviderInjection ||
		route.Ref != expected.Route || profile.Ref != expected.Profile {
		return closure, governedConflictV1("invocation material V2 exact Owner readers drifted")
	}
	routeCallDigest, err := DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		return closure, err
	}
	authorization, err := SealInvocationMaterialAuthorizationV2(InvocationMaterialAuthorizationV2{
		PreparedRef:          prepared.Ref(),
		CurrentRef:           current.Ref(),
		RouteCallDigest:      routeCallDigest,
		SourceLineage:        observed,
		ProviderInjectionRef: provider.Ref,
		RouteRef:             route.Ref,
		ProfileRef:           profile.Ref,
		AuthorizedUnixNano:   now.UnixNano(),
		ExpiresUnixNano: minTimeUnixNanoMaterialV1(
			prepared.NotAfterUnixNano,
			current.ExpiresUnixNano,
			contextPair.ExpiresUnixNano,
			toolPair.ExpiresUnixNano,
			provider.ExpiresUnixNano,
			route.ExpiresUnixNano,
			profile.ExpiresUnixNano,
		),
	})
	if err != nil {
		return closure, err
	}
	closure = invocationMaterialAuthorizationClosureV3{
		authorization: authorization,
		contextPair:   contextPair,
		toolPair:      toolPair,
	}
	return closure, nil
}

type InvocationMaterialRefV2 struct {
	ContractVersion          string                               `json:"contract_version"`
	ID                       string                               `json:"id"`
	Revision                 core.Revision                        `json:"revision"`
	Digest                   core.Digest                          `json:"digest"`
	PreparedRef              PreparedModelInvocationRefV1         `json:"prepared_ref"`
	RouteCallDigest          core.Digest                          `json:"route_call_digest"`
	UnifiedRequestDigest     core.Digest                          `json:"unified_request_digest"`
	RequestToolsDigest       core.Digest                          `json:"request_tools_digest"`
	ContextMappedInputDigest core.Digest                          `json:"context_mapped_input_digest"`
	PreparedPlanDigest       core.Digest                          `json:"prepared_plan_digest"`
	RouteDigest              core.Digest                          `json:"route_digest"`
	ProfileDigest            core.Digest                          `json:"profile_digest"`
	ModelDigest              core.Digest                          `json:"model_digest"`
	BudgetDigest             core.Digest                          `json:"budget_digest"`
	ToolChoiceDigest         core.Digest                          `json:"tool_choice_digest"`
	ConsumerBindingDigest    core.Digest                          `json:"consumer_binding_digest"`
	AuthorizationRef         InvocationMaterialAuthorizationRefV2 `json:"authorization_ref"`
	ExpiresUnixNano          int64                                `json:"expires_unix_nano"`
}

type InvocationMaterialV2 struct {
	ContractVersion          string                            `json:"contract_version"`
	ID                       string                            `json:"id"`
	Revision                 core.Revision                     `json:"revision"`
	Digest                   core.Digest                       `json:"digest"`
	PreparedRef              PreparedModelInvocationRefV1      `json:"prepared_ref"`
	RouteCallDigest          core.Digest                       `json:"route_call_digest"`
	UnifiedRequestDigest     core.Digest                       `json:"unified_request_digest"`
	RequestToolsDigest       core.Digest                       `json:"request_tools_digest"`
	ContextMappedInputDigest core.Digest                       `json:"context_mapped_input_digest"`
	PreparedPlanDigest       core.Digest                       `json:"prepared_plan_digest"`
	RouteDigest              core.Digest                       `json:"route_digest"`
	ProfileDigest            core.Digest                       `json:"profile_digest"`
	ModelDigest              core.Digest                       `json:"model_digest"`
	BudgetDigest             core.Digest                       `json:"budget_digest"`
	ToolChoiceDigest         core.Digest                       `json:"tool_choice_digest"`
	ConsumerBindingDigest    core.Digest                       `json:"consumer_binding_digest"`
	Authorization            InvocationMaterialAuthorizationV2 `json:"authorization"`
	Call                     RouteCall                         `json:"call"`
	CreatedUnixNano          int64                             `json:"created_unix_nano"`
	ExpiresUnixNano          int64                             `json:"expires_unix_nano"`
}

func (m InvocationMaterialV2) RefV2() InvocationMaterialRefV2 {
	return InvocationMaterialRefV2{
		ContractVersion:          m.ContractVersion,
		ID:                       m.ID,
		Revision:                 m.Revision,
		Digest:                   m.Digest,
		PreparedRef:              m.PreparedRef,
		RouteCallDigest:          m.RouteCallDigest,
		UnifiedRequestDigest:     m.UnifiedRequestDigest,
		RequestToolsDigest:       m.RequestToolsDigest,
		ContextMappedInputDigest: m.ContextMappedInputDigest,
		PreparedPlanDigest:       m.PreparedPlanDigest,
		RouteDigest:              m.RouteDigest,
		ProfileDigest:            m.ProfileDigest,
		ModelDigest:              m.ModelDigest,
		BudgetDigest:             m.BudgetDigest,
		ToolChoiceDigest:         m.ToolChoiceDigest,
		ConsumerBindingDigest:    m.ConsumerBindingDigest,
		AuthorizationRef:         m.Authorization.RefV2(),
		ExpiresUnixNano:          m.ExpiresUnixNano,
	}
}

func (r InvocationMaterialRefV2) Validate() error {
	if r.ContractVersion != InvocationMaterialContractVersionV2 ||
		strings.TrimSpace(r.ID) == "" || r.Revision != 1 ||
		r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil ||
		r.AuthorizationRef.Validate() != nil || r.ExpiresUnixNano <= 0 ||
		r.AuthorizationRef.PreparedRef != r.PreparedRef ||
		r.AuthorizationRef.RouteCallDigest != r.RouteCallDigest ||
		r.AuthorizationRef.SourceLineage.RequestToolsDigest != r.RequestToolsDigest ||
		r.AuthorizationRef.SourceLineage.ContextMappedInputDigest != r.ContextMappedInputDigest ||
		r.ExpiresUnixNano > r.AuthorizationRef.ExpiresUnixNano {
		return governedInvalidV1("invocation material V2 exact Ref is invalid")
	}
	for _, digest := range []core.Digest{
		r.RouteCallDigest,
		r.UnifiedRequestDigest,
		r.RequestToolsDigest,
		r.ContextMappedInputDigest,
		r.PreparedPlanDigest,
		r.RouteDigest,
		r.ProfileDigest,
		r.ModelDigest,
		r.BudgetDigest,
		r.ToolChoiceDigest,
		r.ConsumerBindingDigest,
	} {
		if digest.Validate() != nil {
			return governedInvalidV1("invocation material V2 exact Ref digest is invalid")
		}
	}
	id, err := invocationMaterialIdentityV2(
		r.PreparedRef,
		r.RouteCallDigest,
		r.AuthorizationRef.ID,
	)
	if err != nil || id != r.ID {
		return governedConflictV1("invocation material V2 exact Ref identity drifted")
	}
	return nil
}

func (m InvocationMaterialV2) Validate() error {
	if m.ContractVersion != InvocationMaterialContractVersionV2 ||
		m.Revision != 1 || m.CreatedUnixNano <= 0 ||
		m.ExpiresUnixNano <= m.CreatedUnixNano ||
		m.ExpiresUnixNano > m.Authorization.ExpiresUnixNano ||
		m.RefV2().Validate() != nil || m.Authorization.Validate() != nil ||
		m.Authorization.RefV2() != m.RefV2().AuthorizationRef ||
		m.RequestToolsDigest != m.Authorization.SourceLineage.RequestToolsDigest ||
		m.ContextMappedInputDigest != m.Authorization.SourceLineage.ContextMappedInputDigest {
		return governedInvalidV1("invocation material V2 fields are invalid")
	}
	routeCallDigest, err := DigestGovernedModelTurnRouteCallV2(m.Call)
	if err != nil || routeCallDigest != m.RouteCallDigest {
		return governedConflictV1("invocation material V2 RouteCall drifted")
	}
	contextDigest, err := DigestGovernedModelTurnContextV2(m.Call)
	if err != nil || contextDigest != m.ContextMappedInputDigest {
		return governedConflictV1("invocation material V2 Context mapping drifted")
	}
	requestToolsDigest, err := DigestGovernedModelTurnRequestToolsV2(m.Call)
	if err != nil || requestToolsDigest != m.RequestToolsDigest {
		return governedConflictV1("invocation material V2 request Tools drifted")
	}
	model, budget, choice, consumer, err := invocationMaterialComponentDigestsV1(m.Call)
	if err != nil || model != m.ModelDigest || budget != m.BudgetDigest ||
		choice != m.ToolChoiceDigest || consumer != m.ConsumerBindingDigest {
		return governedConflictV1("invocation material V2 component digest drifted")
	}
	if m.UnifiedRequestDigest != m.PreparedRef.UnifiedRequestDigest {
		return governedConflictV1("invocation material V2 request digest drifted")
	}
	expected, err := invocationMaterialDigestV2(m)
	if err != nil || expected != m.Digest {
		return governedConflictV1("invocation material V2 digest drifted")
	}
	return nil
}

func (m InvocationMaterialV2) ValidateAgainstPreparedV2(
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	now time.Time,
) error {
	if m.Validate() != nil || prepared.Validate() != nil || now.IsZero() ||
		!now.Before(time.Unix(0, m.ExpiresUnixNano)) ||
		current.ValidateAgainstFact(prepared) != nil ||
		m.PreparedRef != prepared.Ref() ||
		m.UnifiedRequestDigest != prepared.UnifiedRequestDigest ||
		m.RequestToolsDigest != prepared.RequestToolsDigest ||
		m.PreparedPlanDigest != prepared.PreparedPlanDigest ||
		m.RouteDigest != prepared.RouteDigest ||
		m.ProfileDigest != prepared.ProfileDigest ||
		m.Authorization.SourceLineage.ExpectedInjectionDigest != prepared.ActualToolSurfaceDigest ||
		m.ExpiresUnixNano > prepared.NotAfterUnixNano ||
		m.Authorization.ValidateAgainstV2(
			prepared,
			current,
			m.RouteCallDigest,
			m.ContextMappedInputDigest,
			m.RequestToolsDigest,
			now,
		) != nil {
		return governedConflictV1("invocation material V2 differs from exact durable Prepared fact")
	}
	return nil
}

func SealInvocationMaterialV2(m InvocationMaterialV2) (InvocationMaterialV2, error) {
	if m.ContractVersion != "" && m.ContractVersion != InvocationMaterialContractVersionV2 {
		return InvocationMaterialV2{}, governedInvalidV1("invocation material V2 version drifted")
	}
	m.ContractVersion = InvocationMaterialContractVersionV2
	m.Revision = 1
	canonical, err := canonicalInvocationMaterialCallV1(m.Call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	m.Call = canonical
	m.RouteCallDigest, err = DigestGovernedModelTurnRouteCallV2(m.Call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	m.ContextMappedInputDigest, err = DigestGovernedModelTurnContextV2(m.Call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	m.RequestToolsDigest, err = DigestGovernedModelTurnRequestToolsV2(m.Call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	if m.Authorization.Validate() != nil ||
		m.Authorization.SourceLineage.ContextMappedInputDigest != m.ContextMappedInputDigest ||
		m.Authorization.SourceLineage.RequestToolsDigest != m.RequestToolsDigest ||
		m.Authorization.PreparedRef != m.PreparedRef ||
		m.Authorization.RouteCallDigest != m.RouteCallDigest {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 lacks exact authorization closure")
	}
	model, budget, choice, consumer, err := invocationMaterialComponentDigestsV1(m.Call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	m.ModelDigest, m.BudgetDigest = model, budget
	m.ToolChoiceDigest, m.ConsumerBindingDigest = choice, consumer
	id, err := invocationMaterialIdentityV2(
		m.PreparedRef,
		m.RouteCallDigest,
		m.Authorization.ID,
	)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	if m.ID != "" && m.ID != id {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 ID drifted")
	}
	m.ID = id
	provided := m.Digest
	m.Digest = ""
	m = m.CloneV2()
	m.Digest, err = invocationMaterialDigestV2(m)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	if provided != "" && provided != m.Digest {
		return InvocationMaterialV2{}, governedConflictV1("supplied invocation material V2 digest drifted")
	}
	return m, m.Validate()
}

func (m InvocationMaterialV2) CloneV2() InvocationMaterialV2 {
	payload, _ := json.Marshal(m)
	var clone InvocationMaterialV2
	_ = core.DecodeStrictJSON(payload, &clone)
	if string(clone.Call.Request.Output.Schema) == "null" {
		clone.Call.Request.Output.Schema = nil
	}
	return clone
}

func EncodeInvocationMaterialV2(m InvocationMaterialV2) (json.RawMessage, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func DecodeInvocationMaterialV2(payload json.RawMessage) (InvocationMaterialV2, error) {
	var material InvocationMaterialV2
	if err := core.DecodeStrictJSON(payload, &material); err != nil {
		return InvocationMaterialV2{}, err
	}
	if string(material.Call.Request.Output.Schema) == "null" {
		material.Call.Request.Output.Schema = nil
	}
	return material.CloneV2(), material.Validate()
}

type InvocationMaterialPersistRequestV2 struct {
	material      InvocationMaterialV2
	authorization InvocationMaterialAuthorizationV2
}

func (r InvocationMaterialPersistRequestV2) ValidateV2() error {
	if r.material.Validate() != nil || r.authorization.Validate() != nil ||
		!reflect.DeepEqual(r.material.Authorization, r.authorization) {
		return governedConflictV1("invocation material V2 persist request lacks exact authorization")
	}
	return nil
}

func (r InvocationMaterialPersistRequestV2) MaterialV2() InvocationMaterialV2 {
	return r.material.CloneV2()
}

type InvocationMaterialReaderV2 interface {
	InspectExactInvocationMaterialV2(context.Context, InvocationMaterialRefV2) (InvocationMaterialV2, error)
}

type InvocationMaterialRepositoryV2 interface {
	InvocationMaterialReaderV2
	EnsureAuthorizedInvocationMaterialV2(context.Context, InvocationMaterialPersistRequestV2) (InvocationMaterialV2, error)
}

func AuthorizeAndEnsureInvocationMaterialV2(
	ctx context.Context,
	authorizer *InvocationMaterialAuthorizerV2,
	repository InvocationMaterialRepositoryV2,
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	call RouteCall,
	closure InvocationMaterialExactClosureV2,
	clock func() time.Time,
) (InvocationMaterialV2, error) {
	if ctx == nil || ctx.Err() != nil || authorizer == nil ||
		nilLikeInvocationMaterialV1(repository) || clock == nil ||
		prepared.Validate() != nil || current.ValidateAgainstFact(prepared) != nil ||
		closure.ValidateAgainstPreparedV2(prepared) != nil {
		return InvocationMaterialV2{}, governedInvalidV1("invocation material V2 constructor input is invalid")
	}
	canonical, err := canonicalInvocationMaterialCallV1(call)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	s1 := clock()
	if s1.IsZero() || current.ValidateCurrent(current.Ref(), s1) != nil {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 S1 is not current")
	}
	authorizationS1, err := authorizer.authorizeV2(ctx, prepared, current, canonical, closure, s1)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	contextDigest, err := DigestGovernedModelTurnContextV2(canonical)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	requestToolsDigest, err := DigestGovernedModelTurnRequestToolsV2(canonical)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	routeCallDigest, err := DigestGovernedModelTurnRouteCallV2(canonical)
	if err != nil || authorizationS1.ValidateAgainstV2(
		prepared,
		current,
		routeCallDigest,
		contextDigest,
		requestToolsDigest,
		s1,
	) != nil {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 S1 authorization is not exact")
	}
	s2 := clock()
	if s2.IsZero() || s2.Before(s1) || current.ValidateCurrent(current.Ref(), s2) != nil {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 S2 is not current")
	}
	authorizationS2, err := authorizer.authorizeV2(ctx, prepared, current, canonical, closure, s2)
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	if authorizationS2.ValidateAgainstV2(
		prepared,
		current,
		routeCallDigest,
		contextDigest,
		requestToolsDigest,
		s2,
	) != nil || !sameInvocationMaterialAuthorizationClosureV2(authorizationS1, authorizationS2) ||
		authorizationS2.AuthorizedUnixNano < authorizationS1.AuthorizedUnixNano ||
		authorizationS2.ExpiresUnixNano > authorizationS1.ExpiresUnixNano {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 authorization drifted between S1 and S2")
	}
	sealedAt := clock()
	if sealedAt.IsZero() || sealedAt.Before(s2) ||
		current.ValidateCurrent(current.Ref(), sealedAt) != nil ||
		!sealedAt.Before(time.Unix(0, authorizationS2.ExpiresUnixNano)) {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 seal crossed current TTL")
	}
	material, err := SealInvocationMaterialV2(InvocationMaterialV2{
		PreparedRef:          prepared.Ref(),
		UnifiedRequestDigest: prepared.UnifiedRequestDigest,
		PreparedPlanDigest:   prepared.PreparedPlanDigest,
		RouteDigest:          prepared.RouteDigest,
		ProfileDigest:        prepared.ProfileDigest,
		Authorization:        authorizationS2,
		Call:                 canonical,
		CreatedUnixNano:      sealedAt.UnixNano(),
		ExpiresUnixNano: minTimeUnixNanoMaterialV1(
			prepared.NotAfterUnixNano,
			current.ExpiresUnixNano,
			authorizationS2.ExpiresUnixNano,
		),
	})
	if err != nil {
		return InvocationMaterialV2{}, err
	}
	request := InvocationMaterialPersistRequestV2{material: material, authorization: authorizationS2}
	ensured, err := repository.EnsureAuthorizedInvocationMaterialV2(ctx, request)
	if err != nil {
		if GovernedModelInvocationErrorKindOfV1(err) != GovernedModelInvocationErrorIndeterminate {
			return InvocationMaterialV2{}, err
		}
		recovered, inspectErr := repository.InspectExactInvocationMaterialV2(
			context.WithoutCancel(ctx),
			material.RefV2(),
		)
		if inspectErr != nil {
			return InvocationMaterialV2{}, errors.Join(err, inspectErr)
		}
		ensured = recovered
	}
	if ensured.RefV2() != material.RefV2() || !reflect.DeepEqual(ensured, material) {
		return InvocationMaterialV2{}, governedConflictV1("invocation material V2 repository returned different canonical content")
	}
	return ensured.CloneV2(), nil
}

func invocationMaterialAuthorizationIdentityV2(
	prepared PreparedModelInvocationRefV1,
	current PreparedModelInvocationCurrentRefV1,
	route core.Digest,
) (string, error) {
	digest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material-authorization",
		"v2",
		"InvocationMaterialAuthorizationIdentityV2",
		struct {
			Prepared PreparedModelInvocationRefV1        `json:"prepared"`
			Current  PreparedModelInvocationCurrentRefV1 `json:"current"`
			Route    core.Digest                         `json:"route_call_digest"`
		}{prepared, current, route},
	)
	if err != nil {
		return "", err
	}
	return "invocation-material-authorization-v2/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func invocationMaterialAuthorizationDigestV2(a InvocationMaterialAuthorizationV2) (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material-authorization",
		"v2",
		"InvocationMaterialAuthorizationV2",
		a,
	)
}

func invocationMaterialIdentityV2(
	prepared PreparedModelInvocationRefV1,
	route core.Digest,
	authorizationID string,
) (string, error) {
	digest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material",
		"v2",
		"InvocationMaterialIdentityV2",
		struct {
			Prepared      PreparedModelInvocationRefV1 `json:"prepared"`
			Route         core.Digest                  `json:"route_call_digest"`
			Authorization string                       `json:"authorization_id"`
		}{prepared, route, authorizationID},
	)
	if err != nil {
		return "", err
	}
	return "invocation-material-v2/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func invocationMaterialDigestV2(m InvocationMaterialV2) (core.Digest, error) {
	m.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-material",
		"v2",
		"InvocationMaterialV2",
		m,
	)
}
