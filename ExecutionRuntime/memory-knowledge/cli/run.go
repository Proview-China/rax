// Package cli maps stable Praxis commands to the reference API. It does not
// choose an endpoint, credential source, process topology, or production SLA.
package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type Transport interface {
	Do(context.Context, string, []byte) ([]byte, error)
}
type HTTPTransport struct {
	BaseURL string
	Client  *http.Client
}

func (t HTTPTransport) Do(ctx context.Context, path string, body []byte) ([]byte, error) {
	if strings.TrimSpace(t.BaseURL) == "" || !strings.HasPrefix(path, "/v1/") {
		return nil, contract.ErrInvalidArgument
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	out, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(out) > 1<<20 {
		return nil, contract.ErrInvalidArgument
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: api status %d", contract.ErrNotCurrent, response.StatusCode)
	}
	return out, nil
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, transport Transport) error {
	if transport == nil || stdout == nil {
		return contract.ErrInvalidArgument
	}
	path, err := commandPath(args)
	if err != nil {
		return err
	}
	body := []byte("{}")
	if stdin != nil {
		read, err := io.ReadAll(io.LimitReader(stdin, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(read) > 1<<20 {
			return contract.ErrInvalidArgument
		}
		if len(bytes.TrimSpace(read)) > 0 {
			body = read
		}
	}
	result, err := transport.Do(ctx, path, body)
	if err != nil {
		return err
	}
	_, err = stdout.Write(result)
	return err
}

func commandPath(args []string) (string, error) {
	switch strings.Join(args, " ") {
	case "memory search":
		return "/v1/memory/query", nil
	case "memory inspect":
		return "/v1/memory/inspect", nil
	case "memory forget":
		return "/v1/memory/forget", nil
	case "knowledge source list":
		return "/v1/knowledge/source/list", nil
	case "knowledge snapshot build":
		return "/v1/knowledge/snapshot/build", nil
	case "knowledge query":
		return "/v1/knowledge/query", nil
	case "index status":
		return "/v1/index/status", nil
	default:
		return "", fmt.Errorf("%w: unknown command", contract.ErrInvalidArgument)
	}
}
