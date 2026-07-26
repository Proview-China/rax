package cli

import (
	"context"
	"flag"
	"io"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
)

func (r *RunnerV1) runCorePackPreviewV1(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("tool core-pack preview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configJSON := flags.String("config-json", "", "strict Core Pack Preview config JSON")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*configJSON) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "tool core-pack preview arguments are invalid")
	}
	config, err := api.DecodeCorePackPreviewConfigV1([]byte(*configJSON))
	if err != nil {
		return err
	}
	result, err := r.corePackPreview.PreviewV1(ctx, config)
	if err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	return writeJSONV1(output, result)
}
