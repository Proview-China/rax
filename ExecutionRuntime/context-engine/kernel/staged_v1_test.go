package kernel

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestStagedRendererIsByteExactWithLegacyV1(t *testing.T) {
	fixtures := [][]byte{
		[]byte("ascii"), []byte("你好"), {0, 1, 2, 0xff},
		bytes.Repeat([]byte{0x5a}, renderRawChunkBytesV1-1),
		bytes.Repeat([]byte{0x5a}, renderRawChunkBytesV1),
		bytes.Repeat([]byte{0x5a}, renderRawChunkBytesV1+1),
	}
	for _, content := range fixtures {
		regions := map[contract.FrameRegion][]renderedFragment{
			contract.RegionStablePrefix: {{Position: 1, Kind: contract.FragmentInstruction, CandidateDigest: testkit.D("candidate"), Content: content}},
			contract.RegionSemiStable:   {},
			contract.RegionDynamicTail:  {{Position: 2, Kind: contract.FragmentConversation, CandidateDigest: testkit.D("tail"), Content: content}},
		}
		wantStable, wantSemi, wantDynamic, wantRendered, err := renderRegions(regions)
		if err != nil {
			t.Fatal(err)
		}
		gotStable, gotSemi, gotDynamic, gotRendered, err := renderRegionsContextV1(context.Background(), regions, 4*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wantStable, gotStable) || !bytes.Equal(wantSemi, gotSemi) || !bytes.Equal(wantDynamic, gotDynamic) || !bytes.Equal(wantRendered, gotRendered) {
			t.Fatalf("staged renderer drifted for %d bytes", len(content))
		}
	}
}

func TestStagedRendererFourMiBGoldenAndExactLimitV1(t *testing.T) {
	if os.Getenv("PRAXIS_CONTEXT_MAX_SIZE") != "1" {
		t.Skip("covered by the bounded max-size evidence run")
	}
	regions := map[contract.FrameRegion][]renderedFragment{
		contract.RegionStablePrefix: {{Position: 1, Kind: contract.FragmentInstruction, CandidateDigest: testkit.D("candidate-4mib"), Content: bytes.Repeat([]byte{0x5a}, 4*1024*1024)}},
		contract.RegionSemiStable:   {}, contract.RegionDynamicTail: {},
	}
	wantStable, wantSemi, wantDynamic, wantRendered, err := renderRegions(regions)
	if err != nil {
		t.Fatal(err)
	}
	gotStable, gotSemi, gotDynamic, gotRendered, err := renderRegionsContextV1(context.Background(), regions, uint64(len(wantRendered)))
	if err != nil || !bytes.Equal(wantStable, gotStable) || !bytes.Equal(wantSemi, gotSemi) || !bytes.Equal(wantDynamic, gotDynamic) || !bytes.Equal(wantRendered, gotRendered) {
		t.Fatalf("4 MiB staged renderer drift: %v", err)
	}
	stable, semi, dynamic, rendered, err := renderRegionsContextV1(context.Background(), regions, uint64(len(wantRendered)-1))
	if !errors.Is(err, contract.ErrLimitExceeded) || stable != nil || semi != nil || dynamic != nil || rendered != nil {
		t.Fatalf("renderer exact-1 did not fail closed: %d/%d/%d/%d %v", len(stable), len(semi), len(dynamic), len(rendered), err)
	}
}

func TestStagedRendererCancellationAndLimitFailClosedV1(t *testing.T) {
	regions := map[contract.FrameRegion][]renderedFragment{
		contract.RegionStablePrefix: {{Position: 1, Kind: contract.FragmentInstruction, CandidateDigest: testkit.D("candidate"), Content: bytes.Repeat([]byte("x"), 256*1024)}},
		contract.RegionSemiStable:   {}, contract.RegionDynamicTail: {},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stable, semi, dynamic, rendered, err := renderRegionsContextV1(ctx, regions, 4*1024*1024)
	if !errors.Is(err, context.Canceled) || stable != nil || semi != nil || dynamic != nil || rendered != nil {
		t.Fatalf("cancel returned partial render: %d %d %d %d %v", len(stable), len(semi), len(dynamic), len(rendered), err)
	}
	stable, semi, dynamic, rendered, err = renderRegionsContextV1(context.Background(), regions, 64)
	if !errors.Is(err, contract.ErrLimitExceeded) || stable != nil || semi != nil || dynamic != nil || rendered != nil {
		t.Fatalf("limit returned partial render: %d %d %d %d %v", len(stable), len(semi), len(dynamic), len(rendered), err)
	}
}
