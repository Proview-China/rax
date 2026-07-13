package profile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ManifestProbeStatus string

const (
	ManifestProbeNotRun   ManifestProbeStatus = "not_run"
	ManifestProbeReported ManifestProbeStatus = "reported"
	ManifestProbeObserved ManifestProbeStatus = "observed"
)

type ManifestFieldState string

const (
	ManifestFieldPresent ManifestFieldState = "present"
	ManifestFieldAbsent  ManifestFieldState = "absent"
	ManifestFieldOpaque  ManifestFieldState = "opaque"
)

type ManifestEvidenceSource string

const (
	ManifestEvidenceReported           ManifestEvidenceSource = "reported"
	ManifestEvidenceObserved           ManifestEvidenceSource = "observed"
	ManifestEvidenceInferredFromConfig ManifestEvidenceSource = "inferred_from_config"
	ManifestEvidenceInferredFromSource ManifestEvidenceSource = "inferred_from_source"
	ManifestEvidenceOpaque             ManifestEvidenceSource = "opaque"
)

type ManifestEvidence struct {
	Source     ManifestEvidenceSource `json:"source"`
	Confidence int                    `json:"confidence"`
	Reference  string                 `json:"reference,omitempty"`
}

type ManifestField struct {
	Path     string             `json:"path"`
	State    ManifestFieldState `json:"state"`
	Value    string             `json:"value,omitempty"`
	Evidence ManifestEvidence   `json:"evidence,omitempty"`
}

type InjectionManifest struct {
	SchemaVersion string              `json:"schema_version"`
	ProbeStatus   ManifestProbeStatus `json:"probe_status"`
	Fields        []ManifestField     `json:"fields"`
}

func (manifest InjectionManifest) Clone() InjectionManifest {
	clone := manifest
	clone.Fields = append([]ManifestField(nil), manifest.Fields...)
	return clone
}

func (manifest *InjectionManifest) normalize() {
	if manifest == nil {
		return
	}
	sort.Slice(manifest.Fields, func(i, j int) bool { return manifest.Fields[i].Path < manifest.Fields[j].Path })
}

func (manifest InjectionManifest) Validate(actual bool) error {
	if !validVersion(manifest.SchemaVersion) {
		return fmt.Errorf("manifest schema version is required")
	}
	switch manifest.ProbeStatus {
	case ManifestProbeNotRun, ManifestProbeReported, ManifestProbeObserved:
	default:
		return fmt.Errorf("unknown manifest probe status %q", manifest.ProbeStatus)
	}
	if actual && manifest.ProbeStatus == ManifestProbeNotRun && len(manifest.Fields) != 0 {
		return fmt.Errorf("not_run actual manifest must not contain fields")
	}
	seen := make(map[string]struct{}, len(manifest.Fields))
	for index, field := range manifest.Fields {
		if !validManifestPath(field.Path) {
			return fmt.Errorf("manifest field %d has an invalid path", index)
		}
		if _, duplicate := seen[field.Path]; duplicate {
			return fmt.Errorf("manifest duplicates field %q", field.Path)
		}
		seen[field.Path] = struct{}{}
		switch field.State {
		case ManifestFieldPresent:
			if field.Value == "" {
				return fmt.Errorf("manifest field %q present value is required", field.Path)
			}
		case ManifestFieldAbsent:
			if field.Value != "" {
				return fmt.Errorf("manifest field %q absent value must be empty", field.Path)
			}
		case ManifestFieldOpaque:
			if field.Value != "" {
				return fmt.Errorf("manifest field %q opaque value must be empty", field.Path)
			}
		default:
			return fmt.Errorf("manifest field %q has unknown state %q", field.Path, field.State)
		}
		if actual {
			if err := field.Evidence.Validate(field.State); err != nil {
				return fmt.Errorf("manifest field %q evidence: %w", field.Path, err)
			}
		}
	}
	return nil
}

func (evidence ManifestEvidence) Validate(state ManifestFieldState) error {
	switch evidence.Source {
	case ManifestEvidenceReported, ManifestEvidenceObserved, ManifestEvidenceInferredFromConfig, ManifestEvidenceInferredFromSource:
		if state == ManifestFieldOpaque {
			return fmt.Errorf("opaque field must use opaque evidence")
		}
	case ManifestEvidenceOpaque:
		if state != ManifestFieldOpaque {
			return fmt.Errorf("opaque evidence requires opaque field state")
		}
	default:
		return fmt.Errorf("evidence source is required")
	}
	if evidence.Confidence < 0 || evidence.Confidence > 100 {
		return fmt.Errorf("confidence must be between 0 and 100")
	}
	if evidence.Reference == "" {
		return fmt.Errorf("evidence reference is required")
	}
	return nil
}

func (manifest InjectionManifest) Digest(actual bool) (string, error) {
	if err := manifest.Validate(actual); err != nil {
		return "", err
	}
	clone := manifest.Clone()
	clone.normalize()
	return digestJSON(clone)
}

type ManifestDriftLevel string

const (
	ManifestDriftP0 ManifestDriftLevel = "P0"
	ManifestDriftP1 ManifestDriftLevel = "P1"
	ManifestDriftP2 ManifestDriftLevel = "P2"
	ManifestDriftP3 ManifestDriftLevel = "P3"
)

type ManifestDiffKind string

const (
	ManifestDiffMissing      ManifestDiffKind = "missing"
	ManifestDiffUnexpected   ManifestDiffKind = "unexpected"
	ManifestDiffValueChanged ManifestDiffKind = "value_changed"
	ManifestDiffBecameOpaque ManifestDiffKind = "became_opaque"
	ManifestDiffStateChanged ManifestDiffKind = "state_changed"
)

type ManifestDiff struct {
	Path     string             `json:"path"`
	Level    ManifestDriftLevel `json:"level"`
	Kind     ManifestDiffKind   `json:"kind"`
	Expected *ManifestField     `json:"expected,omitempty"`
	Actual   *ManifestField     `json:"actual,omitempty"`
}

type ManifestResidual struct {
	Path       string             `json:"path"`
	Level      ManifestDriftLevel `json:"level"`
	Impact     string             `json:"impact"`
	Mitigation string             `json:"mitigation"`
}

type ManifestEvaluation struct {
	Allowed     bool               `json:"allowed"`
	Differences []ManifestDiff     `json:"differences"`
	Residuals   []ManifestResidual `json:"residuals"`
}

func CompareManifests(expected, actual InjectionManifest, mode ContextMode, allowedP2Paths []string) (ManifestEvaluation, error) {
	if err := expected.Validate(false); err != nil {
		return ManifestEvaluation{}, fmt.Errorf("expected manifest: %w", err)
	}
	if err := actual.Validate(true); err != nil {
		return ManifestEvaluation{}, fmt.Errorf("actual manifest: %w", err)
	}
	if actual.ProbeStatus == ManifestProbeNotRun {
		return ManifestEvaluation{}, &Error{Code: ErrorManifestUnavailable, Operation: "compare_manifest", Message: "actual manifest probe was not run"}
	}
	expectedFields := manifestFieldMap(expected.Fields)
	actualFields := manifestFieldMap(actual.Fields)
	paths := make([]string, 0, len(expectedFields)+len(actualFields))
	seen := make(map[string]struct{}, len(expectedFields)+len(actualFields))
	for path := range expectedFields {
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range actualFields {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	evaluation := ManifestEvaluation{Allowed: true}
	for _, path := range paths {
		expectedField, hasExpected := expectedFields[path]
		actualField, hasActual := actualFields[path]
		kind, differs := manifestDifference(expectedField, hasExpected, actualField, hasActual)
		if !differs {
			continue
		}
		level := classifyManifestDrift(path, expectedField, hasExpected, actualField, hasActual)
		diff := ManifestDiff{Path: path, Level: level, Kind: kind}
		if hasExpected {
			copy := expectedField
			diff.Expected = &copy
		}
		if hasActual {
			copy := actualField
			diff.Actual = &copy
		}
		evaluation.Differences = append(evaluation.Differences, diff)

		allowed := manifestDifferenceAllowed(mode, level, path, allowedP2Paths)
		if !allowed {
			evaluation.Allowed = false
			continue
		}
		evaluation.Residuals = append(evaluation.Residuals, ManifestResidual{
			Path: path, Level: level, Impact: manifestImpact(kind),
			Mitigation: "preserve drift in MappingReport and enforce RuntimePolicy",
		})
	}
	return evaluation, nil
}

func manifestFieldMap(fields []ManifestField) map[string]ManifestField {
	result := make(map[string]ManifestField, len(fields))
	for _, field := range fields {
		result[field.Path] = field
	}
	return result
}

func manifestDifference(expected ManifestField, hasExpected bool, actual ManifestField, hasActual bool) (ManifestDiffKind, bool) {
	switch {
	case hasExpected && !hasActual:
		return ManifestDiffMissing, true
	case !hasExpected && hasActual:
		return ManifestDiffUnexpected, true
	case actual.State == ManifestFieldOpaque && expected.State != ManifestFieldOpaque:
		return ManifestDiffBecameOpaque, true
	case expected.State != actual.State:
		return ManifestDiffStateChanged, true
	case expected.Value != actual.Value:
		return ManifestDiffValueChanged, true
	default:
		return "", false
	}
}

func classifyManifestDrift(path string, _ ManifestField, _ bool, actual ManifestField, hasActual bool) ManifestDriftLevel {
	switch {
	case strings.HasPrefix(path, "identity."),
		strings.HasPrefix(path, "auth."),
		strings.HasPrefix(path, "sandbox."),
		strings.HasPrefix(path, "secrets."),
		strings.HasPrefix(path, "workspace.root"),
		strings.HasSuffix(path, ".execution_owner"):
		return ManifestDriftP0
	case strings.HasPrefix(path, "instructions."),
		strings.HasPrefix(path, "context.sources."),
		strings.HasPrefix(path, "permissions."),
		strings.HasPrefix(path, "hooks."),
		strings.HasPrefix(path, "plugins."),
		strings.HasPrefix(path, "skills."),
		strings.HasPrefix(path, "mcp."),
		strings.HasPrefix(path, "memory."),
		strings.HasSuffix(path, ".schema_digest"),
		strings.HasSuffix(path, ".registered"),
		strings.HasSuffix(path, ".executable"):
		if hasActual && strings.HasSuffix(path, ".executable") && actual.State == ManifestFieldPresent && actual.Value == "false" {
			return ManifestDriftP2
		}
		return ManifestDriftP1
	case strings.HasPrefix(path, "event."),
		strings.HasSuffix(path, ".model_visible"),
		strings.HasPrefix(path, "compaction."),
		strings.HasPrefix(path, "retry."):
		return ManifestDriftP2
	case strings.HasPrefix(path, "display."), strings.HasPrefix(path, "telemetry."):
		return ManifestDriftP3
	default:
		return ManifestDriftP1
	}
}

func manifestDifferenceAllowed(mode ContextMode, level ManifestDriftLevel, path string, allowedP2Paths []string) bool {
	switch level {
	case ManifestDriftP0:
		return false
	case ManifestDriftP1:
		return mode != ContextSemanticStable
	case ManifestDriftP2:
		if mode != ContextSemanticStable {
			return true
		}
		return containsString(allowedP2Paths, path)
	case ManifestDriftP3:
		return true
	default:
		return false
	}
}

func manifestImpact(kind ManifestDiffKind) string {
	switch kind {
	case ManifestDiffUnexpected:
		return "actual Harness surface contains an unplanned field"
	case ManifestDiffMissing:
		return "expected Harness field is unavailable"
	case ManifestDiffBecameOpaque:
		return "expected field can no longer be observed"
	default:
		return "actual Harness semantics differ from the expected manifest"
	}
}

func validManifestPath(path string) bool {
	if path == "" || len(path) > 512 || strings.ContainsAny(path, "\x00\r\n") {
		return false
	}
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		if !validStableName(segment) {
			return false
		}
	}
	return true
}

func (evaluation ManifestEvaluation) Digest() (string, error) {
	encoded, err := json.Marshal(evaluation)
	if err != nil {
		return "", err
	}
	return DigestString(string(encoded)), nil
}
