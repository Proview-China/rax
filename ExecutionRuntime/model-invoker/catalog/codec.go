package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const MaxDocumentBytes int64 = 16 << 20

// Decode reads exactly one strict JSON document. Unknown fields are rejected
// so misspelled policy and evidence fields cannot silently weaken a route.
func Decode(reader io.Reader) (Document, error) {
	if reader == nil {
		return Document{}, fmt.Errorf("decode upstream catalog: reader is nil")
	}
	payload, err := io.ReadAll(io.LimitReader(reader, MaxDocumentBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("decode upstream catalog: %w", err)
	}
	if int64(len(payload)) > MaxDocumentBytes {
		return Document{}, fmt.Errorf("decode upstream catalog: document exceeds %d bytes", MaxDocumentBytes)
	}
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return Document{}, fmt.Errorf("decode upstream catalog: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode upstream catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Document{}, fmt.Errorf("decode upstream catalog: multiple JSON values")
		}
		return Document{}, fmt.Errorf("decode upstream catalog trailing data: %w", err)
	}
	return document, nil
}

func rejectDuplicateJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing token %v", token)
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, location string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", location)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q at %s", key, location)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, location+"."+key); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object at %s is not closed", location)
		}
	case '[':
		for index := 0; decoder.More(); index++ {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array at %s is not closed", location)
		}
	default:
		return fmt.Errorf("unexpected delimiter %q at %s", delimiter, location)
	}
	return nil
}

type evidenceDigestPayload struct {
	Route           upstream.UpstreamRoute `json:"route"`
	Maturity        Maturity               `json:"maturity"`
	ModelDiscovery  ModelDiscovery         `json:"model_discovery"`
	Sources         []OfficialSource       `json:"official_sources"`
	Evidence        evidenceDigestEvidence `json:"evidence"`
	SDKs            []SDKMetadata          `json:"sdks"`
	Capabilities    []CapabilityMetadata   `json:"capabilities"`
	IgnoredFields   []string               `json:"ignored_fields"`
	ExtensionFields []string               `json:"extension_fields"`
	StreamEvents    []string               `json:"stream_events"`
	ErrorDialect    ErrorDialect           `json:"error_dialect"`
	Boundaries      OperationalBoundaries  `json:"boundaries"`
}

type evidenceDigestEvidence struct {
	Status                EvidenceStatus   `json:"status"`
	TTLClass              EvidenceTTLClass `json:"ttl_class"`
	CheckedAt             time.Time        `json:"checked_at"`
	ValidUntil            time.Time        `json:"valid_until"`
	InvalidatedBySourceID string           `json:"invalidated_by_source_id,omitempty"`
}

// ComputeEvidenceDigest returns a deterministic digest of every source-backed
// route assertion. Implementation state is deliberately excluded.
func ComputeEvidenceDigest(entry Entry) (string, error) {
	clone := entry.Clone()
	canonicalizeEntry(&clone)
	payload := evidenceDigestPayload{
		Route:          clone.Route,
		Maturity:       clone.Maturity,
		ModelDiscovery: clone.ModelDiscovery,
		Sources:        clone.Sources,
		Evidence: evidenceDigestEvidence{
			Status: clone.Evidence.Status, TTLClass: clone.Evidence.TTLClass,
			CheckedAt: clone.Evidence.CheckedAt, ValidUntil: clone.Evidence.ValidUntil,
			InvalidatedBySourceID: clone.Evidence.InvalidatedBySourceID,
		},
		SDKs: clone.SDKs, Capabilities: clone.Capabilities,
		IgnoredFields: clone.IgnoredFields, ExtensionFields: clone.ExtensionFields,
		StreamEvents: clone.StreamEvents, ErrorDialect: clone.ErrorDialect,
		Boundaries: clone.Boundaries,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("compute evidence digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:]), nil
}

func canonicalizeEntry(entry *Entry) {
	sort.Strings(entry.Route.Offering.Entitlement.ClientRestrictions)
	sort.Strings(entry.Route.Offering.Entitlement.AllowedClientNames)
	sort.Slice(entry.Route.Credential.References, func(i, j int) bool {
		left, right := entry.Route.Credential.References[i], entry.Route.Credential.References[j]
		return left.Purpose < right.Purpose || left.Purpose == right.Purpose && (left.Store < right.Store || left.Store == right.Store && left.Name < right.Name)
	})
	sort.Strings(entry.Route.Credential.Scopes)
	sort.Strings(entry.Route.Credential.KeyPrefixes)
	sort.Strings(entry.Route.Credential.DeniedKeyPrefixes)
	sort.Slice(entry.Route.Credential.AllowedProviderIDs, func(i, j int) bool {
		return entry.Route.Credential.AllowedProviderIDs[i] < entry.Route.Credential.AllowedProviderIDs[j]
	})
	sort.Slice(entry.Route.Credential.AllowedOfferingIDs, func(i, j int) bool {
		return entry.Route.Credential.AllowedOfferingIDs[i] < entry.Route.Credential.AllowedOfferingIDs[j]
	})
	sort.Slice(entry.Route.Credential.AllowedDeploymentIDs, func(i, j int) bool {
		return entry.Route.Credential.AllowedDeploymentIDs[i] < entry.Route.Credential.AllowedDeploymentIDs[j]
	})
	sort.Strings(entry.Route.Credential.AllowedRegions)
	sort.Slice(entry.Route.Credential.AllowedEndpointIDs, func(i, j int) bool {
		return entry.Route.Credential.AllowedEndpointIDs[i] < entry.Route.Credential.AllowedEndpointIDs[j]
	})
	sort.Slice(entry.ModelDiscovery.Aliases, func(i, j int) bool {
		left, right := entry.ModelDiscovery.Aliases[i], entry.ModelDiscovery.Aliases[j]
		return left.Alias < right.Alias || left.Alias == right.Alias && left.ProviderModelRef < right.ProviderModelRef
	})
	sort.Slice(entry.Sources, func(i, j int) bool { return entry.Sources[i].ID < entry.Sources[j].ID })
	sort.Slice(entry.SDKs, func(i, j int) bool {
		left, right := entry.SDKs[i], entry.SDKs[j]
		return left.Language < right.Language || left.Language == right.Language && left.Package < right.Package
	})
	for index := range entry.Capabilities {
		sort.Strings(entry.Capabilities[index].Limitations)
	}
	sort.Slice(entry.Capabilities, func(i, j int) bool { return entry.Capabilities[i].ID < entry.Capabilities[j].ID })
	sort.Strings(entry.IgnoredFields)
	sort.Strings(entry.ExtensionFields)
	sort.Strings(entry.StreamEvents)
	sort.Strings(entry.ErrorDialect.RequestIDHeaders)
	sort.Strings(entry.ErrorDialect.RetryHeaders)
}

func Encode(writer io.Writer, document Document) error {
	if writer == nil {
		return fmt.Errorf("encode upstream catalog: writer is nil")
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode upstream catalog: %w", err)
	}
	return nil
}
