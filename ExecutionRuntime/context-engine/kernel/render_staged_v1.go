package kernel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const renderRawChunkBytesV1 = 48 * 1024

func renderRegionsContextV1(ctx context.Context, regions map[contract.FrameRegion][]renderedFragment, maxGenerated uint64) ([]byte, []byte, []byte, []byte, error) {
	if err := checkContextV1(ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	stable, err := renderRegionContextV1(ctx, regions[contract.RegionStablePrefix], maxGenerated)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	semi, err := renderRegionContextV1(ctx, regions[contract.RegionSemiStable], maxGenerated)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	dynamic, err := renderRegionContextV1(ctx, regions[contract.RegionDynamicTail], maxGenerated)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	buffer := &boundedContextBufferV1{ctx: ctx, max: maxGenerated}
	for _, part := range [][]byte{[]byte(`{"stable_prefix":`), stable, []byte(`,"semi_stable":`), semi, []byte(`,"dynamic_tail":`), dynamic, []byte(`}`)} {
		if _, err := buffer.Write(part); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	rendered, err := cloneKernelBytesContextV1(ctx, buffer.Bytes())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return stable, semi, dynamic, rendered, nil
}

func renderRegionContextV1(ctx context.Context, fragments []renderedFragment, max uint64) ([]byte, error) {
	buffer := &boundedContextBufferV1{ctx: ctx, max: max}
	if _, err := buffer.Write([]byte{'['}); err != nil {
		return nil, err
	}
	for index, fragment := range fragments {
		if err := checkContextV1(ctx); err != nil {
			return nil, err
		}
		if index > 0 {
			if _, err := buffer.Write([]byte{','}); err != nil {
				return nil, err
			}
		}
		kind, _ := json.Marshal(fragment.Kind)
		digest, _ := json.Marshal(fragment.CandidateDigest)
		header := append([]byte(`{"position":`), strconv.AppendUint(nil, uint64(fragment.Position), 10)...)
		header = append(header, []byte(`,"kind":`)...)
		header = append(header, kind...)
		header = append(header, []byte(`,"candidate_digest":`)...)
		header = append(header, digest...)
		header = append(header, []byte(`,"content":"`)...)
		if _, err := buffer.Write(header); err != nil {
			return nil, err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, buffer)
		for offset := 0; offset < len(fragment.Content); offset += renderRawChunkBytesV1 {
			if err := checkContextV1(ctx); err != nil {
				_ = encoder.Close()
				return nil, err
			}
			end := offset + renderRawChunkBytesV1
			if end > len(fragment.Content) {
				end = len(fragment.Content)
			}
			if _, err := encoder.Write(fragment.Content[offset:end]); err != nil {
				return nil, err
			}
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
		if _, err := buffer.Write([]byte(`"}`)); err != nil {
			return nil, err
		}
	}
	if _, err := buffer.Write([]byte{']'}); err != nil {
		return nil, err
	}
	return cloneKernelBytesContextV1(ctx, buffer.Bytes())
}

type boundedContextBufferV1 struct {
	ctx context.Context
	max uint64
	buf bytes.Buffer
}

func (b *boundedContextBufferV1) Write(value []byte) (int, error) {
	written := 0
	for offset := 0; offset < len(value); offset += StagedStreamChunkBytesV1 {
		if err := checkContextV1(b.ctx); err != nil {
			return written, err
		}
		end := offset + StagedStreamChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		chunk := value[offset:end]
		if uint64(len(chunk)) > b.max || uint64(b.buf.Len()) > b.max-uint64(len(chunk)) {
			return written, fmt.Errorf("%w: staged render", contract.ErrLimitExceeded)
		}
		n, err := b.buf.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, checkContextV1(b.ctx)
}

func (b *boundedContextBufferV1) Bytes() []byte { return b.buf.Bytes() }

func cloneKernelBytesContextV1(ctx context.Context, value []byte) ([]byte, error) {
	result := make([]byte, len(value))
	for offset := 0; offset < len(value); offset += StagedCloneChunkBytesV1 {
		if err := checkContextV1(ctx); err != nil {
			return nil, err
		}
		end := offset + StagedCloneChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		copy(result[offset:end], value[offset:end])
	}
	return result, checkContextV1(ctx)
}
