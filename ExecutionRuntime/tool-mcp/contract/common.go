package contract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var ErrExternalEffectUnsupported = core.NewError(core.ErrorPreconditionFailed, core.ReasonRecoveryEffectNotPermitted, "real external effects are unsupported in tool-mcp Wave 1")

type ObjectRef struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ObjectRef) Validate() error {
	if !ValidObjectID(r.ID) || r.Revision == 0 {
		return invalid("object reference requires stable id and non-zero revision")
	}
	return r.Digest.Validate()
}

func ValidObjectID(value string) bool {
	return ValidateStableID(value) == nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(value)) == nil
}

type Residual struct {
	Class       runtimeports.ResidualClassV2  `json:"class"`
	Code        runtimeports.NamespacedNameV2 `json:"code"`
	Inspectable bool                          `json:"inspectable"`
	Detail      string                        `json:"detail,omitempty"`
}

func (r Residual) Validate() error {
	if r.Class == runtimeports.ResidualNone || runtimeports.ValidateNamespacedNameV2(r.Code) != nil || len(r.Detail) > MaxStringBytes {
		return invalid("residual requires non-none class, namespaced code and bounded detail")
	}
	return nil
}

func StableID(prefix string, parts ...string) (string, error) {
	if !validToken(prefix) || len(parts) == 0 {
		return "", invalid("stable id requires a canonical prefix and at least one part")
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || len(part) > MaxStringBytes {
			return "", invalid("stable id parts must be non-blank and bounded")
		}
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.id", "v1", prefix, parts)
	if err != nil {
		return "", err
	}
	return prefix + "_" + strings.TrimPrefix(string(digest), "sha256:")[:32], nil
}

func ValidateStableID(value string) error {
	if len(value) < 5 || len(value) > 96 || strings.TrimSpace(value) != value {
		return invalid("stable id is not bounded canonical ASCII")
	}
	for _, c := range []byte(value) {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return invalid("stable id contains unsupported characters")
		}
	}
	return nil
}

func Seal(domain, version, discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest(domain, version, discriminator, value)
}

func ValidateSortedUniqueNames(values []runtimeports.NamespacedNameV2, maximum int) error {
	if len(values) == 0 || len(values) > maximum {
		return invalid("namespaced list is empty or exceeds its limit")
	}
	for i, value := range values {
		if runtimeports.ValidateNamespacedNameV2(value) != nil {
			return invalid("namespaced list contains an invalid value")
		}
		if i > 0 && string(values[i-1]) >= string(value) {
			return invalid("namespaced list must be sorted and unique")
		}
	}
	return nil
}

func SortedUniqueNames(values []runtimeports.NamespacedNameV2) []runtimeports.NamespacedNameV2 {
	seen := make(map[runtimeports.NamespacedNameV2]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]runtimeports.NamespacedNameV2, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validToken(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func conflict(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

func ContainsName(values []runtimeports.NamespacedNameV2, target runtimeports.NamespacedNameV2) bool {
	i := sort.Search(len(values), func(i int) bool { return values[i] >= target })
	return i < len(values) && values[i] == target
}

func RequireDigest(label string, digest core.Digest) error {
	if err := digest.Validate(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
