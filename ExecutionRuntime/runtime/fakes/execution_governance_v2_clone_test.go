package fakes

import (
	"bytes"
	"testing"
)

func TestCloneBytesPreservingNilV1PreservesNilAndEmptyIdentity(t *testing.T) {
	if cloned := cloneBytesPreservingNilV1(nil); cloned != nil {
		t.Fatal("nil payload became a present empty payload")
	}

	empty := []byte{}
	clonedEmpty := cloneBytesPreservingNilV1(empty)
	if clonedEmpty == nil || len(clonedEmpty) != 0 {
		t.Fatal("present empty payload became absent")
	}

	source := []byte("payload")
	cloned := cloneBytesPreservingNilV1(source)
	if !bytes.Equal(cloned, source) {
		t.Fatal("payload bytes drifted during clone")
	}
	cloned[0] = 'P'
	if bytes.Equal(cloned, source) {
		t.Fatal("payload clone aliases its source")
	}
}
