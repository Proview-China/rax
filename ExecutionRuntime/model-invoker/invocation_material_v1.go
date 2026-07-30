package modelinvoker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const InvocationMaterialContractVersionV1 = "praxis.model-invoker.invocation-material/v1"
const InvocationMaterialAuthorizationContractVersionV1 = "praxis.model-invoker.invocation-material-authorization/v1"

type InvocationMaterialExactSourceRefV1 struct {
	Owner    core.OwnerRef `json:"owner"`
	Kind     string        `json:"kind"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}
type InvocationMaterialAuthorizationRefV1 struct {
	ContractVersion string                              `json:"contract_version"`
	ID              string                              `json:"id"`
	Revision        core.Revision                       `json:"revision"`
	Digest          core.Digest                         `json:"digest"`
	PreparedRef     PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef      PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	RouteCallDigest core.Digest                         `json:"route_call_digest"`
}
type InvocationMaterialAuthorizationV1 struct {
	ContractVersion      string                              `json:"contract_version"`
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	Digest               core.Digest                         `json:"digest"`
	PreparedRef          PreparedModelInvocationRefV1        `json:"prepared_ref"`
	CurrentRef           PreparedModelInvocationCurrentRefV1 `json:"current_ref"`
	RouteCallDigest      core.Digest                         `json:"route_call_digest"`
	ContextFrameRef      InvocationMaterialExactSourceRefV1  `json:"context_frame_ref"`
	ToolSurfaceRef       InvocationMaterialExactSourceRefV1  `json:"tool_surface_ref"`
	ProviderInjectionRef InvocationMaterialExactSourceRefV1  `json:"provider_injection_ref"`
	RouteRef             InvocationMaterialExactSourceRefV1  `json:"route_ref"`
	ProfileRef           InvocationMaterialExactSourceRefV1  `json:"profile_ref"`
	AuthorizedUnixNano   int64                               `json:"authorized_unix_nano"`
	ExpiresUnixNano      int64                               `json:"expires_unix_nano"`
}

type InvocationMaterialExactClosureV1 struct {
	ContextFrame      InvocationMaterialExactSourceRefV1 `json:"context_frame"`
	ToolSurface       InvocationMaterialExactSourceRefV1 `json:"tool_surface"`
	ProviderInjection InvocationMaterialExactSourceRefV1 `json:"provider_injection"`
	Route             InvocationMaterialExactSourceRefV1 `json:"route"`
	Profile           InvocationMaterialExactSourceRefV1 `json:"profile"`
}

type InvocationMaterialExactSourceProjectionV1 struct {
	Ref             InvocationMaterialExactSourceRefV1 `json:"ref"`
	CheckedUnixNano int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano int64                              `json:"expires_unix_nano"`
}

func (p InvocationMaterialExactSourceProjectionV1) ValidateCurrentV1(expected InvocationMaterialExactSourceRefV1, now time.Time) error {
	if expected.Validate() != nil || p.Ref.Validate() != nil || p.Ref != expected || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return governedConflictV1("invocation material exact source is not current")
	}
	return nil
}

type InvocationMaterialContextFrameExactReaderV1 interface {
	InspectExactInvocationContextFrameV1(context.Context, InvocationMaterialExactSourceRefV1) (InvocationMaterialExactSourceProjectionV1, error)
}
type InvocationMaterialToolSurfaceExactReaderV1 interface {
	InspectExactInvocationToolSurfaceV1(context.Context, InvocationMaterialExactSourceRefV1) (InvocationMaterialExactSourceProjectionV1, error)
}
type InvocationMaterialProviderInjectionExactReaderV1 interface {
	InspectExactInvocationProviderInjectionV1(context.Context, InvocationMaterialExactSourceRefV1) (InvocationMaterialExactSourceProjectionV1, error)
}
type InvocationMaterialRouteExactReaderV1 interface {
	InspectExactInvocationRouteV1(context.Context, InvocationMaterialExactSourceRefV1) (InvocationMaterialExactSourceProjectionV1, error)
}
type InvocationMaterialProfileExactReaderV1 interface {
	InspectExactInvocationProfileV1(context.Context, InvocationMaterialExactSourceRefV1) (InvocationMaterialExactSourceProjectionV1, error)
}

type InvocationMaterialAuthorizerConfigV1 struct {
	ContextFrame      InvocationMaterialContextFrameExactReaderV1
	ToolSurface       InvocationMaterialToolSurfaceExactReaderV1
	ProviderInjection InvocationMaterialProviderInjectionExactReaderV1
	Route             InvocationMaterialRouteExactReaderV1
	Profile           InvocationMaterialProfileExactReaderV1
}

type InvocationMaterialAuthorizerV1 struct {
	config InvocationMaterialAuthorizerConfigV1
}

func NewInvocationMaterialAuthorizerV1(config InvocationMaterialAuthorizerConfigV1) (*InvocationMaterialAuthorizerV1, error) {
	if nilLikeInvocationMaterialV1(config.ContextFrame) || nilLikeInvocationMaterialV1(config.ToolSurface) || nilLikeInvocationMaterialV1(config.ProviderInjection) || nilLikeInvocationMaterialV1(config.Route) || nilLikeInvocationMaterialV1(config.Profile) {
		return nil, governedInvalidV1("invocation material Authorizer requires five exact owner readers")
	}
	return &InvocationMaterialAuthorizerV1{config: config}, nil
}

func (r InvocationMaterialExactSourceRefV1) Validate() error {
	if r.Owner.Validate() != nil || strings.TrimSpace(r.Kind) == "" || strings.TrimSpace(r.ID) == "" || r.Revision == 0 || r.Digest.Validate() != nil {
		return governedInvalidV1("invocation material exact source Ref is invalid")
	}
	return nil
}
func (c InvocationMaterialExactClosureV1) ValidateAgainstPreparedV1(prepared PreparedModelInvocationFactV1) error {
	for _, ref := range []InvocationMaterialExactSourceRefV1{c.ContextFrame, c.ToolSurface, c.ProviderInjection, c.Route, c.Profile} {
		if ref.Validate() != nil {
			return governedInvalidV1("invocation material exact closure Ref is invalid")
		}
	}
	if prepared.Validate() != nil ||
		c.ToolSurface.Digest != prepared.ActualToolSurfaceDigest ||
		c.ProviderInjection.Digest != prepared.ActualProviderInjectionDigest ||
		c.Route.Digest != prepared.RouteDigest ||
		c.Profile.Digest != prepared.ProfileDigest {
		return governedConflictV1("invocation material exact closure differs from Prepared")
	}
	return nil
}

func (a *InvocationMaterialAuthorizerV1) authorizeV1(ctx context.Context, prepared PreparedModelInvocationFactV1, current PreparedModelInvocationCurrentProjectionV1, call RouteCall, expected InvocationMaterialExactClosureV1, now time.Time) (InvocationMaterialAuthorizationV1, error) {
	if a == nil || nilLikeInvocationMaterialV1(a.config.ContextFrame) || nilLikeInvocationMaterialV1(a.config.ToolSurface) || nilLikeInvocationMaterialV1(a.config.ProviderInjection) || nilLikeInvocationMaterialV1(a.config.Route) || nilLikeInvocationMaterialV1(a.config.Profile) || ctx == nil || ctx.Err() != nil || now.IsZero() || expected.ValidateAgainstPreparedV1(prepared) != nil || current.ValidateAgainstFact(prepared) != nil || current.ValidateCurrent(current.Ref(), now) != nil {
		return InvocationMaterialAuthorizationV1{}, governedInvalidV1("invocation material Authorizer input is invalid")
	}
	contextFrame, err := a.config.ContextFrame.InspectExactInvocationContextFrameV1(ctx, expected.ContextFrame)
	if err != nil || contextFrame.ValidateCurrentV1(expected.ContextFrame, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material ContextFrame source is not current")
		}
		return InvocationMaterialAuthorizationV1{}, err
	}
	toolSurface, err := a.config.ToolSurface.InspectExactInvocationToolSurfaceV1(ctx, expected.ToolSurface)
	if err != nil || toolSurface.ValidateCurrentV1(expected.ToolSurface, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material ToolSurface source is not current")
		}
		return InvocationMaterialAuthorizationV1{}, err
	}
	providerInjection, err := a.config.ProviderInjection.InspectExactInvocationProviderInjectionV1(ctx, expected.ProviderInjection)
	if err != nil || providerInjection.ValidateCurrentV1(expected.ProviderInjection, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material ProviderInjection source is not current")
		}
		return InvocationMaterialAuthorizationV1{}, err
	}
	route, err := a.config.Route.InspectExactInvocationRouteV1(ctx, expected.Route)
	if err != nil || route.ValidateCurrentV1(expected.Route, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material Route source is not current")
		}
		return InvocationMaterialAuthorizationV1{}, err
	}
	profile, err := a.config.Profile.InspectExactInvocationProfileV1(ctx, expected.Profile)
	if err != nil || profile.ValidateCurrentV1(expected.Profile, now) != nil {
		if err == nil {
			err = governedConflictV1("invocation material Profile source is not current")
		}
		return InvocationMaterialAuthorizationV1{}, err
	}
	observed := InvocationMaterialExactClosureV1{ContextFrame: contextFrame.Ref, ToolSurface: toolSurface.Ref, ProviderInjection: providerInjection.Ref, Route: route.Ref, Profile: profile.Ref}
	if observed != expected || observed.ValidateAgainstPreparedV1(prepared) != nil {
		return InvocationMaterialAuthorizationV1{}, governedConflictV1("invocation material exact owner reader drifted")
	}
	routeCallDigest, err := DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		return InvocationMaterialAuthorizationV1{}, err
	}
	return SealInvocationMaterialAuthorizationV1(InvocationMaterialAuthorizationV1{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), RouteCallDigest: routeCallDigest,
		ContextFrameRef: contextFrame.Ref, ToolSurfaceRef: toolSurface.Ref, ProviderInjectionRef: providerInjection.Ref, RouteRef: route.Ref, ProfileRef: profile.Ref,
		AuthorizedUnixNano: now.UnixNano(),
		ExpiresUnixNano: minTimeUnixNanoMaterialV1(
			prepared.NotAfterUnixNano,
			current.ExpiresUnixNano,
			contextFrame.ExpiresUnixNano,
			toolSurface.ExpiresUnixNano,
			providerInjection.ExpiresUnixNano,
			route.ExpiresUnixNano,
			profile.ExpiresUnixNano,
		),
	})
}

func nilLikeInvocationMaterialV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
func (a InvocationMaterialAuthorizationV1) RefV1() InvocationMaterialAuthorizationRefV1 {
	return InvocationMaterialAuthorizationRefV1{ContractVersion: a.ContractVersion, ID: a.ID, Revision: a.Revision, Digest: a.Digest, PreparedRef: a.PreparedRef, CurrentRef: a.CurrentRef, RouteCallDigest: a.RouteCallDigest}
}
func (r InvocationMaterialAuthorizationRefV1) Validate() error {
	if r.ContractVersion != InvocationMaterialAuthorizationContractVersionV1 || strings.TrimSpace(r.ID) == "" || r.Revision != 1 || r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil || r.CurrentRef.Validate() != nil || r.CurrentRef.Prepared != r.PreparedRef || r.RouteCallDigest.Validate() != nil {
		return governedInvalidV1("invocation material authorization Ref is invalid")
	}
	return nil
}
func (a InvocationMaterialAuthorizationV1) Validate() error {
	if a.RefV1().Validate() != nil || a.AuthorizedUnixNano <= 0 || a.ExpiresUnixNano <= a.AuthorizedUnixNano {
		return governedInvalidV1("invocation material authorization is invalid")
	}
	for _, ref := range []InvocationMaterialExactSourceRefV1{a.ContextFrameRef, a.ToolSurfaceRef, a.ProviderInjectionRef, a.RouteRef, a.ProfileRef} {
		if ref.Validate() != nil {
			return governedInvalidV1("invocation material authorization source Ref is invalid")
		}
	}
	expectedID, err := invocationMaterialAuthorizationIdentityV1(a.PreparedRef, a.CurrentRef, a.RouteCallDigest)
	if err != nil || expectedID != a.ID {
		return governedConflictV1("invocation material authorization identity drifted")
	}
	expected, err := invocationMaterialAuthorizationDigestV1(a)
	if err != nil || expected != a.Digest {
		return governedConflictV1("invocation material authorization digest drifted")
	}
	return nil
}
func (a InvocationMaterialAuthorizationV1) ValidateAgainstV1(prepared PreparedModelInvocationFactV1, current PreparedModelInvocationCurrentProjectionV1, routeCallDigest core.Digest, now time.Time) error {
	if a.Validate() != nil || prepared.Validate() != nil || current.ValidateAgainstFact(prepared) != nil || current.ValidateCurrent(current.Ref(), now) != nil || now.IsZero() || !now.Before(time.Unix(0, a.ExpiresUnixNano)) || a.ExpiresUnixNano > current.ExpiresUnixNano || a.ExpiresUnixNano > prepared.NotAfterUnixNano || a.PreparedRef != prepared.Ref() || a.CurrentRef != current.Ref() || a.RouteCallDigest != routeCallDigest || a.ToolSurfaceRef.Digest != prepared.ActualToolSurfaceDigest || a.ProviderInjectionRef.Digest != prepared.ActualProviderInjectionDigest || a.RouteRef.Digest != prepared.RouteDigest || a.ProfileRef.Digest != prepared.ProfileDigest {
		return governedConflictV1("invocation material authorization differs from exact Prepared/current sources")
	}
	return nil
}
func SealInvocationMaterialAuthorizationV1(a InvocationMaterialAuthorizationV1) (InvocationMaterialAuthorizationV1, error) {
	a.ContractVersion = InvocationMaterialAuthorizationContractVersionV1
	a.Revision = 1
	id, err := invocationMaterialAuthorizationIdentityV1(a.PreparedRef, a.CurrentRef, a.RouteCallDigest)
	if err != nil {
		return InvocationMaterialAuthorizationV1{}, err
	}
	if a.ID != "" && a.ID != id {
		return InvocationMaterialAuthorizationV1{}, governedConflictV1("invocation material authorization ID drifted")
	}
	a.ID = id
	supplied := a.Digest
	a.Digest = ""
	a.Digest, err = invocationMaterialAuthorizationDigestV1(a)
	if err != nil {
		return InvocationMaterialAuthorizationV1{}, err
	}
	if supplied != "" && supplied != a.Digest {
		return InvocationMaterialAuthorizationV1{}, governedConflictV1("invocation material authorization digest drifted")
	}
	if err := a.Validate(); err != nil {
		return InvocationMaterialAuthorizationV1{}, err
	}
	return a, nil
}

type InvocationMaterialRefV1 struct {
	ContractVersion       string                               `json:"contract_version"`
	ID                    string                               `json:"id"`
	Revision              core.Revision                        `json:"revision"`
	Digest                core.Digest                          `json:"digest"`
	PreparedRef           PreparedModelInvocationRefV1         `json:"prepared_ref"`
	RouteCallDigest       core.Digest                          `json:"route_call_digest"`
	UnifiedRequestDigest  core.Digest                          `json:"unified_request_digest"`
	PreparedPlanDigest    core.Digest                          `json:"prepared_plan_digest"`
	RouteDigest           core.Digest                          `json:"route_digest"`
	ProfileDigest         core.Digest                          `json:"profile_digest"`
	ModelDigest           core.Digest                          `json:"model_digest"`
	BudgetDigest          core.Digest                          `json:"budget_digest"`
	ToolChoiceDigest      core.Digest                          `json:"tool_choice_digest"`
	ConsumerBindingDigest core.Digest                          `json:"consumer_binding_digest"`
	AuthorizationRef      InvocationMaterialAuthorizationRefV1 `json:"authorization_ref"`
	ExpiresUnixNano       int64                                `json:"expires_unix_nano"`
}

type InvocationMaterialV1 struct {
	ContractVersion       string                            `json:"contract_version"`
	ID                    string                            `json:"id"`
	Revision              core.Revision                     `json:"revision"`
	Digest                core.Digest                       `json:"digest"`
	PreparedRef           PreparedModelInvocationRefV1      `json:"prepared_ref"`
	RouteCallDigest       core.Digest                       `json:"route_call_digest"`
	UnifiedRequestDigest  core.Digest                       `json:"unified_request_digest"`
	PreparedPlanDigest    core.Digest                       `json:"prepared_plan_digest"`
	RouteDigest           core.Digest                       `json:"route_digest"`
	ProfileDigest         core.Digest                       `json:"profile_digest"`
	ModelDigest           core.Digest                       `json:"model_digest"`
	BudgetDigest          core.Digest                       `json:"budget_digest"`
	ToolChoiceDigest      core.Digest                       `json:"tool_choice_digest"`
	ConsumerBindingDigest core.Digest                       `json:"consumer_binding_digest"`
	Authorization         InvocationMaterialAuthorizationV1 `json:"authorization"`
	Call                  RouteCall                         `json:"call"`
	CreatedUnixNano       int64                             `json:"created_unix_nano"`
	ExpiresUnixNano       int64                             `json:"expires_unix_nano"`
}

type InvocationMaterialRepositoryV1 interface {
	InvocationMaterialReaderV1
	EnsureAuthorizedInvocationMaterialV1(context.Context, InvocationMaterialPersistRequestV1) (InvocationMaterialV1, error)
}
type InvocationMaterialReaderV1 interface {
	InspectExactInvocationMaterialV1(context.Context, InvocationMaterialRefV1) (InvocationMaterialV1, error)
}

// InvocationMaterialPersistRequestV1 is an opaque Model Owner write token.
// Callers cannot construct a valid value or persist a raw material directly;
// AuthorizeAndEnsureInvocationMaterialV1 is the sole constructor.
type InvocationMaterialPersistRequestV1 struct {
	material      InvocationMaterialV1
	authorization InvocationMaterialAuthorizationV1
}

func (r InvocationMaterialPersistRequestV1) ValidateV1() error {
	if err := r.material.Validate(); err != nil {
		return err
	}
	if err := r.authorization.Validate(); err != nil || r.material.Authorization != r.authorization {
		return governedConflictV1("invocation material persist request lacks the exact owner authorization")
	}
	return nil
}

func (r InvocationMaterialPersistRequestV1) MaterialV1() InvocationMaterialV1 {
	return r.material.CloneV1()
}

func (m InvocationMaterialV1) RefV1() InvocationMaterialRefV1 {
	return InvocationMaterialRefV1{ContractVersion: m.ContractVersion, ID: m.ID, Revision: m.Revision, Digest: m.Digest, PreparedRef: m.PreparedRef, RouteCallDigest: m.RouteCallDigest, UnifiedRequestDigest: m.UnifiedRequestDigest, PreparedPlanDigest: m.PreparedPlanDigest, RouteDigest: m.RouteDigest, ProfileDigest: m.ProfileDigest, ModelDigest: m.ModelDigest, BudgetDigest: m.BudgetDigest, ToolChoiceDigest: m.ToolChoiceDigest, ConsumerBindingDigest: m.ConsumerBindingDigest, AuthorizationRef: m.Authorization.RefV1(), ExpiresUnixNano: m.ExpiresUnixNano}
}
func (m InvocationMaterialV1) CloneV1() InvocationMaterialV1 {
	payload, _ := json.Marshal(m)
	var clone InvocationMaterialV1
	_ = core.DecodeStrictJSON(payload, &clone)
	if string(clone.Call.Request.Output.Schema) == "null" {
		clone.Call.Request.Output.Schema = nil
	}
	return clone
}
func EncodeInvocationMaterialV1(m InvocationMaterialV1) (json.RawMessage, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}
func DecodeInvocationMaterialV1(payload json.RawMessage) (InvocationMaterialV1, error) {
	var material InvocationMaterialV1
	if err := core.DecodeStrictJSON(payload, &material); err != nil {
		return InvocationMaterialV1{}, err
	}
	if string(material.Call.Request.Output.Schema) == "null" {
		material.Call.Request.Output.Schema = nil
	}
	return material.CloneV1(), material.Validate()
}
func (r InvocationMaterialRefV1) Validate() error {
	if r.ContractVersion != InvocationMaterialContractVersionV1 || strings.TrimSpace(r.ID) == "" || r.Revision != 1 || r.Digest.Validate() != nil || r.PreparedRef.Validate() != nil || r.ExpiresUnixNano <= 0 {
		return governedInvalidV1("invocation material exact Ref is invalid")
	}
	for _, digest := range []core.Digest{r.RouteCallDigest, r.UnifiedRequestDigest, r.PreparedPlanDigest, r.RouteDigest, r.ProfileDigest, r.ModelDigest, r.BudgetDigest, r.ToolChoiceDigest, r.ConsumerBindingDigest} {
		if digest.Validate() != nil {
			return governedInvalidV1("invocation material exact Ref digest is invalid")
		}
	}
	if err := r.AuthorizationRef.Validate(); err != nil {
		return err
	}
	id, err := invocationMaterialIdentityV1(r.PreparedRef, r.RouteCallDigest, r.AuthorizationRef.Digest)
	if err != nil || id != r.ID {
		return governedConflictV1("invocation material exact Ref identity drifted")
	}
	return nil
}
func (m InvocationMaterialV1) Validate() error {
	if m.ContractVersion != InvocationMaterialContractVersionV1 || m.Revision != 1 || m.CreatedUnixNano <= 0 || m.ExpiresUnixNano <= m.CreatedUnixNano || m.ExpiresUnixNano > m.Authorization.ExpiresUnixNano {
		return governedInvalidV1("invocation material fields are invalid")
	}
	if err := m.RefV1().Validate(); err != nil {
		return err
	}
	if err := m.Authorization.Validate(); err != nil || m.Authorization.RefV1() != m.RefV1().AuthorizationRef {
		return governedConflictV1("invocation material authorization drifted")
	}
	digest, err := DigestGovernedModelTurnRouteCallV2(m.Call)
	if err != nil || digest != m.RouteCallDigest {
		return governedConflictV1("invocation material RouteCall drifted")
	}
	model, budget, choice, consumer, err := invocationMaterialComponentDigestsV1(m.Call)
	if err != nil || model != m.ModelDigest || budget != m.BudgetDigest || choice != m.ToolChoiceDigest || consumer != m.ConsumerBindingDigest {
		return governedConflictV1("invocation material component digest drifted")
	}
	if m.UnifiedRequestDigest != m.PreparedRef.UnifiedRequestDigest {
		return governedConflictV1("invocation material request digest drifted")
	}
	expected, err := invocationMaterialDigestV1(m)
	if err != nil || expected != m.Digest {
		return governedConflictV1("invocation material digest drifted")
	}
	return nil
}
func (m InvocationMaterialV1) ValidateAgainstPreparedV1(prepared PreparedModelInvocationFactV1, current PreparedModelInvocationCurrentProjectionV1, now time.Time) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := prepared.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, m.ExpiresUnixNano)) || current.ValidateAgainstFact(prepared) != nil || m.PreparedRef != prepared.Ref() || m.UnifiedRequestDigest != prepared.UnifiedRequestDigest || m.PreparedPlanDigest != prepared.PreparedPlanDigest || m.RouteDigest != prepared.RouteDigest || m.ProfileDigest != prepared.ProfileDigest || m.ExpiresUnixNano > prepared.NotAfterUnixNano || m.Authorization.ValidateAgainstV1(prepared, current, m.RouteCallDigest, now) != nil {
		return governedConflictV1("invocation material differs from exact durable Prepared fact")
	}
	return nil
}
func SealInvocationMaterialV1(m InvocationMaterialV1) (InvocationMaterialV1, error) {
	if m.ContractVersion != "" && m.ContractVersion != InvocationMaterialContractVersionV1 {
		return InvocationMaterialV1{}, governedInvalidV1("invocation material version is invalid")
	}
	m.ContractVersion = InvocationMaterialContractVersionV1
	m.Revision = 1
	canonicalCall, err := canonicalInvocationMaterialCallV1(m.Call)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	m.Call = canonicalCall
	routeDigest, err := DigestGovernedModelTurnRouteCallV2(m.Call)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	if m.RouteCallDigest != "" && m.RouteCallDigest != routeDigest {
		return InvocationMaterialV1{}, governedConflictV1("invocation material RouteCall digest drifted")
	}
	m.RouteCallDigest = routeDigest
	if err := m.Authorization.Validate(); err != nil || m.Authorization.PreparedRef != m.PreparedRef || m.Authorization.RouteCallDigest != routeDigest {
		return InvocationMaterialV1{}, governedConflictV1("invocation material lacks exact owner authorization")
	}
	id, err := invocationMaterialIdentityV1(m.PreparedRef, routeDigest, m.Authorization.Digest)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	if m.ID != "" && m.ID != id {
		return InvocationMaterialV1{}, governedConflictV1("invocation material ID drifted")
	}
	m.ID = id
	supplied := m.Digest
	m.Digest = ""
	m = m.CloneV1()
	m.Digest, err = invocationMaterialDigestV1(m)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	if supplied != "" && supplied != m.Digest {
		return InvocationMaterialV1{}, governedConflictV1("invocation material digest drifted")
	}
	if err := m.Validate(); err != nil {
		return InvocationMaterialV1{}, err
	}
	return m, nil
}
func AuthorizeAndEnsureInvocationMaterialV1(ctx context.Context, authorizer *InvocationMaterialAuthorizerV1, repository InvocationMaterialRepositoryV1, prepared PreparedModelInvocationFactV1, current PreparedModelInvocationCurrentProjectionV1, call RouteCall, closure InvocationMaterialExactClosureV1, clock func() time.Time) (InvocationMaterialV1, error) {
	if ctx == nil || ctx.Err() != nil || authorizer == nil || repository == nil || clock == nil || prepared.Validate() != nil || current.ValidateAgainstFact(prepared) != nil || closure.ValidateAgainstPreparedV1(prepared) != nil {
		return InvocationMaterialV1{}, governedInvalidV1("invocation material clock or Prepared fact is invalid")
	}
	var err error
	call, err = canonicalInvocationMaterialCallV1(call)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	s1 := clock()
	if s1.IsZero() || current.ValidateCurrent(current.Ref(), s1) != nil {
		return InvocationMaterialV1{}, governedConflictV1("invocation material S1 is not current")
	}
	authorizationS1, err := authorizer.authorizeV1(ctx, prepared.Clone(), current.Clone(), call, closure, s1)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	routeCallDigest, err := DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	if err := authorizationS1.ValidateAgainstV1(prepared, current, routeCallDigest, s1); err != nil {
		return InvocationMaterialV1{}, &GovernedModelInvocationErrorV1{
			Kind:      GovernedModelInvocationErrorConflict,
			Operation: "new_invocation_material",
			Message:   "invocation material owner S1 authorization is not exact",
			Err:       err,
		}
	}
	s2 := clock()
	if s2.IsZero() || s2.Before(s1) || current.ValidateCurrent(current.Ref(), s2) != nil {
		return InvocationMaterialV1{}, governedConflictV1("invocation material S2 is not current")
	}
	authorizationS2, err := authorizer.authorizeV1(ctx, prepared.Clone(), current.Clone(), call, closure, s2)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	if err := authorizationS2.ValidateAgainstV1(prepared, current, routeCallDigest, s2); err != nil {
		return InvocationMaterialV1{}, &GovernedModelInvocationErrorV1{Kind: GovernedModelInvocationErrorConflict, Operation: "new_invocation_material", Message: "invocation material owner S2 authorization is not exact", Err: err}
	}
	if !sameInvocationMaterialAuthorizationClosureV1(authorizationS1, authorizationS2) || authorizationS2.AuthorizedUnixNano < authorizationS1.AuthorizedUnixNano || authorizationS2.ExpiresUnixNano > authorizationS1.ExpiresUnixNano {
		return InvocationMaterialV1{}, governedConflictV1("invocation material exact authorization drifted between S1 and S2")
	}
	sealedAt := clock()
	if sealedAt.IsZero() || sealedAt.Before(s2) || current.ValidateCurrent(current.Ref(), sealedAt) != nil || !sealedAt.Before(time.Unix(0, authorizationS2.ExpiresUnixNano)) {
		return InvocationMaterialV1{}, governedConflictV1("invocation material owner seal crossed current TTL")
	}
	model, budget, choice, consumer, err := invocationMaterialComponentDigestsV1(call)
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	material, err := SealInvocationMaterialV1(InvocationMaterialV1{PreparedRef: prepared.Ref(), Authorization: authorizationS2, Call: call, UnifiedRequestDigest: prepared.UnifiedRequestDigest, PreparedPlanDigest: prepared.PreparedPlanDigest, RouteDigest: prepared.RouteDigest, ProfileDigest: prepared.ProfileDigest, ModelDigest: model, BudgetDigest: budget, ToolChoiceDigest: choice, ConsumerBindingDigest: consumer, CreatedUnixNano: sealedAt.UnixNano(), ExpiresUnixNano: minTimeUnixNanoMaterialV1(prepared.NotAfterUnixNano, current.ExpiresUnixNano, authorizationS2.ExpiresUnixNano)})
	if err != nil {
		return InvocationMaterialV1{}, err
	}
	request := InvocationMaterialPersistRequestV1{material: material, authorization: authorizationS2}
	ensured, err := repository.EnsureAuthorizedInvocationMaterialV1(ctx, request)
	if err != nil {
		if GovernedModelInvocationErrorKindOfV1(err) != GovernedModelInvocationErrorIndeterminate {
			return InvocationMaterialV1{}, err
		}
		recovered, inspectErr := repository.InspectExactInvocationMaterialV1(context.WithoutCancel(ctx), material.RefV1())
		if inspectErr != nil {
			return InvocationMaterialV1{}, errors.Join(err, inspectErr)
		}
		ensured = recovered
	}
	if ensured.RefV1() != material.RefV1() || !reflect.DeepEqual(ensured, material) {
		return InvocationMaterialV1{}, governedConflictV1("invocation material repository returned different canonical content")
	}
	return ensured.CloneV1(), nil
}

func sameInvocationMaterialAuthorizationClosureV1(left, right InvocationMaterialAuthorizationV1) bool {
	return left.PreparedRef == right.PreparedRef &&
		left.CurrentRef == right.CurrentRef &&
		left.RouteCallDigest == right.RouteCallDigest &&
		left.ContextFrameRef == right.ContextFrameRef &&
		left.ToolSurfaceRef == right.ToolSurfaceRef &&
		left.ProviderInjectionRef == right.ProviderInjectionRef &&
		left.RouteRef == right.RouteRef &&
		left.ProfileRef == right.ProfileRef
}
func DigestGovernedModelTurnRouteCallV2(call RouteCall) (core.Digest, error) {
	if strings.TrimSpace(string(call.RouteID)) == "" || call.Invocation == (upstream.InvocationContext{}) || call.EntitlementState != nil || strings.TrimSpace(call.Request.Model) == "" || len(call.Request.Input) == 0 || call.Request.Stream || call.Request.Provider != "" || call.Request.Protocol != ProtocolAuto || strings.TrimSpace(call.Request.Endpoint) != "" || call.Request.State != nil || len(call.Request.ProviderOptions) != 0 {
		return "", governedInvalidV1("governed model turn RouteCall must be synchronous, RouteID-owned, durable and continuation-free")
	}
	if len(call.Request.Tools) == 0 || call.Request.ParallelToolCalls == nil || *call.Request.ParallelToolCalls || call.Request.ToolChoice.Mode == ToolChoiceNone {
		return "", governedInvalidV1("governed model turn requires tools and explicit non-parallel execution")
	}
	names := map[string]struct{}{}
	for _, tool := range call.Request.Tools {
		if strings.TrimSpace(tool.Name) == "" || tool.Strict == nil || !*tool.Strict {
			return "", governedInvalidV1("governed model turn requires strict named tools")
		}
		if _, ok := names[tool.Name]; ok {
			return "", governedConflictV1("governed model turn tool names must be unique")
		}
		names[tool.Name] = struct{}{}
		if err := validateStrictJSONObjectV1(tool.Parameters); err != nil {
			return "", err
		}
	}
	if call.Request.ToolChoice.Mode == ToolChoiceFunction {
		if _, ok := names[call.Request.ToolChoice.Name]; !ok {
			return "", governedInvalidV1("governed model turn selected tool is absent")
		}
	}
	if call.Request.Output.Type != OutputText || len(call.Request.Output.Schema) != 0 || call.Request.Output.Strict != nil {
		return "", governedInvalidV1("governed model turn tool-call slice requires default text output constraint")
	}
	return core.CanonicalJSONDigest("praxis.model-invoker.governed-model-turn", "v2", "GovernedModelTurnRouteCallV2", call)
}
func invocationMaterialIdentityV1(prepared PreparedModelInvocationRefV1, route, authorization core.Digest) (string, error) {
	d, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "InvocationMaterialIdentityV1", struct {
		Prepared      PreparedModelInvocationRefV1 `json:"prepared"`
		Route         core.Digest                  `json:"route_call_digest"`
		Authorization core.Digest                  `json:"authorization_digest"`
	}{prepared, route, authorization})
	if err != nil {
		return "", err
	}
	return "invocation-material/" + strings.TrimPrefix(string(d), "sha256:"), nil
}
func invocationMaterialAuthorizationIdentityV1(prepared PreparedModelInvocationRefV1, current PreparedModelInvocationCurrentRefV1, route core.Digest) (string, error) {
	d, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material-authorization", "v1", "InvocationMaterialAuthorizationIdentityV1", struct {
		Prepared PreparedModelInvocationRefV1        `json:"prepared"`
		Current  PreparedModelInvocationCurrentRefV1 `json:"current"`
		Route    core.Digest                         `json:"route_call_digest"`
	}{prepared, current, route})
	if err != nil {
		return "", err
	}
	return "invocation-material-authorization/" + strings.TrimPrefix(string(d), "sha256:"), nil
}
func invocationMaterialAuthorizationDigestV1(a InvocationMaterialAuthorizationV1) (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.invocation-material-authorization", "v1", "InvocationMaterialAuthorizationV1", a)
}
func invocationMaterialDigestV1(m InvocationMaterialV1) (core.Digest, error) {
	m.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "InvocationMaterialV1", m)
}
func minTimeUnixNanoMaterialV1(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}
func invocationMaterialComponentDigestsV1(call RouteCall) (core.Digest, core.Digest, core.Digest, core.Digest, error) {
	model, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "ModelBindingV1", struct {
		Model string `json:"model"`
	}{call.Request.Model})
	if err != nil {
		return "", "", "", "", err
	}
	budget, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "BudgetBindingV1", call.Request.Budget)
	if err != nil {
		return "", "", "", "", err
	}
	choice, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "ToolChoiceBindingV1", call.Request.ToolChoice)
	if err != nil {
		return "", "", "", "", err
	}
	consumer, err := core.CanonicalJSONDigest("praxis.model-invoker.invocation-material", "v1", "ConsumerBindingV1", call.Invocation)
	return model, budget, choice, consumer, err
}
func canonicalInvocationMaterialCallV1(call RouteCall) (RouteCall, error) {
	payload, err := json.Marshal(call)
	if err != nil {
		return RouteCall{}, err
	}
	var canonical RouteCall
	if err := core.DecodeStrictJSON(payload, &canonical); err != nil {
		return RouteCall{}, err
	}
	if string(canonical.Request.Output.Schema) == "null" {
		canonical.Request.Output.Schema = nil
	}
	return canonical, nil
}
