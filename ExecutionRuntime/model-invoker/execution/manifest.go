package execution

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

// CompareContextManifests enforces the prepared semantic surface as a subset
// of the observed surface. Adapters may append launch/probe evidence, but they
// cannot remove or rewrite a planned component, tool, or opaque boundary.
func CompareContextManifests(expected, actual union.ContextManifestSummary) error {
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("%w: invalid expected manifest: %v", ErrPreflightManifestDrift, err)
	}
	if err := actual.Validate(); err != nil {
		return fmt.Errorf("%w: invalid actual manifest: %v", ErrPreflightManifestDrift, err)
	}
	if err := verifyManifestDigest("expected", expected); err != nil {
		return err
	}
	if err := verifyManifestDigest("actual", actual); err != nil {
		return err
	}
	if expected.Version != actual.Version || expected.Mode != actual.Mode {
		return fmt.Errorf("%w: version or context mode changed", ErrPreflightManifestDrift)
	}
	actualComponents := make(map[string]union.ManifestComponent, len(actual.Components))
	for _, component := range actual.Components {
		actualComponents[component.Kind+"\x00"+component.Name] = component
	}
	for _, planned := range expected.Components {
		observed, exists := actualComponents[planned.Kind+"\x00"+planned.Name]
		if !exists {
			return fmt.Errorf("%w: component %s/%s is missing", ErrPreflightManifestDrift, planned.Kind, planned.Name)
		}
		if observed != planned {
			return fmt.Errorf("%w: component %s/%s changed", ErrPreflightManifestDrift, planned.Kind, planned.Name)
		}
	}
	actualTools := make(map[string]union.ToolSurfaceEntry, len(actual.Tools.Entries))
	for _, tool := range actual.Tools.Entries {
		actualTools[tool.ID] = tool
	}
	for _, planned := range expected.Tools.Entries {
		observed, exists := actualTools[planned.ID]
		if !exists {
			return fmt.Errorf("%w: tool %s is missing", ErrPreflightManifestDrift, planned.ID)
		}
		if observed != planned {
			return fmt.Errorf("%w: tool %s changed", ErrPreflightManifestDrift, planned.ID)
		}
	}
	actualOpaque := make(map[string]struct{}, len(actual.OpaqueFields))
	for _, path := range actual.OpaqueFields {
		actualOpaque[path] = struct{}{}
	}
	for _, path := range expected.OpaqueFields {
		if _, exists := actualOpaque[path]; !exists {
			return fmt.Errorf("%w: opaque boundary %s is missing", ErrPreflightManifestDrift, path)
		}
	}
	return nil
}

func verifyManifestDigest(label string, manifest union.ContextManifestSummary) error {
	if manifest.Digest == "" {
		return nil
	}
	computed, err := manifest.ComputeDigest()
	if err != nil || computed != manifest.Digest {
		return fmt.Errorf("%w: %s manifest digest changed", ErrPreflightManifestDrift, label)
	}
	return nil
}
