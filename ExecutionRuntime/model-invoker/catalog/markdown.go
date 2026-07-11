package catalog

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	CurrentBindingsStartMarker = "<!-- BEGIN GENERATED: praxis-model-invoker-current-bindings -->"
	CurrentBindingsEndMarker   = "<!-- END GENERATED: praxis-model-invoker-current-bindings -->"
)

// RenderCurrentBindingsMarkdown renders only the callable bindings represented
// by the supplied catalog. The broader research matrix remains authored and
// reviewed separately.
func RenderCurrentBindingsMarkdown(document Document, now time.Time) ([]byte, error) {
	snapshot, err := New(document, now)
	if err != nil {
		return nil, fmt.Errorf("render current catalog bindings: %w", err)
	}

	var builder strings.Builder
	builder.WriteString(CurrentBindingsStartMarker)
	builder.WriteString("\n")
	builder.WriteString("| Route ID | Provider | Runtime Adapter ID | Offering | Deployment | Protocol | Endpoint | Credential Profile | Evidence | Praxis状态 |\n")
	builder.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	callable := 0
	for _, entry := range snapshot.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		callable++
		endpoint := entry.Route.Endpoint.Scheme + "://" + entry.Route.Endpoint.HostTemplate + entry.Route.Endpoint.BasePath
		_, _ = fmt.Fprintf(
			&builder,
			"| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			markdownCell(string(entry.ID)),
			markdownCell(string(entry.Route.Provider)),
			markdownCell(entry.Implementation.AdapterID),
			markdownCell(string(entry.Route.Offering.ID)),
			markdownCell(string(entry.Route.Deployment.ID)),
			markdownCell(string(entry.Route.Protocol.ID)),
			markdownCell(endpoint),
			markdownCell(string(entry.Route.Credential.ID)),
			markdownCell(string(entry.Evidence.Status)),
			markdownCell(string(entry.Implementation.Status)),
		)
	}
	if callable == 0 {
		return nil, fmt.Errorf("render current catalog bindings: no callable bindings")
	}
	builder.WriteString(CurrentBindingsEndMarker)
	builder.WriteString("\n")
	return []byte(builder.String()), nil
}

// ReplaceCurrentBindingsMarkdown replaces exactly one checked-in generated
// block and preserves all manually reviewed research content around it.
func ReplaceCurrentBindingsMarkdown(markdown, generated []byte) ([]byte, error) {
	start := []byte(CurrentBindingsStartMarker)
	end := []byte(CurrentBindingsEndMarker)
	if bytes.Count(markdown, start) != 1 || bytes.Count(markdown, end) != 1 {
		return nil, fmt.Errorf("provider matrix must contain exactly one current-bindings marker pair")
	}
	startIndex := bytes.Index(markdown, start)
	endIndex := bytes.Index(markdown, end)
	if endIndex < startIndex {
		return nil, fmt.Errorf("provider matrix current-bindings markers are out of order")
	}
	endIndex += len(end)
	if endIndex < len(markdown) && markdown[endIndex] == '\r' {
		endIndex++
	}
	if endIndex < len(markdown) && markdown[endIndex] == '\n' {
		endIndex++
	}
	result := make([]byte, 0, len(markdown)-(endIndex-startIndex)+len(generated))
	result = append(result, markdown[:startIndex]...)
	result = append(result, generated...)
	result = append(result, markdown[endIndex:]...)
	return result, nil
}

// ValidateCurrentBindingsMarkdown rejects drift between the checked-in matrix
// block and the current machine-readable catalog.
func ValidateCurrentBindingsMarkdown(document Document, now time.Time, markdown []byte) error {
	generated, err := RenderCurrentBindingsMarkdown(document, now)
	if err != nil {
		return err
	}
	replaced, err := ReplaceCurrentBindingsMarkdown(markdown, generated)
	if err != nil {
		return err
	}
	if !bytes.Equal(replaced, markdown) {
		return fmt.Errorf("provider matrix current-bindings block is stale; regenerate it from catalog.DefaultDocument")
	}
	return nil
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", `\|`)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
