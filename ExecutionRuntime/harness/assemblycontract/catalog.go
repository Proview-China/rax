package assemblycontract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func catalogSchema(name string) runtimeports.SchemaRefV2 {
	canonical := strings.NewReplacer(".", "-", "*", "wildcard", "/", "-").Replace(name)
	return runtimeports.SchemaRefV2{
		Namespace:     "praxis.harness.assembly",
		Name:          canonical,
		Version:       "1.0.0",
		MediaType:     "application/json",
		ContentDigest: core.DigestBytes([]byte("praxis.harness.assembly.schema/v1:" + name)),
	}
}

func slotSpec(id string, scope LifecycleScopeV1, cardinality CardinalityV1, required bool, owner runtimeports.CapabilityNameV2, kinds ...SlotContributionKindV1) SlotSpecV1 {
	value := SlotSpecV1{
		ContractVersion:   ContractVersionV1,
		SlotID:            id,
		LifecycleScope:    scope,
		Cardinality:       cardinality,
		Required:          required,
		OwnerCapability:   owner,
		ContributionKinds: kinds,
		InputSchema:       catalogSchema("slot-" + id + "-input"),
		OutputSchema:      catalogSchema("slot-" + id + "-output"),
		EffectClass:       "declared-by-owner-port",
		ConcurrencyPolicy: "compiled-deterministic",
		FailurePolicy:     "fail-closed-if-required",
		DegradationPolicy: "explicit-residual-only",
	}
	digest, _ := SlotSpecDigestV1(value)
	value.Digest = digest
	return value
}

// SlotCatalogV1 returns a fresh copy of the Harness-owned public catalog. Only
// the five Wave 1 core slots are required; the remaining entries freeze names
// and contribution ceilings but do not make their runtime adapters available.
func SlotCatalogV1() []SlotSpecV1 {
	return []SlotSpecV1{
		slotSpec("kernel.loop", LifecycleRunV1, CardinalityExactlyOneV1, true, "praxis.harness/kernel", SlotContributionOwnerV1, SlotContributionReferenceV1),
		slotSpec("model.turn", LifecycleGenerationV1, CardinalityActiveBindingV1, true, "praxis.model-invoker/route", SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("context.frame", LifecycleRunV1, CardinalityOwnerSourcesV1, true, "praxis.context-cache/frame", SlotContributionOwnerV1, SlotContributionSourceV1, SlotContributionReferenceV1),
		slotSpec("event.candidate", LifecycleSessionV1, CardinalityExactlyOneV1, true, "praxis.harness/event-candidate", SlotContributionSourceV1, SlotContributionReferenceV1),
		slotSpec("action.router", LifecycleRunV1, CardinalityOwnerSourcesV1, false, "praxis.tool-mcp/action-router", SlotContributionOwnerV1, SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("tool.provider.*", LifecycleGenerationV1, CardinalityZeroOrManyV1, false, "praxis.tool-mcp/tool-provider", SlotContributionProviderV1),
		slotSpec("mcp.provider.*", LifecycleGenerationV1, CardinalityZeroOrManyV1, false, "praxis.tool-mcp/mcp-provider", SlotContributionProviderV1),
		slotSpec("review.gate", LifecycleRunV1, CardinalityOwnerSourcesV1, false, "praxis.review/gate", SlotContributionOwnerV1, SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("sandbox.execution", LifecycleInstanceV1, CardinalityActiveBindingV1, false, "praxis.sandbox/execution", SlotContributionOwnerV1, SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("continuity.timeline", LifecycleRunV1, CardinalityOwnerSourcesV1, false, "praxis.continuity/timeline", SlotContributionOwnerV1, SlotContributionSourceV1, SlotContributionReferenceV1),
		slotSpec("memory.state", LifecycleRunV1, CardinalityZeroOrManyV1, false, "praxis.memory/state", SlotContributionOwnerV1, SlotContributionSourceV1, SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("knowledge.query", LifecycleRunV1, CardinalityZeroOrManyV1, false, "praxis.knowledge/query", SlotContributionOwnerV1, SlotContributionSourceV1, SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("asset.store", LifecycleRunV1, CardinalityZeroOrManyV1, false, "praxis.asset/store", SlotContributionProviderV1, SlotContributionReferenceV1),
		slotSpec("identity.authority", LifecycleRunV1, CardinalityExactlyOneV1, false, "praxis.runtime/authority", SlotContributionReferenceV1),
		slotSpec("organization.policy", LifecycleRunV1, CardinalityExactlyOneV1, false, "praxis.organization/policy", SlotContributionReferenceV1),
		slotSpec("budget.policy", LifecycleRunV1, CardinalityExactlyOneV1, false, "praxis.runtime/budget", SlotContributionReferenceV1),
		slotSpec("evidence.sink", LifecycleSessionV1, CardinalityExactlyOneV1, false, "praxis.runtime/evidence", SlotContributionSourceV1, SlotContributionReferenceV1),
		slotSpec("runtime.gateway", LifecycleInstanceV1, CardinalityExactlyOneV1, true, "praxis.runtime/gateway", SlotContributionReferenceV1),
		slotSpec("management.control", LifecycleRunV1, CardinalityZeroOrOneV1, false, "praxis.application/management", SlotContributionReferenceV1),
		slotSpec("domain.*", LifecycleRunV1, CardinalityZeroOrManyV1, false, "praxis.domain/extension", SlotContributionOwnerV1, SlotContributionSourceV1, SlotContributionProviderV1, SlotContributionReferenceV1),
	}
}

type phaseCatalogEntryV1 struct {
	phase string
	kinds []PhaseCapabilityV1
}

var phaseCatalogEntriesV1 = []phaseCatalogEntryV1{
	{"assembly.graph.compile.before", []PhaseCapabilityV1{PhaseFilterV1}}, {"assembly.graph.compile.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"assembly.binding.validate", []PhaseCapabilityV1{PhaseFilterV1}},
	{"assembly.preflight.before", []PhaseCapabilityV1{PhaseFilterV1}}, {"assembly.preflight.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"endpoint.open.before", []PhaseCapabilityV1{PhaseGateV1}}, {"endpoint.open.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"session.start.before", []PhaseCapabilityV1{PhaseFilterV1}}, {"session.start.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"run.start.before", []PhaseCapabilityV1{PhaseGateV1}}, {"run.start.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"input.accept.before", []PhaseCapabilityV1{PhaseFilterV1}}, {"input.accept.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"context.sources.collect", []PhaseCapabilityV1{PhasePortV1}}, {"context.frame.validate", []PhaseCapabilityV1{PhaseFilterV1}}, {"context.frame.frozen", []PhaseCapabilityV1{PhaseObserverV1}},
	{"model.request.prepare", []PhaseCapabilityV1{PhaseFilterV1}}, {"model.dispatch.before", []PhaseCapabilityV1{PhaseGateV1}}, {"model.response.observed", []PhaseCapabilityV1{PhaseObserverV1}}, {"model.output.validate", []PhaseCapabilityV1{PhaseFilterV1}},
	{"action.candidate.created", []PhaseCapabilityV1{PhaseObserverV1}}, {"action.admission", []PhaseCapabilityV1{PhaseFilterV1}}, {"action.review", []PhaseCapabilityV1{PhaseGateV1}}, {"action.dispatch", []PhaseCapabilityV1{PhasePortV1}}, {"action.result.normalize", []PhaseCapabilityV1{PhaseFilterV1}}, {"action.result.observed", []PhaseCapabilityV1{PhaseObserverV1}},
	{"action.batch.completed", []PhaseCapabilityV1{PhaseFilterV1, PhaseObserverV1}}, {"turn.continuation.evaluate", []PhaseCapabilityV1{PhaseFilterV1}}, {"turn.completed", []PhaseCapabilityV1{PhaseObserverV1}},
	{"context.compact.before", []PhaseCapabilityV1{PhaseGateV1}}, {"context.compact.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"checkpoint.create.before", []PhaseCapabilityV1{PhaseGateV1}}, {"checkpoint.create.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"run.pause.before", []PhaseCapabilityV1{PhaseGateV1}}, {"run.pause.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"run.resume.before", []PhaseCapabilityV1{PhaseGateV1}}, {"run.resume.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"agent.spawn.before", []PhaseCapabilityV1{PhaseGateV1}}, {"agent.spawn.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"agent.handoff.before", []PhaseCapabilityV1{PhaseGateV1}}, {"agent.handoff.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"subagent.completion.validate", []PhaseCapabilityV1{PhaseGateV1}},
	{"run.cancel.before", []PhaseCapabilityV1{PhaseGateV1}}, {"run.cancel.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"run.completion.validate", []PhaseCapabilityV1{PhaseGateV1}}, {"run.terminal.observed", []PhaseCapabilityV1{PhaseObserverV1}},
	{"session.end.before", []PhaseCapabilityV1{PhaseFilterV1}}, {"session.end.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"endpoint.close.before", []PhaseCapabilityV1{PhaseGateV1}}, {"endpoint.close.after", []PhaseCapabilityV1{PhaseObserverV1}},
	{"cleanup.before", []PhaseCapabilityV1{PhasePortV1}}, {"cleanup.after", []PhaseCapabilityV1{PhaseObserverV1}}, {"residual.detected", []PhaseCapabilityV1{PhaseObserverV1}},
}

func hookFaceSpec(phase string, kind PhaseCapabilityV1) HookFaceSpecV1 {
	id := "praxis.harness/" + strings.ReplaceAll(phase, ".", "-") + "-" + string(kind)
	value := HookFaceSpecV1{
		ContractVersion: ContractVersionV1, HookFaceID: id, PhaseID: phase, Kind: kind,
		InputSchema: catalogSchema("phase-" + phase + "-input"), OutputSchema: catalogSchema("phase-" + phase + "-output"),
		AuthorityCeiling: "candidate-only", EffectClass: "none-unless-port-owner-declares",
		TimeoutPolicy: "bounded-required", FailurePolicy: "fail-closed-if-required", ConcurrencyPolicy: "compiled-deterministic", ReceiptPolicy: "observation-only",
	}
	if kind == PhaseFilterV1 {
		value.MutationMask = append([]string(nil), filterMutationMasksV1[phase]...)
	}
	digest, _ := HookFaceSpecDigestV1(value)
	value.Digest = digest
	return value
}

var filterMutationMasksV1 = map[string][]string{
	"assembly.graph.compile.before": {"assembly.input.normalized"},
	"assembly.binding.validate":     {"binding.candidate.validated"},
	"assembly.preflight.before":     {"preflight.candidate.validated"},
	"session.start.before":          {"session.start.candidate"},
	"input.accept.before":           {"input.normalized.payload"},
	"context.frame.validate":        {"context.frame.candidate"},
	"model.request.prepare":         {"model.request.candidate"},
	"model.output.validate":         {"model.output.candidate"},
	"action.admission":              {"action.candidate.admission"},
	"action.result.normalize":       {"action.result.normalized"},
	"action.batch.completed":        {"action.batch.summary"},
	"turn.continuation.evaluate":    {"turn.continuation.candidate"},
	"session.end.before":            {"session.end.candidate"},
}

// HookFaceCatalogV1 returns a fresh copy of the versioned Observer/Filter/Gate/Port catalog.
func HookFaceCatalogV1() []HookFaceSpecV1 {
	values := make([]HookFaceSpecV1, 0, len(phaseCatalogEntriesV1))
	for _, entry := range phaseCatalogEntriesV1 {
		for _, kind := range entry.kinds {
			values = append(values, hookFaceSpec(entry.phase, kind))
		}
	}
	return values
}
