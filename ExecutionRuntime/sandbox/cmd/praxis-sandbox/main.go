package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
)

const usage = `praxis-sandbox [flags] COMMAND

Commands:
  backends, match, inspect, fence, diff, commit, checkpoint, restore,
  release, residuals, cleanup, lifecycle
  operation ID | execute ID | reconcile ID | cancel ID | watch

All effectful commands submit an asynchronous governed Controller intent.
Use --execute to ask the API worker to start the queued operation.`

type options struct {
	endpoint    string
	tenant      string
	token       string
	idempotency string
	payload     string
	payloadFile string
	execute     bool
	ttl         time.Duration
	timeout     time.Duration
	after       uint64
	limit       uint64
	wait        time.Duration
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	flags := flag.NewFlagSet("praxis-sandbox", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts options
	flags.StringVar(&opts.endpoint, "endpoint", "http://127.0.0.1:9781", "Sandbox API endpoint or unix:///path.sock")
	flags.StringVar(&opts.tenant, "tenant", "", "exact tenant ID")
	flags.StringVar(&opts.token, "token", "", "transport bearer token (or PRAXIS_SANDBOX_TOKEN)")
	flags.StringVar(&opts.idempotency, "idempotency-key", "", "stable idempotency key")
	flags.StringVar(&opts.payload, "payload", "{}", "strict JSON payload")
	flags.StringVar(&opts.payloadFile, "payload-file", "", "strict JSON payload file")
	flags.BoolVar(&opts.execute, "execute", false, "start the queued governed operation")
	flags.DurationVar(&opts.ttl, "ttl", 10*time.Minute, "request TTL")
	flags.DurationVar(&opts.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	flags.Uint64Var(&opts.after, "after", 0, "Watch cursor")
	flags.Uint64Var(&opts.limit, "limit", 100, "Watch result limit")
	flags.DurationVar(&opts.wait, "wait", 0, "Watch long-poll duration, max 30s")
	flags.Usage = func() { fmt.Fprintln(stderr, usage) }
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	if len(remaining) == 0 {
		flags.Usage()
		return errors.New("command is required")
	}
	if opts.token == "" {
		opts.token = getenv("PRAXIS_SANDBOX_TOKEN")
	}
	if opts.token == "" {
		return errors.New("transport bearer token is required")
	}
	client, base, err := newAPIClient(opts.endpoint, opts.timeout)
	if err != nil {
		return err
	}
	command := remaining[0]
	switch command {
	case "operation", "execute", "reconcile", "cancel":
		if len(remaining) != 2 {
			return fmt.Errorf("%s requires one operation ID", command)
		}
		method, path := http.MethodGet, "/v1/operations/"+url.PathEscape(remaining[1])
		if command != "operation" {
			method, path = http.MethodPost, path+"/"+command
		}
		return requestAndPrint(ctx, client, base+path, method, opts.token, nil, stdout)
	case "watch":
		if opts.limit == 0 || opts.limit > 1000 || opts.wait < 0 || opts.wait > 30*time.Second {
			return errors.New("watch requires limit 1..1000 and wait 0..30s")
		}
		query := url.Values{"after": {strconv.FormatUint(opts.after, 10)}, "limit": {strconv.FormatUint(opts.limit, 10)}, "wait_ms": {strconv.FormatInt(opts.wait.Milliseconds(), 10)}}
		return requestAndPrint(ctx, client, base+"/v1/watch?"+query.Encode(), http.MethodGet, opts.token, nil, stdout)
	default:
		action, ok := commandAction(command)
		if !ok {
			return fmt.Errorf("unknown command %q", command)
		}
		if opts.tenant == "" {
			return errors.New("--tenant is required for a new operation")
		}
		payload, err := readPayload(opts)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		id, err := randomID("sandbox-op")
		if err != nil {
			return err
		}
		key := opts.idempotency
		if key == "" {
			key = id
		}
		request, err := api.SealOperationRequestV1(api.OperationRequestV1{
			RequestID: id, IdempotencyKey: key, TenantID: opts.tenant, Action: action,
			PayloadSchema: "praxis.sandbox.api/" + command + "/v1", PayloadRevision: 1, Payload: payload,
			RequestedUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(opts.ttl).UnixNano(),
		})
		if err != nil {
			return err
		}
		body, err := json.Marshal(request)
		if err != nil {
			return err
		}
		fact, err := requestFact(ctx, client, base+"/v1/operations", http.MethodPost, opts.token, body)
		if err != nil {
			return err
		}
		if opts.execute {
			fact, err = requestFact(ctx, client, base+"/v1/operations/"+url.PathEscape(fact.ID)+"/execute", http.MethodPost, opts.token, nil)
			if err != nil {
				return err
			}
		}
		return writePrettyJSON(stdout, fact)
	}
}

func commandAction(command string) (api.ActionV1, bool) {
	switch command {
	case "backends":
		return api.ActionDescribeBackendsV1, true
	case "match":
		return api.ActionMatchRequirementV1, true
	case "inspect", "residuals":
		return api.ActionInspectV1, true
	case "diff":
		return api.ActionWorkspaceDiffV1, true
	case "lifecycle":
		return api.ActionLifecycleV1, true
	case "fence":
		return api.ActionFenceV1, true
	case "commit":
		return api.ActionWorkspaceCommitV1, true
	case "checkpoint":
		return api.ActionCheckpointV1, true
	case "restore":
		return api.ActionWorkspaceRestoreV1, true
	case "release":
		return api.ActionReleaseV1, true
	case "cleanup":
		return api.ActionCleanupV1, true
	default:
		return "", false
	}
}

func readPayload(opts options) ([]byte, error) {
	if opts.payloadFile == "" {
		return []byte(opts.payload), nil
	}
	if opts.payload != "{}" {
		return nil, errors.New("use only one of --payload and --payload-file")
	}
	return os.ReadFile(opts.payloadFile)
}

func newAPIClient(endpoint string, timeout time.Duration) (*http.Client, string, error) {
	if timeout <= 0 {
		return nil, "", errors.New("HTTP timeout must be positive")
	}
	if strings.HasPrefix(endpoint, "unix://") {
		path := strings.TrimPrefix(endpoint, "unix://")
		if path == "" {
			return nil, "", errors.New("Unix socket path is required")
		}
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		}}
		return &http.Client{Transport: transport, Timeout: timeout}, "http://unix", nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, "", errors.New("endpoint must be http(s)://host or unix:///path.sock")
	}
	return &http.Client{Timeout: timeout}, strings.TrimRight(endpoint, "/"), nil
}

func requestAndPrint(ctx context.Context, client *http.Client, target, method, token string, body []byte, stdout io.Writer) error {
	response, err := requestJSON(ctx, client, target, method, token, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("sandbox API %s: %s", response.Status, strings.TrimSpace(string(payload)))
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return err
	}
	return writePrettyJSON(stdout, value)
}

func requestFact(ctx context.Context, client *http.Client, target, method, token string, body []byte) (api.OperationFactV1, error) {
	response, err := requestJSON(ctx, client, target, method, token, body)
	if err != nil {
		return api.OperationFactV1{}, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return api.OperationFactV1{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return api.OperationFactV1{}, fmt.Errorf("sandbox API %s: %s", response.Status, strings.TrimSpace(string(payload)))
	}
	var fact api.OperationFactV1
	if err := json.Unmarshal(payload, &fact); err != nil {
		return api.OperationFactV1{}, err
	}
	return fact, fact.ValidateShape()
}

func requestJSON(ctx context.Context, client *http.Client, target, method, token string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return client.Do(request)
}

func writePrettyJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func randomID(prefix string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(entropy[:]), nil
}
