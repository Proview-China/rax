package corepack

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

// RegisterV1 owns only descriptor registration. Package Admission remains on
// the existing verification-aware Registry path and is intentionally not
// bypassed here.
func RegisterV1(target *registry.Registry, catalog CatalogV1, now time.Time) (registry.Record, error) {
	if target == nil || now.IsZero() {
		return registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Tool Pack registry target and time are required")
	}
	if err := catalog.Validate(); err != nil {
		return registry.Record{}, err
	}
	for _, capability := range catalog.Capabilities {
		record, err := target.SubmitCapability(capability, now)
		if err != nil {
			return registry.Record{}, err
		}
		for record.State != registry.StateActive {
			next := registry.StateAdmitted
			if record.State == registry.StateAdmitted {
				next = registry.StateActive
			}
			record, err = target.Transition("capability", string(capability.ID), record.RegistryRevision, next, now)
			if err != nil {
				current, exact, ok := target.ResolveCapability(string(capability.ID))
				if !ok || current.Revision != capability.Revision || current.Digest != capability.Digest {
					return registry.Record{}, err
				}
				record = exact
			}
		}
	}
	for _, tool := range catalog.Tools {
		record, err := target.SubmitTool(tool, now)
		if err != nil {
			return registry.Record{}, err
		}
		for record.State != registry.StateActive {
			next := registry.StateAdmitted
			if record.State == registry.StateAdmitted {
				next = registry.StateActive
			}
			record, err = target.Transition("tool", string(tool.ID), record.RegistryRevision, next, now)
			if err != nil {
				current, exact, ok := target.ResolveTool(string(tool.ID))
				if !ok || current.Revision != tool.Revision || current.Digest != tool.Digest {
					return registry.Record{}, err
				}
				record = exact
			}
		}
	}
	return target.SubmitPackage(catalog.Package, now)
}
