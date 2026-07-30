package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

const MaxContextModelInputSegmentsV1 = 512

type ContextModelInputChannelV1 string

const (
	ContextModelInputInstructionV1    ContextModelInputChannelV1 = "instruction"
	ContextModelInputMessageV1        ContextModelInputChannelV1 = "input_message"
	ContextModelInputFunctionCallV1   ContextModelInputChannelV1 = "function_call"
	ContextModelInputFunctionResultV1 ContextModelInputChannelV1 = "function_result"
	ContextModelInputReferenceV1      ContextModelInputChannelV1 = "reference"
)

type ContextModelInputRoleV1 string

const (
	ContextModelInputRoleSystemV1    ContextModelInputRoleV1 = "system"
	ContextModelInputRoleDeveloperV1 ContextModelInputRoleV1 = "developer"
	ContextModelInputRoleUserV1      ContextModelInputRoleV1 = "user"
	ContextModelInputRoleAssistantV1 ContextModelInputRoleV1 = "assistant"
	ContextModelInputRoleToolV1      ContextModelInputRoleV1 = "tool"
)

type ContextModelInputEncodingV1 string

const (
	ContextModelInputUTF8V1            ContextModelInputEncodingV1 = "utf8"
	ContextModelInputCanonicalJSONV1   ContextModelInputEncodingV1 = "canonical_json"
	ContextModelInputArtifactRefJSONV1 ContextModelInputEncodingV1 = "artifact_ref_json"
)

type ContextModelInputMaterialRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (r ContextModelInputMaterialRefV1) Validate() error {
	if validateID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: model input material reference", ErrInvalid)
	}
	return nil
}

// ContextModelInputSegmentBindingV1 is explicit caller-owned semantic input.
// CallID and Name are never recovered from Content. They are bound here before
// Context materialization and represented in the material by Digest.
type ContextModelInputSegmentBindingV1 struct {
	FragmentRef FactRef                     `json:"fragment_ref"`
	Region      FrameRegion                 `json:"region"`
	Position    uint32                      `json:"position"`
	Kind        FragmentKind                `json:"kind"`
	Trust       TrustClass                  `json:"trust"`
	Channel     ContextModelInputChannelV1  `json:"channel"`
	Role        ContextModelInputRoleV1     `json:"role"`
	Encoding    ContextModelInputEncodingV1 `json:"encoding"`
	CallID      string                      `json:"call_id,omitempty"`
	Name        string                      `json:"name,omitempty"`
	Digest      Digest                      `json:"digest"`
}

func (b ContextModelInputSegmentBindingV1) digestValue() (Digest, error) {
	copy := b
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-semantic-binding-v1", copy)
}

func (b ContextModelInputSegmentBindingV1) Validate() error {
	if b.FragmentRef.Validate() != nil || !validRegion(b.Region) || b.Position == 0 || !validFragmentKind(b.Kind) || !validTrust(b.Trust) || !validContextModelInputChannelV1(b.Channel) || !validContextModelInputRoleV1(b.Role) || !validContextModelInputEncodingV1(b.Encoding) || b.Digest.Validate() != nil {
		return fmt.Errorf("%w: model input segment binding", ErrInvalid)
	}
	switch b.Channel {
	case ContextModelInputInstructionV1:
		if b.Kind == FragmentToolCall || b.Kind == FragmentToolResult || b.Role != ContextModelInputRoleSystemV1 && b.Role != ContextModelInputRoleDeveloperV1 || b.CallID != "" || b.Name != "" {
			return fmt.Errorf("%w: instruction semantic binding", ErrConflict)
		}
	case ContextModelInputMessageV1:
		if b.Kind == FragmentToolCall || b.Kind == FragmentToolResult || b.Role != ContextModelInputRoleUserV1 && b.Role != ContextModelInputRoleAssistantV1 || b.CallID != "" || b.Name != "" {
			return fmt.Errorf("%w: input message semantic binding", ErrConflict)
		}
	case ContextModelInputFunctionCallV1:
		if b.Kind != FragmentToolCall || b.Role != ContextModelInputRoleAssistantV1 || validateID(b.CallID) != nil || validateID(b.Name) != nil {
			return fmt.Errorf("%w: function call semantic binding", ErrConflict)
		}
	case ContextModelInputFunctionResultV1:
		if b.Kind != FragmentToolResult || b.Role != ContextModelInputRoleToolV1 || validateID(b.CallID) != nil || validateID(b.Name) != nil {
			return fmt.Errorf("%w: function result semantic binding", ErrConflict)
		}
	case ContextModelInputReferenceV1:
		if b.Kind == FragmentToolCall || b.Kind == FragmentToolResult || b.CallID != "" || b.Name != "" {
			return fmt.Errorf("%w: reference semantic binding", ErrConflict)
		}
	}
	want, err := b.digestValue()
	if err != nil || want != b.Digest {
		return fmt.Errorf("%w: model input segment binding digest", ErrConflict)
	}
	return nil
}

func SealContextModelInputSegmentBindingV1(b ContextModelInputSegmentBindingV1) (ContextModelInputSegmentBindingV1, error) {
	b.Digest = ""
	digest, err := b.digestValue()
	if err != nil {
		return ContextModelInputSegmentBindingV1{}, err
	}
	b.Digest = digest
	return b, b.Validate()
}

type ContextModelInputSegmentV1 struct {
	FragmentRef           FactRef                     `json:"fragment_ref"`
	Region                FrameRegion                 `json:"region"`
	Position              uint32                      `json:"position"`
	Kind                  FragmentKind                `json:"kind"`
	Trust                 TrustClass                  `json:"trust"`
	Channel               ContextModelInputChannelV1  `json:"channel"`
	Role                  ContextModelInputRoleV1     `json:"role"`
	Encoding              ContextModelInputEncodingV1 `json:"encoding"`
	CallID                string                      `json:"call_id,omitempty"`
	Name                  string                      `json:"name,omitempty"`
	ContentRef            ContentRef                  `json:"content_ref"`
	Content               []byte                      `json:"content"`
	SemanticBindingDigest Digest                      `json:"semantic_binding_digest"`
}

func (s ContextModelInputSegmentV1) Validate() error {
	if s.FragmentRef.Validate() != nil || !validRegion(s.Region) || s.Position == 0 || !validFragmentKind(s.Kind) || !validTrust(s.Trust) || !validContextModelInputChannelV1(s.Channel) || !validContextModelInputRoleV1(s.Role) || !validContextModelInputEncodingV1(s.Encoding) || s.ContentRef.Validate() != nil || len(s.Content) == 0 || uint64(len(s.Content)) != s.ContentRef.Length || DigestBytes(s.Content) != s.ContentRef.Digest || s.SemanticBindingDigest.Validate() != nil {
		return fmt.Errorf("%w: model input segment", ErrInvalid)
	}
	binding := ContextModelInputSegmentBindingV1{
		FragmentRef: s.FragmentRef,
		Region:      s.Region,
		Position:    s.Position,
		Kind:        s.Kind,
		Trust:       s.Trust,
		Channel:     s.Channel,
		Role:        s.Role,
		Encoding:    s.Encoding,
		CallID:      s.CallID,
		Name:        s.Name,
		Digest:      s.SemanticBindingDigest,
	}
	if err := binding.Validate(); err != nil {
		return fmt.Errorf("%w: model input segment semantic binding", ErrConflict)
	}
	switch s.Encoding {
	case ContextModelInputUTF8V1:
		if !utf8.Valid(s.Content) {
			return fmt.Errorf("%w: model input utf8 content", ErrInvalid)
		}
	case ContextModelInputCanonicalJSONV1, ContextModelInputArtifactRefJSONV1:
		if !canonicalJSONBytesV1(s.Content) {
			return fmt.Errorf("%w: model input canonical json content", ErrInvalid)
		}
	}
	return nil
}

type ContextModelInputMaterialV1 struct {
	ContractVersion              string                                 `json:"contract_version"`
	Ref                          ContextModelInputMaterialRefV1         `json:"ref"`
	DescriptorRef                ContextFrameConsumptionDescriptorRefV1 `json:"descriptor_ref"`
	FrameRef                     FactRef                                `json:"frame_ref"`
	ManifestRef                  FactRef                                `json:"manifest_ref"`
	GenerationRef                FactRef                                `json:"generation_ref"`
	MaterializedDescriptorDigest Digest                                 `json:"materialized_descriptor_digest"`
	OrderedSegments              []ContextModelInputSegmentV1           `json:"ordered_segments"`
	CheckedUnixNano              int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano              int64                                  `json:"expires_unix_nano"`
	Digest                       Digest                                 `json:"digest"`
}

func (m ContextModelInputMaterialV1) digestValue() (Digest, error) {
	copy := m.Clone()
	copy.Ref.Digest = ""
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-material-v1", copy)
}

func (m ContextModelInputMaterialV1) Validate() error {
	if ValidateContract(m.ContractVersion) != nil || m.Ref.Validate() != nil || m.DescriptorRef.Validate() != nil || m.FrameRef.Validate() != nil || m.ManifestRef.Validate() != nil || m.GenerationRef.Validate() != nil || m.MaterializedDescriptorDigest.Validate() != nil || validateTimes(m.CheckedUnixNano, m.ExpiresUnixNano) != nil || m.Digest.Validate() != nil || m.Ref.Digest != m.Digest {
		return fmt.Errorf("%w: model input material", ErrInvalid)
	}
	if len(m.OrderedSegments) == 0 || len(m.OrderedSegments) > MaxContextModelInputSegmentsV1 {
		return fmt.Errorf("%w: model input segment cardinality", ErrInvalid)
	}
	seen := make(map[FactRef]struct{}, len(m.OrderedSegments))
	for index, segment := range m.OrderedSegments {
		if segment.Validate() != nil || segment.Position != uint32(index+1) {
			return fmt.Errorf("%w: model input segment order", ErrConflict)
		}
		if _, exists := seen[segment.FragmentRef]; exists {
			return fmt.Errorf("%w: duplicate model input fragment", ErrConflict)
		}
		seen[segment.FragmentRef] = struct{}{}
	}
	want, err := m.digestValue()
	if err != nil || want != m.Digest {
		return fmt.Errorf("%w: model input material digest", ErrConflict)
	}
	return nil
}

func SealContextModelInputMaterialV1(m ContextModelInputMaterialV1) (ContextModelInputMaterialV1, error) {
	m.ContractVersion = Version
	m.OrderedSegments = cloneContextModelInputSegmentsV1(m.OrderedSegments)
	m.Ref.Digest = ""
	m.Digest = ""
	digest, err := m.digestValue()
	if err != nil {
		return ContextModelInputMaterialV1{}, err
	}
	m.Ref.Digest = digest
	m.Digest = digest
	return m, m.Validate()
}

func (m ContextModelInputMaterialV1) Clone() ContextModelInputMaterialV1 {
	copy := m
	copy.OrderedSegments = cloneContextModelInputSegmentsV1(m.OrderedSegments)
	return copy
}

type ContextModelInputMaterialExactReaderV1 interface {
	ReadContextModelInputMaterialExactV1(context.Context, ContextModelInputMaterialRefV1, int64) (ContextModelInputMaterialV1, error)
}

type ContextModelInputMaterialCurrentReaderV1 interface {
	ReadContextModelInputMaterialCurrentV1(context.Context, string, int64) (ContextModelInputMaterialV1, error)
}

func cloneContextModelInputSegmentsV1(values []ContextModelInputSegmentV1) []ContextModelInputSegmentV1 {
	result := make([]ContextModelInputSegmentV1, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Content = append([]byte(nil), value.Content...)
	}
	return result
}

func validContextModelInputChannelV1(value ContextModelInputChannelV1) bool {
	switch value {
	case ContextModelInputInstructionV1, ContextModelInputMessageV1, ContextModelInputFunctionCallV1, ContextModelInputFunctionResultV1, ContextModelInputReferenceV1:
		return true
	default:
		return false
	}
}

func validContextModelInputRoleV1(value ContextModelInputRoleV1) bool {
	switch value {
	case ContextModelInputRoleSystemV1, ContextModelInputRoleDeveloperV1, ContextModelInputRoleUserV1, ContextModelInputRoleAssistantV1, ContextModelInputRoleToolV1:
		return true
	default:
		return false
	}
}

func validContextModelInputEncodingV1(value ContextModelInputEncodingV1) bool {
	return value == ContextModelInputUTF8V1 || value == ContextModelInputCanonicalJSONV1 || value == ContextModelInputArtifactRefJSONV1
}

func canonicalJSONBytesV1(payload []byte) bool {
	if err := rejectDuplicateJSONKeys(payload); err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return false
	}
	canonical, err := json.Marshal(value)
	return err == nil && bytes.Equal(payload, canonical)
}
