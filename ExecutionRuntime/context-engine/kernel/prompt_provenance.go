package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const promptProvenanceHashChunkV1 = 64 * 1024

type promptArtifactDigestRecordV1 struct {
	ArtifactID string          `json:"artifact_id"`
	Length     uint64          `json:"length"`
	Digest     contract.Digest `json:"digest"`
}

type promptGeneratedDigestRecordV1 struct {
	Ref    contract.ContentRef `json:"ref"`
	Digest contract.Digest     `json:"bytes_digest"`
}

type promptVerifyRequestDigestBodyV1 struct {
	Domain           string                          `json:"domain"`
	Version          string                          `json:"version"`
	ProvenanceDigest contract.Digest                 `json:"provenance_digest"`
	Artifacts        []promptArtifactDigestRecordV1  `json:"artifacts"`
	LicenseLength    uint64                          `json:"license_length"`
	LicenseDigest    contract.Digest                 `json:"license_digest"`
	Generated        []promptGeneratedDigestRecordV1 `json:"generated"`
	CheckedUnixNano  int64                           `json:"checked_unix_nano"`
	MaxInputBytes    uint64                          `json:"max_input_bytes"`
}

func SealVerifyPromptUpstreamProvenanceRequestV1(ctx context.Context, request contract.VerifyPromptUpstreamProvenanceRequestV1) (contract.VerifyPromptUpstreamProvenanceRequestV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	if err := request.Provenance.Validate(); err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	if err := preflightPromptProvenanceBytesV1(request.ArtifactBytes, request.LicenseBytes, request.GeneratedBytes, request.MaxInputBytes); err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	sealed := request
	sealed.ArtifactBytes = make([]contract.PromptUpstreamArtifactBytesV1, len(request.ArtifactBytes))
	for index, item := range request.ArtifactBytes {
		if err := checkContextV1(ctx); err != nil {
			return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
		}
		sealed.ArtifactBytes[index] = contract.PromptUpstreamArtifactBytesV1{ArtifactID: item.ArtifactID}
		bytes, err := clonePromptBytesV1(ctx, item.Bytes)
		if err != nil {
			return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
		}
		sealed.ArtifactBytes[index].Bytes = bytes
	}
	sort.Slice(sealed.ArtifactBytes, func(i, j int) bool { return sealed.ArtifactBytes[i].ArtifactID < sealed.ArtifactBytes[j].ArtifactID })
	licenseBytes, err := clonePromptBytesV1(ctx, request.LicenseBytes)
	if err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	sealed.LicenseBytes = licenseBytes
	sealed.GeneratedBytes = make([]contract.PromptGeneratedContentBytesV1, len(request.GeneratedBytes))
	for index, item := range request.GeneratedBytes {
		if err := checkContextV1(ctx); err != nil {
			return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
		}
		sealed.GeneratedBytes[index] = contract.PromptGeneratedContentBytesV1{Ref: item.Ref}
		bytes, err := clonePromptBytesV1(ctx, item.Bytes)
		if err != nil {
			return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
		}
		sealed.GeneratedBytes[index].Bytes = bytes
	}
	sort.Slice(sealed.GeneratedBytes, func(i, j int) bool {
		left, right := sealed.GeneratedBytes[i].Ref, sealed.GeneratedBytes[j].Ref
		if left.Ref != right.Ref {
			return left.Ref < right.Ref
		}
		if left.Length != right.Length {
			return left.Length < right.Length
		}
		return left.Digest < right.Digest
	})
	sealed.RequestDigest = ""
	digest, err := promptVerifyRequestDigestV1(ctx, sealed)
	if err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	sealed.RequestDigest = digest
	if err := sealed.ValidateShapeV1(); err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	if err := checkContextV1(ctx); err != nil {
		return contract.VerifyPromptUpstreamProvenanceRequestV1{}, err
	}
	return sealed, nil
}

func VerifyPromptUpstreamProvenanceV1(ctx context.Context, request contract.VerifyPromptUpstreamProvenanceRequestV1) (contract.PromptUpstreamVerificationReportV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if err := preflightPromptProvenanceBytesV1(request.ArtifactBytes, request.LicenseBytes, request.GeneratedBytes, request.MaxInputBytes); err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if err := request.ValidateShapeV1(); err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if request.CheckedUnixNano < request.Provenance.CreatedUnixNano || request.CheckedUnixNano >= request.Provenance.ExpiresUnixNano {
		return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt provenance verification window", contract.ErrExpired)
	}
	wantRequestDigest, err := promptVerifyRequestDigestV1(ctx, request)
	if err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if wantRequestDigest != request.RequestDigest {
		return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt provenance request digest", contract.ErrConflict)
	}
	if uint64(len(request.LicenseBytes)) != request.Provenance.License.ByteLength {
		return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt license length", contract.ErrConflict)
	}
	licenseDigest, err := digestPromptBytesV1(ctx, request.LicenseBytes)
	if err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if licenseDigest != request.Provenance.License.ContentDigest {
		return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt license digest", contract.ErrConflict)
	}
	artifactIDs := make([]string, 0, len(request.Provenance.Artifacts))
	for index, artifact := range request.Provenance.Artifacts {
		if err := checkContextV1(ctx); err != nil {
			return contract.PromptUpstreamVerificationReportV1{}, err
		}
		material := request.ArtifactBytes[index]
		if material.ArtifactID != artifact.ID {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt artifact binding", contract.ErrNotFound)
		}
		if uint64(len(material.Bytes)) != artifact.ByteLength {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt artifact length", contract.ErrConflict)
		}
		digest, err := digestPromptBytesV1(ctx, material.Bytes)
		if err != nil {
			return contract.PromptUpstreamVerificationReportV1{}, err
		}
		if digest != artifact.ContentDigest {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt artifact digest", contract.ErrConflict)
		}
		for _, extracted := range artifact.ExtractedRanges {
			if err := checkContextV1(ctx); err != nil {
				return contract.PromptUpstreamVerificationReportV1{}, err
			}
			rangeDigest, err := digestPromptBytesV1(ctx, material.Bytes[extracted.Start:extracted.End])
			if err != nil {
				return contract.PromptUpstreamVerificationReportV1{}, err
			}
			if rangeDigest != extracted.Digest {
				return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt extracted range digest", contract.ErrConflict)
			}
		}
		artifactIDs = append(artifactIDs, artifact.ID)
	}
	verifiedContent := make([]contract.ContentRef, 0, len(request.Provenance.GeneratedContent))
	for index, expected := range request.Provenance.GeneratedContent {
		if err := checkContextV1(ctx); err != nil {
			return contract.PromptUpstreamVerificationReportV1{}, err
		}
		material := request.GeneratedBytes[index]
		if material.Ref != expected {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt generated content binding", contract.ErrNotFound)
		}
		if uint64(len(material.Bytes)) != expected.Length || len(material.Bytes) == 0 {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt generated content length", contract.ErrConflict)
		}
		digest, err := digestPromptBytesV1(ctx, material.Bytes)
		if err != nil {
			return contract.PromptUpstreamVerificationReportV1{}, err
		}
		if digest != expected.Digest {
			return contract.PromptUpstreamVerificationReportV1{}, fmt.Errorf("%w: prompt generated content digest", contract.ErrConflict)
		}
		verifiedContent = append(verifiedContent, expected)
	}
	if err := checkContextV1(ctx); err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	provenanceRef, err := request.Provenance.RefV1()
	if err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	report, err := contract.SealPromptUpstreamVerificationReportV1(contract.PromptUpstreamVerificationReportV1{
		ProvenanceRef: provenanceRef, SourceSetDigest: request.Provenance.SourceSetDigest,
		GeneratedSetDigest: request.Provenance.GeneratedSetDigest, ClosureDigest: request.Provenance.Closure.ClosureDigest,
		VerifiedArtifactIDs: artifactIDs, VerifiedContentRefs: verifiedContent,
		CheckedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: request.Provenance.ExpiresUnixNano,
	})
	if err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	if err := checkContextV1(ctx); err != nil {
		return contract.PromptUpstreamVerificationReportV1{}, err
	}
	return report, nil
}

func preflightPromptProvenanceBytesV1(artifacts []contract.PromptUpstreamArtifactBytesV1, license []byte, generated []contract.PromptGeneratedContentBytesV1, max uint64) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("%w: prompt provenance requires artifact bytes", contract.ErrInvalid)
	}
	if max == 0 || max > contract.MaxPromptUpstreamInputBytesV1 || len(artifacts) > contract.MaxPromptUpstreamArtifactsV1 || len(generated) > contract.MaxPromptGeneratedContentV1 {
		return fmt.Errorf("%w: prompt provenance input bounds", contract.ErrLimitExceeded)
	}
	total := uint64(len(license))
	for _, item := range artifacts {
		if uint64(len(item.Bytes)) > math.MaxUint64-total {
			return fmt.Errorf("%w: prompt provenance input overflow", contract.ErrLimitExceeded)
		}
		total += uint64(len(item.Bytes))
	}
	for _, item := range generated {
		if uint64(len(item.Bytes)) > math.MaxUint64-total {
			return fmt.Errorf("%w: prompt provenance input overflow", contract.ErrLimitExceeded)
		}
		total += uint64(len(item.Bytes))
	}
	if total > max {
		return fmt.Errorf("%w: prompt provenance input bytes", contract.ErrLimitExceeded)
	}
	return nil
}

func promptVerifyRequestDigestV1(ctx context.Context, request contract.VerifyPromptUpstreamProvenanceRequestV1) (contract.Digest, error) {
	artifacts := make([]promptArtifactDigestRecordV1, 0, len(request.ArtifactBytes))
	for _, item := range request.ArtifactBytes {
		digest, err := digestPromptBytesV1(ctx, item.Bytes)
		if err != nil {
			return "", err
		}
		artifacts = append(artifacts, promptArtifactDigestRecordV1{ArtifactID: item.ArtifactID, Length: uint64(len(item.Bytes)), Digest: digest})
	}
	generated := make([]promptGeneratedDigestRecordV1, 0, len(request.GeneratedBytes))
	for _, item := range request.GeneratedBytes {
		digest, err := digestPromptBytesV1(ctx, item.Bytes)
		if err != nil {
			return "", err
		}
		generated = append(generated, promptGeneratedDigestRecordV1{Ref: item.Ref, Digest: digest})
	}
	licenseDigest, err := digestPromptBytesV1(ctx, request.LicenseBytes)
	if err != nil {
		return "", err
	}
	if err := checkContextV1(ctx); err != nil {
		return "", err
	}
	return contract.DigestJSON(promptVerifyRequestDigestBodyV1{
		Domain: "praxis.context.verify-prompt-upstream-provenance-request", Version: "v1",
		ProvenanceDigest: request.Provenance.ProvenanceDigest, Artifacts: artifacts,
		LicenseLength: uint64(len(request.LicenseBytes)), LicenseDigest: licenseDigest, Generated: generated,
		CheckedUnixNano: request.CheckedUnixNano, MaxInputBytes: request.MaxInputBytes,
	})
}

func digestPromptBytesV1(ctx context.Context, value []byte) (contract.Digest, error) {
	hash := sha256.New()
	for offset := 0; offset < len(value); offset += promptProvenanceHashChunkV1 {
		if err := checkContextV1(ctx); err != nil {
			return "", err
		}
		end := offset + promptProvenanceHashChunkV1
		if end > len(value) {
			end = len(value)
		}
		_, _ = hash.Write(value[offset:end])
	}
	if err := checkContextV1(ctx); err != nil {
		return "", err
	}
	return contract.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}

func clonePromptBytesV1(ctx context.Context, value []byte) ([]byte, error) {
	result := make([]byte, len(value))
	for offset := 0; offset < len(value); offset += promptProvenanceHashChunkV1 {
		if err := checkContextV1(ctx); err != nil {
			return nil, err
		}
		end := offset + promptProvenanceHashChunkV1
		if end > len(value) {
			end = len(value)
		}
		copy(result[offset:end], value[offset:end])
	}
	return result, checkContextV1(ctx)
}
