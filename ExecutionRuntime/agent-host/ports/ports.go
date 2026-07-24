package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type HostV1 interface {
	Validate(context.Context, contract.ValidateRequestV1) (contract.ValidateResultV1, error)
	Assemble(context.Context, contract.AssembleRequestV1) (contract.AssembleResultV1, error)
	Start(context.Context, contract.StartRequestV1) (contract.StartResultV1, error)
	Inspect(context.Context, contract.InspectRequestV1) (contract.InspectResultV1, error)
	Stop(context.Context, contract.StopRequestV1) (contract.StopResultV1, error)
}

// DefinitionDecoderV1 is Host-owned dependency inversion. Its production
// implementation remains owned by agent-definition.
type DefinitionDecoderV1 interface {
	DecodeDefinitionV1(context.Context, contract.HostConfigV1) (contract.DecodedDefinitionV1, error)
}

// DefinitionSourceCurrentReaderV1 resolves an opaque configured source id to
// one exact Agent Definition owner ref without type-punning source and owner ids.
type DefinitionSourceCurrentReaderV1 interface {
	InspectDefinitionSourceCurrentV1(context.Context, string) (contract.DefinitionSourceCurrentV1, error)
}

// AgentAssemblerV1 is Host-owned dependency inversion. It neither permits the
// Host to rewrite Definition semantics nor transfers plan ownership.
type AgentAssemblerV1 interface {
	ResolveAgentV1(context.Context, contract.HostConfigV1, contract.DecodedDefinitionV1) (contract.ResolvedAssemblyV1, error)
}

// ResolutionInputsCurrentReaderV1 atomically resolves the two opaque Host
// configuration ids to exact Agent Assembler owner refs. Consumers must use
// S1/exact/S2 and compare the complete projections.
type ResolutionInputsCurrentReaderV1 interface {
	InspectResolutionInputsCurrentV1(context.Context, string, string) (contract.ResolutionInputsCurrentV1, error)
}

type HarnessCompilerV1 interface {
	CompileHarnessV1(context.Context, contract.HostConfigV1, contract.ResolvedAssemblyV1) (contract.CompiledAssemblyV1, error)
}

type HarnessCompilerArtifactsV2 interface {
	CompileHarnessArtifactsV2(context.Context, contract.HostConfigV1, contract.ResolvedAssemblyV1) (contract.CompiledAssemblyArtifactsV2, error)
}

type AssemblyPublisherV2 interface {
	PublishAssemblyV2(context.Context, contract.AssemblyPublicationRequestV2) (contract.AssemblyPublicationResultV2, error)
}

// BindingPortV1 is intentionally narrow until H3 supplies the real Runtime
// association path. StartOrInspectBindingV1 must implement canonical start-or-inspect:
// after a lost/unknown create reply it inspects the original Runtime Binding
// identity and must never create a second Binding. A returned ref is proof
// input, never Host-owned authority.
type BindingPortV1 interface {
	StartOrInspectBindingV1(context.Context, BindingRequestV1) (contract.ExactRefV1, error)
	InspectBindingV1(context.Context, BindingRequestV1) (contract.ExactRefV1, error)
}

type BindingRequestV1 struct {
	HostID       string
	StartID      string
	ConfigDigest contract.DigestV1
	Attempt      contract.BindingAttemptV1
	Config       contract.HostConfigV1
	Definition   contract.DecodedDefinitionV1
	Resolved     contract.ResolvedAssemblyV1
	Compiled     contract.CompiledAssemblyV1
}

type ReadinessPortV1 interface {
	VerifySystemReadyV1(context.Context, ReadinessRequestV1) (contract.SystemReadyV1, error)
	InspectSystemReadyV1(context.Context, contract.ExactRefV1) (contract.SystemReadyV1, error)
}

type ReadinessRequestV1 struct {
	HostID     string
	StartID    string
	Definition contract.DecodedDefinitionV1
	Resolved   contract.ResolvedAssemblyV1
	Compiled   contract.CompiledAssemblyV1
	BindingRef contract.ExactRefV1
	Components []contract.ConstructedComponentV1
}

type JournalFactPortV1 interface {
	CreateHostJournalV1(context.Context, contract.HostJournalV1) (contract.HostJournalV1, error)
	CompareAndSwapHostJournalV1(context.Context, contract.ExactRefV1, contract.HostJournalV1) (contract.HostJournalV1, error)
	InspectHostJournalV1(context.Context, string, string) (contract.HostJournalV1, error)
}

type JournalFactPortV2 interface {
	CreateHostJournalV2(context.Context, contract.HostJournalV2) (contract.HostJournalV2, error)
	CompareAndSwapHostJournalV2(context.Context, contract.ExactRefV1, contract.HostJournalV2) (contract.HostJournalV2, error)
	InspectHostJournalV2(context.Context, string, string) (contract.HostJournalV2, error)
}

type ComponentFactoryV1 interface {
	StartOrInspectConstructionV1(context.Context, ConstructRequestV1) (ComponentHandleV1, error)
	InspectConstructionV1(context.Context, ConstructRequestV1) (ComponentHandleV1, error)
}

type ConstructRequestV1 struct {
	HostID       string
	StartID      string
	Node         contract.ComponentNodeV1
	Attempt      contract.ConstructionAttemptV1
	Dependencies []contract.ConstructedComponentV1
}

type ComponentHandleV1 interface {
	RefV1() contract.ExactRefV1
	CleanupV1(context.Context) (contract.CleanupItemV1, error)
}

type ClockV1 interface{ Now() time.Time }

// HostStartClaimCurrentReaderV1 is the narrow read-only current seam used by
// SystemReady and supervisors. The exact Ref prevents Host orchestration from
// substituting a free expiry or another Host/Start claim.
type HostStartClaimCurrentReaderV1 interface {
	InspectHostStartClaimCurrentV1(context.Context, contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error)
}

// HostStartClaimPortV1 is the single version-neutral conflict-domain owner for
// governed V1/V2 Host starts. Implementations must retain HostID+StartID
// uniqueness after expiry; expiry never authorizes replacement content.
type HostStartClaimPortV1 interface {
	HostStartClaimCurrentReaderV1
	ClaimOrInspectHostStartV1(context.Context, contract.HostStartClaimV1) (contract.HostStartClaimV1, error)
	InspectHostStartClaimV1(context.Context, string, string) (contract.HostStartClaimV1, error)
}

type CleanupAttemptFactPortV2 interface {
	CreateCleanupAttemptV2(context.Context, contract.CleanupAttemptV2) (contract.CleanupAttemptV2, error)
	CompareAndSwapCleanupAttemptV2(context.Context, contract.ExactRefV1, contract.CleanupAttemptV2) (contract.CleanupAttemptV2, error)
	InspectCleanupAttemptV2(context.Context, string) (contract.CleanupAttemptV2, error)
}

// ControlAdapterFactoryV2 is deliberately distinct from ComponentFactoryV1.
// It may only assemble in-memory control adapters over exact pre-opened
// resources. Component-specific typed wiring remains in the composition root;
// this interface exposes no generic raw-provider or resource-handle resolver.
type ControlAdapterFactoryV2 interface {
	DescriptorV2() contract.ControlAdapterFactoryDescriptorV2
	StartOrInspectControlAdapterV2(context.Context, contract.ControlAdapterConstructRequestV2) (ControlAdapterHandleV2, error)
	InspectControlAdapterV2(context.Context, contract.ControlAdapterConstructRequestV2) (ControlAdapterHandleV2, error)
}

type ControlAdapterHandleV2 interface {
	InstanceV2() contract.ControlAdapterInstanceV2
}

type ControlAdapterConformanceReaderV2 interface {
	InspectControlAdapterConformanceV2(context.Context, contract.ControlAdapterFactoryRefV2) (contract.ControlAdapterConformanceV2, error)
}

type SystemReadyCurrentReaderV2 interface {
	InspectSystemReadyCurrentV2(context.Context, contract.SystemReadyCurrentRefV2) (contract.SystemReadyCurrentV2, error)
}

// SystemReadyAvailabilitySourceV2 is the exact association seam between the
// Host-owned SystemReady Current and its Runtime-neutral availability Ref. It
// does not expose discovery by ID or a writable Runtime fact.
type SystemReadyAvailabilitySourceV2 interface {
	InspectSystemReadyCurrentForAvailabilityV2(context.Context, runtimeports.AgentExecutionAvailabilityRefV1) (contract.SystemReadyCurrentV2, error)
}

type SystemReadyFactPortV2 interface {
	SystemReadyCurrentReaderV2
	CreateSystemReadyFactV2(context.Context, contract.SystemReadyFactV2) (contract.SystemReadyFactV2, error)
	InspectSystemReadyFactV2(context.Context, contract.SystemReadyFactRefV2) (contract.SystemReadyFactV2, error)
	CreateSystemReadyCurrentV2(context.Context, contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error)
	CompareAndSwapSystemReadyCurrentV2(context.Context, contract.SystemReadyCurrentRefV2, contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error)
}
