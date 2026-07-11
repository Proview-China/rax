package catalog_test

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
)

func FuzzCatalogDecodeValidateAndDigest(f *testing.F) {
	var valid bytes.Buffer
	if err := catalog.Encode(&valid, catalog.DefaultDocument()); err != nil {
		f.Fatalf("encode seed catalog: %v", err)
	}
	f.Add(valid.Bytes())
	f.Add([]byte(`{"schema_version":"praxis.upstream-catalog/v1","schema_version":"duplicate","entries":[]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		document, err := catalog.Decode(bytes.NewReader(payload))
		if err != nil {
			return
		}
		_ = catalog.Validate(document, testNow)
		for _, entry := range document.Entries {
			_, _ = catalog.ComputeEvidenceDigest(entry)
		}
	})
}

func FuzzCatalogArtifactPaths(f *testing.F) {
	f.Add("provider/openai")
	f.Add("../escape")
	f.Add("/absolute")
	f.Add("tests/openai/provider_contract_test.go")
	f.Fuzz(func(t *testing.T, candidate string) {
		if len(candidate) > 4096 || strings.ContainsRune(candidate, 0) {
			return
		}
		document := catalog.DefaultDocument()
		document.Entries[0].Implementation.CodePaths = []string{candidate}
		_ = catalog.Validate(document, testNow)
		_ = catalog.ValidateArtifacts(fstest.MapFS{}, document)
	})
}
