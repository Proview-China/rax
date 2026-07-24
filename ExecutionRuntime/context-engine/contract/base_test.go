package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func TestDecodeStrictRejectsUnknownAndTrailingJSON(t *testing.T) {
	type payload struct {
		ID string `json:"id"`
	}
	if _, err := contract.DecodeStrict[payload]([]byte(`{"id":"ok","unknown":true}`)); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("unknown field err=%v", err)
	}
	if _, err := contract.DecodeStrict[payload]([]byte(`{"id":"ok"}{"id":"again"}`)); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("trailing json err=%v", err)
	}
	got, err := contract.DecodeStrict[payload]([]byte(`{"id":"ok"}`))
	if err != nil || got.ID != "ok" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestDecodeStrictRejectsRecursiveDuplicateKeys(t *testing.T) {
	type child struct {
		Name string `json:"name"`
	}
	type payload struct {
		ID       string  `json:"id"`
		Nested   child   `json:"nested"`
		Children []child `json:"children"`
	}
	cases := [][]byte{
		[]byte(`{"id":"one","id":"two","nested":{"name":"ok"},"children":[]}`),
		[]byte(`{"id":"one","nested":{"name":"one","name":"two"},"children":[]}`),
		[]byte(`{"id":"one","nested":{"name":"ok"},"children":[{"name":"one","name":"two"}]}`),
		[]byte(`{"id":"one","nested":{"name":"ok"},"children":[{"name":"one","\u006eame":"two"}]}`),
	}
	for _, input := range cases {
		if _, err := contract.DecodeStrict[payload](input); !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("duplicate key input accepted: %s err=%v", input, err)
		}
	}
}
