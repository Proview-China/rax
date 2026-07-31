package modelinvoker

import (
	"context"
	"time"
)

// InvocationMaterialCurrentAuthorizationClosureV3 freezes the complete
// authoritative Context and Tool pair projections observed with one current
// material authorization check. Aggregate authorization expiry alone cannot
// prove that a non-minimum source did not drift between S1 and S2.
type InvocationMaterialCurrentAuthorizationClosureV3 struct {
	Authorization InvocationMaterialAuthorizationV2
	ContextPair   InvocationMaterialContextPairProjectionV2
	ToolPair      InvocationMaterialToolPairProjectionV2
}

// InspectCurrentInvocationMaterialAuthorizationV2 replays the five exact
// Model-owned reader checks for an already persisted InvocationMaterialV2.
// It does not create another material, cross a commit gate, prepare a Provider,
// or grant dispatch authority. Route gateways use it at S1/S2/S3 so the stored
// RouteCall bytes cannot outlive their authoritative Context and Tool pairs.
func InspectCurrentInvocationMaterialAuthorizationV2(
	ctx context.Context,
	authorizer *InvocationMaterialAuthorizerV2,
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	material InvocationMaterialV2,
	now time.Time,
) (InvocationMaterialAuthorizationV2, error) {
	if ctx == nil || ctx.Err() != nil || authorizer == nil || now.IsZero() ||
		material.ValidateAgainstPreparedV2(prepared, current, now) != nil {
		return InvocationMaterialAuthorizationV2{}, governedInvalidV1(
			"invocation material V2 current authorization inputs are invalid",
		)
	}
	closure := InvocationMaterialExactClosureV2{
		SourceLineage:     material.Authorization.SourceLineage,
		ProviderInjection: material.Authorization.ProviderInjectionRef,
		Route:             material.Authorization.RouteRef,
		Profile:           material.Authorization.ProfileRef,
	}
	observed, err := authorizer.authorizeV2(
		ctx,
		prepared,
		current,
		material.Call,
		closure,
		now,
	)
	if err != nil {
		return InvocationMaterialAuthorizationV2{}, err
	}
	if !sameInvocationMaterialAuthorizationClosureV2(
		observed,
		material.Authorization,
	) ||
		observed.AuthorizedUnixNano < material.Authorization.AuthorizedUnixNano ||
		observed.ExpiresUnixNano != material.Authorization.ExpiresUnixNano {
		return InvocationMaterialAuthorizationV2{}, governedConflictV1(
			"invocation material V2 current authorization drifted",
		)
	}
	return observed, nil
}

// InspectCurrentInvocationMaterialAuthorizationClosureV3 additionally freezes
// the full paired Context and Tool current projections. It performs no writes,
// gate crossing, Provider preparation, or dispatch authorization.
func InspectCurrentInvocationMaterialAuthorizationClosureV3(
	ctx context.Context,
	authorizer *InvocationMaterialAuthorizerV2,
	prepared PreparedModelInvocationFactV1,
	current PreparedModelInvocationCurrentProjectionV1,
	material InvocationMaterialV2,
	now time.Time,
) (InvocationMaterialCurrentAuthorizationClosureV3, error) {
	var closure InvocationMaterialCurrentAuthorizationClosureV3
	if ctx == nil || ctx.Err() != nil || authorizer == nil || now.IsZero() ||
		material.ValidateAgainstPreparedV2(prepared, current, now) != nil {
		return closure, governedInvalidV1(
			"invocation material V3 current authorization closure inputs are invalid",
		)
	}
	expected := InvocationMaterialExactClosureV2{
		SourceLineage:     material.Authorization.SourceLineage,
		ProviderInjection: material.Authorization.ProviderInjectionRef,
		Route:             material.Authorization.RouteRef,
		Profile:           material.Authorization.ProfileRef,
	}
	observed, err := authorizer.authorizeClosureV3(
		ctx,
		prepared,
		current,
		material.Call,
		expected,
		now,
	)
	if err != nil {
		return closure, err
	}
	authorization := observed.authorization
	if !sameInvocationMaterialAuthorizationClosureV2(
		authorization,
		material.Authorization,
	) ||
		authorization.AuthorizedUnixNano <
			material.Authorization.AuthorizedUnixNano ||
		authorization.ExpiresUnixNano !=
			material.Authorization.ExpiresUnixNano {
		return closure, governedConflictV1(
			"invocation material V2 current authorization drifted",
		)
	}
	closure = InvocationMaterialCurrentAuthorizationClosureV3{
		Authorization: authorization,
		ContextPair:   observed.contextPair,
		ToolPair:      observed.toolPair,
	}
	return closure, nil
}
