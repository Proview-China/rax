package dataplaneadapter

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameRoundTripAndUnknownFieldRejection(t *testing.T) {
	input := struct {
		Value string `json:"value"`
	}{Value: "ok"}
	var frame bytes.Buffer
	if err := writeFrame(&frame, input); err != nil {
		t.Fatal(err)
	}
	var output struct {
		Value string `json:"value"`
	}
	if err := readFrame(&frame, &output); err != nil {
		t.Fatal(err)
	}
	if output.Value != input.Value {
		t.Fatal("frame changed payload")
	}

	data := []byte(`{"value":"ok","unknown":true}`)
	frame.Reset()
	if err := binary.Write(&frame, binary.BigEndian, uint32(len(data))); err != nil {
		t.Fatal(err)
	}
	frame.Write(data)
	if err := readFrame(&frame, &output); err == nil {
		t.Fatal("unknown IPC field was accepted")
	}
}

func TestFrameSizeFailsClosed(t *testing.T) {
	var frame bytes.Buffer
	if err := binary.Write(&frame, binary.BigEndian, uint32(maxFrameBytes+1)); err != nil {
		t.Fatal(err)
	}
	var output any
	if err := readFrame(&frame, &output); err == nil {
		t.Fatal("oversized frame was accepted")
	}
}
