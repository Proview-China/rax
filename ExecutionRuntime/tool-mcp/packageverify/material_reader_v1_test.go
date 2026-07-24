package packageverify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type exactReadFaultStreamV1 struct {
	*bytes.Reader
	closeErr error
}

func (s *exactReadFaultStreamV1) Close() error { return s.closeErr }

type exactReadFaultArtifactV1 struct {
	data     []byte
	openErr  error
	closeErr error
}

func (r *exactReadFaultArtifactV1) OpenExactSupplyChainArtifactV1(ctx context.Context, _ runtimeports.SupplyChainArtifactContentRefV1) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.openErr != nil {
		return nil, r.openErr
	}
	return &exactReadFaultStreamV1{Reader: bytes.NewReader(append([]byte(nil), r.data...)), closeErr: r.closeErr}, nil
}

func TestReadExactArtifactV1ValidatesSizeDigestAndDeepCopies(t *testing.T) {
	data := []byte("exact-package-artifact")
	ref := artifactRefV1("application/octet-stream", data)
	reader := &exactReadFaultArtifactV1{data: data}
	got, err := ReadExactArtifactV1(context.Background(), reader, ref, uint64(len(data)))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read=%q err=%v", got, err)
	}
	got[0] ^= 0xff
	again, err := ReadExactArtifactV1(context.Background(), reader, ref, uint64(len(data)))
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("exact read exposed mutable bytes: %q err=%v", again, err)
	}
}

func TestReadExactArtifactV1FailsClosedOnShortExtraDigestAndClose(t *testing.T) {
	data := []byte("exact-package-artifact")
	ref := artifactRefV1("application/octet-stream", data)
	cases := []struct {
		name   string
		reader *exactReadFaultArtifactV1
		ref    runtimeports.SupplyChainArtifactContentRefV1
	}{
		{name: "short", reader: &exactReadFaultArtifactV1{data: data[:len(data)-1]}, ref: ref},
		{name: "extra", reader: &exactReadFaultArtifactV1{data: append(append([]byte(nil), data...), 'x')}, ref: ref},
		{name: "digest", reader: &exactReadFaultArtifactV1{data: append([]byte(nil), data...)}, ref: func() runtimeports.SupplyChainArtifactContentRefV1 {
			value := ref
			value.Digest = core.DigestBytes([]byte("other"))
			return value
		}()},
		{name: "close", reader: &exactReadFaultArtifactV1{data: data, closeErr: errors.New("close")}, ref: ref},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ReadExactArtifactV1(context.Background(), test.reader, test.ref, uint64(len(data)+1)); err == nil {
				t.Fatal("faulty exact stream was accepted")
			}
		})
	}
}

func TestReadExactArtifactV1RejectsTypedNilNilAndCanceledBeforeOpen(t *testing.T) {
	var typedNil *exactReadFaultArtifactV1
	ref := artifactRefV1("application/octet-stream", []byte("x"))
	if _, err := ReadExactArtifactV1(context.Background(), typedNil, ref, 1); err == nil {
		t.Fatal("typed-nil exact reader was accepted")
	}
	if _, err := ReadExactArtifactV1(nil, &exactReadFaultArtifactV1{data: []byte("x")}, ref, 1); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadExactArtifactV1(ctx, &exactReadFaultArtifactV1{data: []byte("x")}, ref, 1); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}
