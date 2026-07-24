package contract

import "fmt"

type ArtifactRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

func (r ArtifactRange) Validate() error {
	if r.End <= r.Start {
		return fmt.Errorf("%w: artifact range", ErrInvalid)
	}
	return nil
}

type ArtifactAnchor struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"anchor_id"`
	Revision        uint64        `json:"revision"`
	ArtifactOwner   OwnerRef      `json:"artifact_owner"`
	ArtifactRef     string        `json:"artifact_ref"`
	ArtifactVersion string        `json:"artifact_version"`
	ArtifactDigest  Digest        `json:"artifact_digest"`
	Range           ArtifactRange `json:"range"`
	FrameRef        FactRef       `json:"frame_ref"`
	GenerationID    string        `json:"generation_id"`
	Evidence        EvidenceRef   `json:"evidence"`
	CreatedUnixNano int64         `json:"created_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (a ArtifactAnchor) Validate() error {
	if ValidateContract(a.ContractVersion) != nil || validateID(a.ID) != nil || a.Revision != 1 || a.ArtifactOwner.Validate() != nil || validateID(a.ArtifactRef) != nil || validateID(a.ArtifactVersion) != nil || a.ArtifactDigest.Validate() != nil || a.Range.Validate() != nil || a.FrameRef.Validate() != nil || validateID(a.GenerationID) != nil || a.Evidence.Validate() != nil || validateTimes(a.CreatedUnixNano, a.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: artifact anchor", ErrInvalid)
	}
	return nil
}

func (a ArtifactAnchor) DigestValue() (Digest, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(a)
}

type ArtifactDelta struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"delta_id"`
	Revision        uint64        `json:"revision"`
	BaseAnchor      FactRef       `json:"base_anchor"`
	BaseDigest      Digest        `json:"base_digest"`
	TargetVersion   string        `json:"target_version"`
	TargetDigest    Digest        `json:"target_digest"`
	Range           ArtifactRange `json:"range"`
	Delta           ContentRef    `json:"delta"`
	ChainDepth      uint32        `json:"chain_depth"`
	Evidence        EvidenceRef   `json:"evidence"`
}

func (d ArtifactDelta) Validate() error {
	if ValidateContract(d.ContractVersion) != nil || validateID(d.ID) != nil || d.Revision != 1 || d.BaseAnchor.Validate() != nil || d.BaseDigest.Validate() != nil || validateID(d.TargetVersion) != nil || d.TargetDigest.Validate() != nil || d.Range.Validate() != nil || d.Delta.Validate() != nil || d.ChainDepth == 0 || d.Evidence.Validate() != nil {
		return fmt.Errorf("%w: artifact delta", ErrInvalid)
	}
	return nil
}

func (d ArtifactDelta) DigestValue() (Digest, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(d)
}

type ArtifactReadMode string

const (
	ArtifactUnchanged     ArtifactReadMode = "unchanged"
	ArtifactUseDelta      ArtifactReadMode = "delta"
	ArtifactRematerialize ArtifactReadMode = "rematerialize"
)

func PlanArtifactRead(anchor ArtifactAnchor, currentVersion string, currentDigest Digest, requested ArtifactRange, maxChain uint32, delta *ArtifactDelta, now int64) (ArtifactReadMode, error) {
	if anchor.Validate() != nil || validateID(currentVersion) != nil || currentDigest.Validate() != nil || requested.Validate() != nil || maxChain == 0 || now <= 0 {
		return "", fmt.Errorf("%w: artifact read request", ErrInvalid)
	}
	if now >= anchor.ExpiresUnixNano {
		return ArtifactRematerialize, nil
	}
	if anchor.ArtifactVersion == currentVersion && anchor.ArtifactDigest == currentDigest && anchor.Range == requested {
		return ArtifactUnchanged, nil
	}
	anchorDigest, err := anchor.DigestValue()
	if err != nil {
		return "", err
	}
	if delta != nil && delta.Validate() == nil && delta.BaseAnchor.ID == anchor.ID && delta.BaseAnchor.Revision == anchor.Revision && delta.BaseAnchor.Digest == anchorDigest && delta.BaseDigest == anchor.ArtifactDigest && delta.TargetVersion == currentVersion && delta.TargetDigest == currentDigest && delta.Range == requested && delta.ChainDepth <= maxChain {
		return ArtifactUseDelta, nil
	}
	return ArtifactRematerialize, nil
}

func PlanArtifactReadAfterCompaction(anchor ArtifactAnchor, generation ContextGeneration, currentVersion string, currentDigest Digest, requested ArtifactRange, maxChain uint32, delta *ArtifactDelta, now int64) (ArtifactReadMode, error) {
	if err := anchor.Validate(); err != nil {
		return "", err
	}
	if err := generation.Validate(); err != nil {
		return "", err
	}
	anchorDigest, err := anchor.DigestValue()
	if err != nil {
		return "", err
	}
	retained := false
	for _, ref := range generation.RetainedAnchors {
		if ref.ID == anchor.ID && ref.Revision == anchor.Revision && ref.Digest == anchorDigest {
			retained = true
			break
		}
	}
	if !retained {
		return ArtifactRematerialize, nil
	}
	return PlanArtifactRead(anchor, currentVersion, currentDigest, requested, maxChain, delta, now)
}
