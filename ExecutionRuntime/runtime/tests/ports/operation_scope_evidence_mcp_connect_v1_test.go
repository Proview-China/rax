package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationScopeEvidenceMCPConnectV1ClosedMatrix(t *testing.T) {
	if err := ports.OperationScopeEvidenceMCPConnectMatrixV1().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{
		"action_profile": {OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceMCPConnectEffectKindV1, PolicyProfile: ports.OperationScopeEvidenceActionPolicyProfileV3},
		"tool_effect":    {OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, PolicyProfile: ports.OperationScopeEvidenceMCPConnectPolicyProfileV1},
		"admin":          {OperationKind: ports.OperationScopeAdminV3, EffectKind: ports.OperationScopeEvidenceMCPConnectEffectKindV1, PolicyProfile: ports.OperationScopeEvidenceMCPConnectPolicyProfileV1},
	} {
		t.Run(name, func(t *testing.T) {
			if key.Validate() == nil {
				t.Fatal("unsupported MCP Connect matrix was admitted")
			}
		})
	}
}

func TestOperationScopeEvidenceMCPConnectV1RequiresOnlyRunAndSession(t *testing.T) {
	values := mcpConnectApplicabilityV1()
	if err := ports.ValidateOperationScopeEvidenceMCPConnectApplicabilityV1(values); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]ports.OperationScopeEvidenceApplicabilityV3){
		"run_forbidden": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			v[2].Mode, v[2].Fact = ports.OperationScopeEvidenceForbiddenV3, nil
		},
		"session_kind": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			v[3].Fact.Kind = ports.OperationScopeEvidenceTurnCurrentKindV3
		},
		"turn_required": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			ref := connectFactV1(ports.OperationScopeEvidenceTurnCurrentKindV3, "turn")
			v[4].Mode, v[4].Fact = ports.OperationScopeEvidenceRequiredV3, &ref
		},
		"action_required": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			ref := connectFactV1(ports.OperationScopeEvidenceActionCandidateKindV3, "action")
			v[0].Mode, v[0].Fact = ports.OperationScopeEvidenceRequiredV3, &ref
		},
		"context_required": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			ref := connectFactV1(ports.OperationScopeEvidenceContextParentKindV3, "context")
			v[1].Mode, v[1].Fact = ports.OperationScopeEvidenceRequiredV3, &ref
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := mcpConnectApplicabilityV1()
			mutate(changed)
			if ports.ValidateOperationScopeEvidenceMCPConnectApplicabilityV1(changed) == nil {
				t.Fatal("invalid MCP Connect applicability was admitted")
			}
		})
	}
}

func mcpConnectApplicabilityV1() []ports.OperationScopeEvidenceApplicabilityV3 {
	run := connectFactV1(ports.OperationScopeEvidenceRunCurrentKindV3, "run")
	session := connectFactV1(ports.OperationScopeEvidenceSessionCurrentKindV3, "session")
	return ports.NormalizeOperationScopeEvidenceApplicabilityV3([]ports.OperationScopeEvidenceApplicabilityV3{
		{Dimension: ports.OperationScopeEvidenceRunV3, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &run},
		{Dimension: ports.OperationScopeEvidenceSessionV3, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &session},
		{Dimension: ports.OperationScopeEvidenceTurnV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceActionV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceContextV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
	})
}

func connectFactV1(kind ports.NamespacedNameV2, id string) ports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: kind, ID: "mcp-connect-" + id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
