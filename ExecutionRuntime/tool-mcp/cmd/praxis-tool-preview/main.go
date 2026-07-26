package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/product"
)

const quickstartCheckContractVersionV1 = "praxis.tool-mcp.quickstart-check/v1"

type checkOutputV1 struct {
	ContractVersion  string      `json:"contract_version"`
	OK               bool        `json:"ok"`
	DeclarationCount int         `json:"declaration_count"`
	ModelNames       []string    `json:"model_names"`
	PreviewDigest    core.Digest `json:"preview_digest"`
	AssemblyDigest   core.Digest `json:"assembly_digest"`
	ReferenceOnly    bool        `json:"reference_only"`
	Executable       bool        `json:"executable"`
}

func main() { os.Exit(runV1(context.Background(), os.Args[1:], os.Stdout, os.Stderr, time.Now)) }

func runV1(ctx context.Context, args []string, stdout, stderr io.Writer, clock func() time.Time) int {
	flags := flag.NewFlagSet("praxis-tool-preview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	check := flags.Bool("check", false, "verify the exact five-declaration reference preview closure")
	configJSON := flags.String("config-json", "", "strict Core Pack Preview config JSON")
	if ctx == nil || stdout == nil || stderr == nil || clock == nil || flags.Parse(args) != nil || flags.NArg() != 0 || *configJSON == "" {
		writeErrorV1(stderr, "invalid quickstart arguments")
		return 2
	}
	config, err := api.DecodeCorePackPreviewConfigV1([]byte(*configJSON))
	if err != nil {
		writeErrorV1(stderr, err.Error())
		return 2
	}
	factory, err := product.NewReferencePreviewFactoryV1(clock)
	if err != nil {
		writeErrorV1(stderr, err.Error())
		return 1
	}
	bundle, err := factory.BuildV1(ctx)
	if err != nil {
		writeErrorV1(stderr, err.Error())
		return 1
	}
	var output bytes.Buffer
	if !*check {
		err = bundle.CLI.RunV1(ctx, []string{"tool", "core-pack", "preview", "--config-json", *configJSON}, &output)
	} else {
		var result api.CorePackPreviewResultV1
		result, err = bundle.Preview.PreviewV1(ctx, config)
		if err == nil {
			err = result.Validate()
		}
		if err == nil {
			want := []string{contract.CoreToolProcessExecV1, contract.CoreToolWorkspaceInspectV1, contract.CoreToolWorkspacePatchV1, contract.CoreToolWorkspaceReadV1, contract.CoreToolWorkspaceSearchV1}
			if len(result.Declarations) != len(want) {
				err = fmt.Errorf("expected five declarations")
			} else {
				for i := range want {
					if result.Declarations[i].Order != uint32(i) || result.Declarations[i].ModelName != want[i] {
						err = fmt.Errorf("declaration closure drifted")
						break
					}
				}
			}
			if err == nil {
				err = encodeV1(&output, checkOutputV1{ContractVersion: quickstartCheckContractVersionV1, OK: true, DeclarationCount: len(want), ModelNames: append([]string(nil), want...), PreviewDigest: result.Digest, AssemblyDigest: result.AssemblyDigest, ReferenceOnly: result.ReferenceOnly, Executable: result.Executable})
			}
		}
	}
	if err != nil {
		writeErrorV1(stderr, err.Error())
		return 1
	}
	if _, err = stdout.Write(output.Bytes()); err != nil {
		writeErrorV1(stderr, err.Error())
		return 1
	}
	return 0
}

func encodeV1(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeErrorV1(output io.Writer, message string) { _, _ = fmt.Fprintln(output, message) }
