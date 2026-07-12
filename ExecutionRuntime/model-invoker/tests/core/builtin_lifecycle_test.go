package core_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func TestBuiltinCandidatePostBuildCancellationClosesConstructedProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &receiptProvider{id: "test", endpoint: "https://api.example/v1"}
	cancel()
	_, err := adaptercore.FinalizeCandidateBinding(ctx, "test", modelinvoker.ProtocolResponses, provider.endpoint, provider, nil)
	if !errors.Is(err, context.Canceled) || modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled {
		t.Fatalf("FinalizeCandidateBinding cancellation = %v", err)
	}
	if provider.closes.Load() != 1 {
		t.Fatalf("provider Close calls = %d, want 1", provider.closes.Load())
	}
}

func TestBuiltinCandidateClosesEveryPostConstructionRejectionWithoutLeaking(t *testing.T) {
	const sentinel = "ROTATION-SECRET-MUST-NOT-LEAK"
	const identitySentinel = "FACTORY-ID-SECRET-MUST-NOT-LEAK"
	closeFailure := errors.New(sentinel)
	wrong := &receiptProvider{id: identitySentinel, endpoint: "https://api.example/v1", closeErr: closeFailure}
	missingReceipt := &noReceiptProvider{id: "test", closeErr: closeFailure}
	invalidReceipt := &receiptProvider{id: "test", endpoint: "https://api.example/v1", rejectReceipt: true, closeErr: closeFailure}
	failedBuild := &receiptProvider{id: "test", endpoint: "https://api.example/v1", closeErr: closeFailure}
	tests := []struct {
		name       string
		provider   modelinvoker.Provider
		buildErr   error
		wantCloses *atomic.Int64
	}{
		{"wrong provider identity", wrong, nil, &wrong.closes},
		{"missing receipt interface", missingReceipt, nil, &missingReceipt.closes},
		{"invalid receipt", invalidReceipt, nil, &invalidReceipt.closes},
		{"provider plus build error", failedBuild, errors.New("build failure"), &failedBuild.closes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := adaptercore.FinalizeCandidateBinding(context.Background(), "test", modelinvoker.ProtocolResponses, "https://api.example/v1", test.provider, test.buildErr)
			if err == nil {
				t.Fatal("FinalizeCandidateBinding error = nil, want rejection joined with close failure")
			}
			if !errors.Is(err, closeFailure) {
				t.Fatalf("errors.Is(candidate error, close failure) = false: %v", err)
			}
			if test.wantCloses.Load() != 1 {
				t.Fatalf("Close calls = %d, want 1", test.wantCloses.Load())
			}
			if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), identitySentinel) {
				t.Fatalf("candidate error leaked close or identity sentinel: %v", err)
			}
		})
	}
}

type receiptProvider struct {
	id            modelinvoker.ProviderID
	endpoint      string
	rejectReceipt bool
	closeErr      error
	closes        atomic.Int64
}

func (provider *receiptProvider) ID() modelinvoker.ProviderID { return provider.id }
func (*receiptProvider) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolResponses
}
func (*receiptProvider) Capabilities(context.Context, modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	return nil, nil
}
func (*receiptProvider) Invoke(context.Context, modelinvoker.Request) (modelinvoker.Response, error) {
	return modelinvoker.Response{}, nil
}
func (*receiptProvider) Stream(context.Context, modelinvoker.Request) (modelinvoker.Stream, error) {
	return nil, nil
}
func (provider *receiptProvider) CandidateBindingEndpoint(modelinvoker.Protocol, string) (string, bool) {
	return provider.endpoint, !provider.rejectReceipt && provider.endpoint != ""
}
func (provider *receiptProvider) Close() error {
	provider.closes.Add(1)
	return provider.closeErr
}

type noReceiptProvider struct {
	id       modelinvoker.ProviderID
	closeErr error
	closes   atomic.Int64
}

func (provider *noReceiptProvider) ID() modelinvoker.ProviderID { return provider.id }
func (*noReceiptProvider) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolResponses
}
func (*noReceiptProvider) Capabilities(context.Context, modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	return nil, nil
}
func (*noReceiptProvider) Invoke(context.Context, modelinvoker.Request) (modelinvoker.Response, error) {
	return modelinvoker.Response{}, nil
}
func (*noReceiptProvider) Stream(context.Context, modelinvoker.Request) (modelinvoker.Stream, error) {
	return nil, nil
}
func (provider *noReceiptProvider) Close() error {
	provider.closes.Add(1)
	return provider.closeErr
}
