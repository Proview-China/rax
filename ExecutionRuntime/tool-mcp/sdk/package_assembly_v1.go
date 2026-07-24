package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

// PackageAssemblyV1 is an SDK read projection. It contains only exact,
// active Registry facts from one immutable Snapshot coordinate. It is not a
// Package verification, installation, enablement, or runtime authority fact.
type PackageAssemblyV1 struct {
	RegistrySnapshot  RegistrySnapshotRefV1           `json:"registry_snapshot"`
	Package           contract.ToolPackageManifest    `json:"package"`
	PackageRecord     registry.Record                 `json:"package_record"`
	Tools             []contract.ToolDescriptor       `json:"tools"`
	ToolRecords       []registry.Record               `json:"tool_records"`
	Capabilities      []contract.CapabilityDescriptor `json:"capabilities"`
	CapabilityRecords []registry.Record               `json:"capability_records"`
}

func (a PackageAssemblyV1) Validate() error {
	if a.RegistrySnapshot.Validate() != nil || a.Package.Validate() != nil || a.PackageRecord.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package assembly identity is invalid")
	}
	if a.PackageRecord.Kind != "package" || a.PackageRecord.ID != string(a.Package.ID) || a.PackageRecord.ObjectRevision != a.Package.Revision || a.PackageRecord.ObjectDigest != a.Package.Digest || a.PackageRecord.State != registry.StateActive {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly does not bind an active exact Package")
	}
	if a.PackageRecord.RegistryRevision > a.RegistrySnapshot.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly Package is newer than the exact Registry Snapshot")
	}
	if len(a.Tools) != len(a.Package.Descriptors) || len(a.ToolRecords) != len(a.Tools) || len(a.Capabilities) != len(a.Tools) || len(a.CapabilityRecords) != len(a.Tools) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly closure cardinality drifted")
	}
	for i, descriptor := range a.Package.Descriptors {
		tool, toolRecord := a.Tools[i], a.ToolRecords[i]
		capability, capabilityRecord := a.Capabilities[i], a.CapabilityRecords[i]
		if tool.Validate() != nil || toolRecord.Validate() != nil || capability.Validate() != nil || capabilityRecord.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package assembly contains an invalid Registry object")
		}
		if string(descriptor.ToolID) != string(tool.ID) || descriptor.Revision != tool.Revision || descriptor.Digest != tool.Digest || toolRecord.Kind != "tool" || toolRecord.ID != string(tool.ID) || toolRecord.ObjectRevision != tool.Revision || toolRecord.ObjectDigest != tool.Digest || toolRecord.State != registry.StateActive {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly Tool differs from its exact active descriptor")
		}
		if toolRecord.RegistryRevision > a.RegistrySnapshot.Revision || capabilityRecord.RegistryRevision > a.RegistrySnapshot.Revision {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly dependency is newer than the exact Registry Snapshot")
		}
		if capabilityRecord.Kind != "capability" || capabilityRecord.ID != string(capability.ID) || capabilityRecord.ObjectRevision != capability.Revision || capabilityRecord.ObjectDigest != capability.Digest || capabilityRecord.State != registry.StateActive || tool.Capability.ID != string(capability.ID) || tool.Capability.Revision != capability.Revision || tool.Capability.Digest != capability.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package assembly Capability differs from its exact active Tool binding")
		}
		if err := tool.ValidateAgainst(capability); err != nil {
			return err
		}
		for _, effect := range tool.EffectKinds {
			if !contract.ContainsName(a.Package.EffectKinds, effect) {
				return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "Package assembly under-declares a Tool effect")
			}
		}
	}
	return nil
}

// ResolvePackageForAssemblyV1 resolves one package and its complete Tool and
// Capability closure under a single exact Registry Snapshot. It never resolves
// aliases, fetches artifacts, verifies signatures, or changes Registry state.
func (s *SDKV1) ResolvePackageForAssemblyV1(ctx context.Context, id runtimeports.NamespacedNameV2, snapshotRef RegistrySnapshotRefV1) (PackageAssemblyV1, error) {
	started, err := s.readyV1(ctx)
	if err != nil {
		return PackageAssemblyV1{}, err
	}
	if runtimeports.ValidateNamespacedNameV2(id) != nil || snapshotRef.Validate() != nil {
		return PackageAssemblyV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package assembly resolution coordinates are invalid")
	}
	if err = s.validateSnapshotExactV1(snapshotRef); err != nil {
		return PackageAssemblyV1{}, err
	}
	manifest, packageRecord, ok := s.registry.ResolvePackage(string(id))
	if !ok {
		return PackageAssemblyV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Package was not found for assembly")
	}
	if manifest.Validate() != nil || packageRecord.Validate() != nil || packageRecord.State != registry.StateActive || packageRecord.ObjectRevision != manifest.Revision || packageRecord.ObjectDigest != manifest.Digest {
		return PackageAssemblyV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Package is not an active exact Registry object")
	}
	assembly := PackageAssemblyV1{
		RegistrySnapshot:  snapshotRef,
		Package:           manifest,
		PackageRecord:     packageRecord,
		Tools:             make([]contract.ToolDescriptor, 0, len(manifest.Descriptors)),
		ToolRecords:       make([]registry.Record, 0, len(manifest.Descriptors)),
		Capabilities:      make([]contract.CapabilityDescriptor, 0, len(manifest.Descriptors)),
		CapabilityRecords: make([]registry.Record, 0, len(manifest.Descriptors)),
	}
	for _, descriptor := range manifest.Descriptors {
		tool, toolRecord, found := s.registry.ResolveTool(string(descriptor.ToolID))
		if !found || tool.Validate() != nil || toolRecord.Validate() != nil || toolRecord.State != registry.StateActive || tool.Revision != descriptor.Revision || tool.Digest != descriptor.Digest || toolRecord.ObjectRevision != descriptor.Revision || toolRecord.ObjectDigest != descriptor.Digest {
			return PackageAssemblyV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Package Tool is absent, inactive, or differs from its exact descriptor")
		}
		capability, capabilityRecord, found := s.registry.ResolveCapability(tool.Capability.ID)
		if !found || capability.Validate() != nil || capabilityRecord.Validate() != nil || capabilityRecord.State != registry.StateActive || capability.Revision != tool.Capability.Revision || capability.Digest != tool.Capability.Digest || capabilityRecord.ObjectRevision != capability.Revision || capabilityRecord.ObjectDigest != capability.Digest {
			return PackageAssemblyV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Package Capability is absent, inactive, or differs from the Tool binding")
		}
		assembly.Tools = append(assembly.Tools, tool)
		assembly.ToolRecords = append(assembly.ToolRecords, toolRecord)
		assembly.Capabilities = append(assembly.Capabilities, capability)
		assembly.CapabilityRecords = append(assembly.CapabilityRecords, capabilityRecord)
	}
	if err = assembly.Validate(); err != nil {
		return PackageAssemblyV1{}, err
	}
	if err = s.validateSnapshotExactV1(snapshotRef); err != nil {
		return PackageAssemblyV1{}, err
	}
	finished, err := s.readyV1(ctx)
	if err != nil {
		return PackageAssemblyV1{}, err
	}
	if finished.Before(started) {
		return PackageAssemblyV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Package assembly clock regressed")
	}
	return clonePackageAssemblyV1(assembly), nil
}

func clonePackageAssemblyV1(value PackageAssemblyV1) PackageAssemblyV1 {
	value.Package.Signatures = append([]core.Digest(nil), value.Package.Signatures...)
	value.Package.Descriptors = append([]contract.PackageDescriptorRef(nil), value.Package.Descriptors...)
	value.Package.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.Package.EffectKinds...)
	value.Tools = append([]contract.ToolDescriptor(nil), value.Tools...)
	for i := range value.Tools {
		value.Tools[i].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.Tools[i].EffectKinds...)
		value.Tools[i].Residuals = append([]contract.Residual(nil), value.Tools[i].Residuals...)
	}
	value.ToolRecords = append([]registry.Record(nil), value.ToolRecords...)
	value.Capabilities = append([]contract.CapabilityDescriptor(nil), value.Capabilities...)
	for i := range value.Capabilities {
		value.Capabilities[i].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.Capabilities[i].EffectKinds...)
	}
	value.CapabilityRecords = append([]registry.Record(nil), value.CapabilityRecords...)
	return value
}
